package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"mgfilebox/internal/models"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *sql.DB
}

func New(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)

	repo := &Repository{db: db}
	if err := repo.migrate(context.Background()); err != nil {
		return nil, err
	}

	return repo, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS shares (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL DEFAULT 'file' CHECK (type IN ('file')),
			title TEXT NOT NULL,
			file_name TEXT,
			mime_type TEXT,
			storage_path TEXT,
			access_password_hash TEXT,
			pickcode_encrypted TEXT,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_shares_expires_at ON shares (expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_shares_deleted_at ON shares (deleted_at);`,
		`CREATE TABLE IF NOT EXISTS share_files (
			id TEXT PRIMARY KEY,
			share_id TEXT NOT NULL,
			file_name TEXT NOT NULL,
			mime_type TEXT,
			storage_path TEXT NOT NULL,
			size INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (share_id) REFERENCES shares (id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_share_files_share_id ON share_files (share_id);`,
		`INSERT INTO share_files (id, share_id, file_name, mime_type, storage_path, size, created_at)
		 SELECT id || '-legacy', id, file_name, mime_type, storage_path, 0, created_at
		 FROM shares
		 WHERE file_name IS NOT NULL AND file_name <> '' AND storage_path IS NOT NULL AND storage_path <> ''
		 AND NOT EXISTS (SELECT 1 FROM share_files WHERE share_files.share_id = shares.id);`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (
			id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires_at ON admin_sessions (expires_at);`,
		`CREATE TABLE IF NOT EXISTS access_logs (
			id TEXT PRIMARY KEY,
			share_id TEXT NOT NULL,
			action TEXT NOT NULL,
			ip TEXT,
			user_agent TEXT,
			status_code INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_share_id ON access_logs (share_id);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_created_at ON access_logs (created_at);`,
	}

	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}

	if err := r.ensureColumn(ctx, "shares", "pickcode_encrypted", "TEXT"); err != nil {
		return err
	}

	return nil
}

func (r *Repository) ensureColumn(ctx context.Context, table, column, columnType string) error {
	rows, err := r.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if found {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+columnType); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

func (r *Repository) CreateShare(ctx context.Context, share models.Share) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO shares (
			id, type, title, file_name, mime_type, storage_path, access_password_hash, pickcode_encrypted, expires_at, created_at, deleted_at
		) VALUES (?, 'file', ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, share.ID, share.Title, share.FileName, share.MimeType, share.StoragePath,
		share.AccessPasswordHash, share.PickcodeEncrypted, share.ExpiresAt.UTC(), share.CreatedAt.UTC())
	if err != nil {
		return err
	}

	files := share.Files
	if len(files) == 0 && share.FileName != "" && share.StoragePath != "" {
		files = []models.ShareFile{{
			ID:          share.ID + "-legacy",
			ShareID:     share.ID,
			FileName:    share.FileName,
			MimeType:    share.MimeType,
			StoragePath: share.StoragePath,
			CreatedAt:   share.CreatedAt,
		}}
	}

	for _, file := range files {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO share_files (id, share_id, file_name, mime_type, storage_path, size, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, file.ID, share.ID, file.FileName, file.MimeType, file.StoragePath, file.Size, file.CreatedAt.UTC()); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *Repository) GetShareByID(ctx context.Context, id string) (*models.Share, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, title, file_name, mime_type, storage_path, access_password_hash, COALESCE(pickcode_encrypted, ''), expires_at, created_at, deleted_at
		FROM shares
		WHERE id = ?
	`, id)

	share, err := scanShare(row)
	if err != nil {
		return nil, err
	}
	files, err := r.listShareFiles(ctx, share.ID)
	if err != nil {
		return nil, err
	}
	share.Files = files
	return share, nil
}

func (r *Repository) ListShares(ctx context.Context) ([]models.Share, error) {
	return r.listShares(ctx, `
		SELECT id, title, file_name, mime_type, storage_path, access_password_hash, COALESCE(pickcode_encrypted, ''), expires_at, created_at, deleted_at
		FROM shares
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)
}

func (r *Repository) ListDeletedShares(ctx context.Context) ([]models.Share, error) {
	return r.listShares(ctx, `
		SELECT id, title, file_name, mime_type, storage_path, access_password_hash, COALESCE(pickcode_encrypted, ''), expires_at, created_at, deleted_at
		FROM shares
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
	`)
}

func (r *Repository) GetShareFile(ctx context.Context, shareID, fileID string) (*models.ShareFile, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, share_id, file_name, mime_type, storage_path, size, created_at
		FROM share_files
		WHERE share_id = ? AND id = ?
	`, shareID, fileID)

	var file models.ShareFile
	if err := row.Scan(&file.ID, &file.ShareID, &file.FileName, &file.MimeType, &file.StoragePath, &file.Size, &file.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &file, nil
}

func (r *Repository) listShareFiles(ctx context.Context, shareID string) ([]models.ShareFile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, share_id, file_name, mime_type, storage_path, size, created_at
		FROM share_files
		WHERE share_id = ?
		ORDER BY created_at, id
	`, shareID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]models.ShareFile, 0)
	for rows.Next() {
		var file models.ShareFile
		if err := rows.Scan(&file.ID, &file.ShareID, &file.FileName, &file.MimeType, &file.StoragePath, &file.Size, &file.CreatedAt); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (r *Repository) SoftDeleteShare(ctx context.Context, id string, deletedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE shares
		SET deleted_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, deletedAt.UTC(), id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) PurgeDeletedShare(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM access_logs WHERE share_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM share_files WHERE share_id = ?`, id); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM shares WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}

func (r *Repository) CreateAdminSession(ctx context.Context, session models.AdminSession) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO admin_sessions (id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, session.ID, session.TokenHash, session.ExpiresAt.UTC(), session.CreatedAt.UTC())
	return err
}

func (r *Repository) GetAdminSessionByTokenHash(ctx context.Context, tokenHash string) (*models.AdminSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, token_hash, expires_at, created_at
		FROM admin_sessions
		WHERE token_hash = ?
	`, tokenHash)

	var session models.AdminSession
	if err := row.Scan(&session.ID, &session.TokenHash, &session.ExpiresAt, &session.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &session, nil
}

func (r *Repository) DeleteAdminSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (r *Repository) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= ?`, now.UTC())
	return err
}

func (r *Repository) LogAccess(ctx context.Context, log models.AccessLog) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO access_logs (id, share_id, action, ip, user_agent, status_code, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.ShareID, log.Action, log.IP, log.UserAgent, log.StatusCode, log.CreatedAt.UTC())
	return err
}

func (r *Repository) CountSuccessfulDownloads(ctx context.Context, shareID string) (int64, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM access_logs
		WHERE share_id = ? AND action = 'download' AND status_code = 200
	`, shareID)

	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) CountSuccessfulDownloadsByShareIDs(ctx context.Context, shareIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(shareIDs))
	if len(shareIDs) == 0 {
		return counts, nil
	}

	placeholders := make([]string, len(shareIDs))
	args := make([]any, len(shareIDs))
	for index, shareID := range shareIDs {
		placeholders[index] = "?"
		args[index] = shareID
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT share_id, COUNT(*)
		FROM access_logs
		WHERE action = 'download' AND status_code = 200 AND share_id IN (`+strings.Join(placeholders, ",")+`)
		GROUP BY share_id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var shareID string
		var count int64
		if err := rows.Scan(&shareID, &count); err != nil {
			return nil, err
		}
		counts[shareID] = count
	}
	return counts, rows.Err()
}

func (r *Repository) ListExpiredFileShares(ctx context.Context, now time.Time) ([]models.Share, error) {
	return r.listShares(ctx, `
		SELECT id, title, file_name, mime_type, storage_path, access_password_hash, COALESCE(pickcode_encrypted, ''), expires_at, created_at, deleted_at
		FROM shares
		WHERE deleted_at IS NULL AND expires_at <= ?
	`, now.UTC())
}

func (r *Repository) listShares(ctx context.Context, query string, args ...any) ([]models.Share, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []models.Share
	for rows.Next() {
		share, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, *share)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for index := range shares {
		files, err := r.listShareFiles(ctx, shares[index].ID)
		if err != nil {
			return nil, err
		}
		shares[index].Files = files
	}
	return shares, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanShare(s scanner) (*models.Share, error) {
	var share models.Share
	var deletedAt sql.NullTime
	if err := s.Scan(
		&share.ID,
		&share.Title,
		&share.FileName,
		&share.MimeType,
		&share.StoragePath,
		&share.AccessPasswordHash,
		&share.PickcodeEncrypted,
		&share.ExpiresAt,
		&share.CreatedAt,
		&deletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if deletedAt.Valid {
		value := deletedAt.Time
		share.DeletedAt = &value
	}
	return &share, nil
}
