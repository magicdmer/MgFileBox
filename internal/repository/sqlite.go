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
		`CREATE TABLE IF NOT EXISTS obsolete_files (storage_path TEXT PRIMARY KEY);`,
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
	if err := r.ensureColumn(ctx, "shares", "description", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	for column, columnType := range map[string]string{
		"public_id": "TEXT", "cancelled": "INTEGER NOT NULL DEFAULT 0", "expires_hours": "INTEGER NOT NULL DEFAULT 24",
	} {
		if err := r.ensureColumn(ctx, "shares", column, columnType); err != nil {
			return err
		}
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE shares SET public_id = id WHERE public_id IS NULL`); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_shares_public_id ON shares (public_id)`); err != nil {
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
			id, type, title, description, file_name, mime_type, storage_path, access_password_hash, pickcode_encrypted, expires_at, created_at, deleted_at, public_id, cancelled, expires_hours
		) VALUES (?, 'file', ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)
	`, share.ID, share.Title, share.Description, share.FileName, share.MimeType, share.StoragePath,
		share.AccessPasswordHash, share.PickcodeEncrypted, share.ExpiresAt.UTC(), share.CreatedAt.UTC(), share.LinkID(), share.Cancelled, share.ExpiresHours)
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
		SELECT id, title, description, file_name, mime_type, storage_path, access_password_hash, COALESCE(pickcode_encrypted, ''), expires_at, created_at, deleted_at, public_id, cancelled, expires_hours
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

func (r *Repository) GetShareByPublicID(ctx context.Context, publicID string) (*models.Share, error) {
	var id string
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM shares WHERE public_id = ?`, publicID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	share, err := r.GetShareByID(ctx, id)
	if err == nil && share.PublicID != publicID {
		return nil, ErrNotFound
	}
	return share, err
}

func (r *Repository) CancelShare(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE shares SET cancelled = 1 WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Reshare(ctx context.Context, id, previousPublicID string, share models.Share) error {
	result, err := r.db.ExecContext(ctx, `UPDATE shares SET public_id = ?, cancelled = 0, description = ?, access_password_hash = ?, pickcode_encrypted = ?, expires_at = ?, expires_hours = ? WHERE id = ? AND public_id = ? AND deleted_at IS NULL`,
		share.PublicID, share.Description, share.AccessPasswordHash, share.PickcodeEncrypted, share.ExpiresAt.UTC(), share.ExpiresHours, id, previousPublicID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ListShares(ctx context.Context) ([]models.Share, error) {
	return r.listShares(ctx, `
		SELECT id, title, description, file_name, mime_type, storage_path, access_password_hash, COALESCE(pickcode_encrypted, ''), expires_at, created_at, deleted_at, public_id, cancelled, expires_hours
		FROM shares
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)
}

func (r *Repository) ListDeletedShares(ctx context.Context) ([]models.Share, error) {
	return r.listShares(ctx, `
		SELECT id, title, description, file_name, mime_type, storage_path, access_password_hash, COALESCE(pickcode_encrypted, ''), expires_at, created_at, deleted_at, public_id, cancelled, expires_hours
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

// ReplaceShareFile switches metadata only after the new file is fully stored.
// The previous path is retained for retryable cleanup, including after restart.
func (r *Repository) ReplaceShareFile(ctx context.Context, old models.ShareFile, file models.ShareFile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE share_files SET file_name = ?, mime_type = ?, storage_path = ?, size = ?
		WHERE share_id = ? AND id = ? AND storage_path = ?
		AND EXISTS (SELECT 1 FROM shares WHERE id = ? AND deleted_at IS NULL)`,
		file.FileName, file.MimeType, file.StoragePath, file.Size, old.ShareID, old.ID, old.StoragePath, old.ShareID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	// Keep the legacy first-file fields in sync. Follow renamed auto titles,
	// while preserving titles explicitly chosen by the administrator.
	_, err = tx.ExecContext(ctx, `UPDATE shares SET file_name = ?, mime_type = ?, storage_path = ?,
		title = CASE WHEN title = ? THEN ? ELSE title END
		WHERE id = ? AND (SELECT id FROM share_files WHERE share_id = ? ORDER BY created_at, id LIMIT 1) = ?`,
		file.FileName, file.MimeType, file.StoragePath, old.FileName, file.FileName, old.ShareID, old.ShareID, old.ID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO obsolete_files (storage_path) VALUES (?)`, old.StoragePath); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ListObsoleteFiles(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT storage_path FROM obsolete_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func (r *Repository) ForgetObsoleteFile(ctx context.Context, path string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM obsolete_files WHERE storage_path = ?`, path)
	return err
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

func (r *Repository) UpdateShareDescription(ctx context.Context, id, description string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE shares SET description = ? WHERE id = ? AND deleted_at IS NULL`, description, id)
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
		&share.Description,
		&share.FileName,
		&share.MimeType,
		&share.StoragePath,
		&share.AccessPasswordHash,
		&share.PickcodeEncrypted,
		&share.ExpiresAt,
		&share.CreatedAt,
		&deletedAt,
		&share.PublicID,
		&share.Cancelled,
		&share.ExpiresHours,
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
