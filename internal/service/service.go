package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mgfilebox/internal/config"
	"mgfilebox/internal/models"
	"mgfilebox/internal/repository"
	"mgfilebox/internal/security"
)

type Service struct {
	repo *repository.Repository
	cfg  *config.Config
	now  func() time.Time
}

func New(repo *repository.Repository, cfg *config.Config) *Service {
	return &Service{
		repo: repo,
		cfg:  cfg,
		now:  time.Now,
	}
}

func (s *Service) Login(ctx context.Context, password string) (string, error) {
	if err := security.CheckPassword(s.cfg.AdminPasswordHash, password); err != nil {
		return "", errors.New("管理员密码错误")
	}

	token, err := security.RandomToken(32)
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	sessionID, err := security.RandomToken(16)
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	session := models.AdminSession{
		ID:        sessionID,
		TokenHash: security.SHA256Hex(token),
		ExpiresAt: s.now().Add(s.cfg.SessionTTL),
		CreatedAt: s.now(),
	}

	if err := s.repo.CreateAdminSession(ctx, session); err != nil {
		return "", err
	}

	return token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.repo.DeleteAdminSessionByTokenHash(ctx, security.SHA256Hex(token))
}

func (s *Service) ValidateAdminSession(ctx context.Context, token string) error {
	session, err := s.repo.GetAdminSessionByTokenHash(ctx, security.SHA256Hex(token))
	if err != nil {
		return err
	}
	if !session.ExpiresAt.After(s.now()) {
		_ = s.repo.DeleteAdminSessionByTokenHash(ctx, session.TokenHash)
		return errors.New("登录已过期")
	}
	return nil
}

func (s *Service) CreateFileShare(ctx context.Context, input models.ShareInput, fileHeaders []*multipart.FileHeader) (*models.ShareSummary, error) {
	if len(fileHeaders) == 0 {
		return nil, errors.New("请选择文件")
	}

	shareID, err := generateShareID()
	if err != nil {
		return nil, err
	}

	share, err := s.buildShare(input, shareID)
	if err != nil {
		return nil, err
	}

	createdPaths := make([]string, 0, len(fileHeaders))
	for index, fileHeader := range fileHeaders {
		if fileHeader == nil {
			continue
		}
		fileID, err := security.RandomToken(8)
		if err != nil {
			removeFiles(createdPaths)
			return nil, err
		}
		fileID = strings.ToLower(strings.TrimRight(fileID, "="))
		storageName := shareID + "-" + fileID + filepath.Ext(fileHeader.Filename)
		storagePath := filepath.Join(s.cfg.UploadDir, storageName)
		if err := saveUploadedFile(fileHeader, storagePath); err != nil {
			removeFiles(createdPaths)
			return nil, err
		}
		createdPaths = append(createdPaths, storagePath)
		share.Files = append(share.Files, models.ShareFile{
			ID:          fileID,
			ShareID:     shareID,
			FileName:    filepath.Base(fileHeader.Filename),
			MimeType:    fileHeader.Header.Get("Content-Type"),
			StoragePath: storagePath,
			Size:        fileHeader.Size,
			CreatedAt:   share.CreatedAt.Add(time.Duration(index) * time.Nanosecond),
		})
	}
	if len(share.Files) == 0 {
		return nil, errors.New("请选择文件")
	}
	share.FileName = share.Files[0].FileName
	share.MimeType = share.Files[0].MimeType
	share.StoragePath = share.Files[0].StoragePath
	if share.Title == "" {
		if len(share.Files) == 1 {
			share.Title = share.Files[0].FileName
		} else {
			share.Title = fmt.Sprintf("%d 个文件", len(share.Files))
		}
	}

	if err := s.repo.CreateShare(ctx, share); err != nil {
		removeFiles(createdPaths)
		return nil, err
	}

	return s.toSummary(share), nil
}

func (s *Service) ListShares(ctx context.Context) ([]models.ShareSummary, error) {
	shares, err := s.repo.ListShares(ctx)
	if err != nil {
		return nil, err
	}
	summaries := s.toSummaries(shares)
	if err := s.fillDownloadCounts(ctx, summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

func (s *Service) ListDeletedShares(ctx context.Context) ([]models.ShareSummary, error) {
	shares, err := s.repo.ListDeletedShares(ctx)
	if err != nil {
		return nil, err
	}
	summaries := s.toSummaries(shares)
	if err := s.fillDownloadCounts(ctx, summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

func (s *Service) GetShare(ctx context.Context, id string) (*models.Share, error) {
	share, err := s.repo.GetShareByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return share, nil
}

func (s *Service) GetShareFile(ctx context.Context, shareID, fileID string) (*models.ShareFile, error) {
	return s.repo.GetShareFile(ctx, shareID, fileID)
}

func (s *Service) CountSuccessfulDownloads(ctx context.Context, shareID string) (int64, error) {
	return s.repo.CountSuccessfulDownloads(ctx, shareID)
}

func (s *Service) DeleteShare(ctx context.Context, id string) error {
	share, err := s.repo.GetShareByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.repo.SoftDeleteShare(ctx, id, s.now()); err != nil {
		return err
	}

	removeShareFiles(share.Files)
	return nil
}

func (s *Service) PurgeDeletedShare(ctx context.Context, id string) error {
	return s.repo.PurgeDeletedShare(ctx, id)
}

func (s *Service) CanAccessShare(share *models.Share, signedUnlock string) bool {
	if share == nil || share.IsDeleted() || share.IsExpired(s.now()) {
		return false
	}
	if !share.HasPassword() {
		return true
	}
	if signedUnlock == "" {
		return false
	}

	value, err := security.VerifySignedValue(s.cfg.CookieSecret, signedUnlock)
	if err != nil {
		return false
	}

	expected := share.ID + ":" + security.SHA256Hex(share.AccessPasswordHash)
	return value == expected
}

func (s *Service) VerifySharePassword(share *models.Share, password string) (string, error) {
	if share == nil {
		return "", errors.New("分享不存在")
	}
	if !share.HasPassword() {
		return "", nil
	}
	if err := security.CheckPassword(share.AccessPasswordHash, password); err != nil {
		return "", errors.New("提取码错误")
	}
	value := share.ID + ":" + security.SHA256Hex(share.AccessPasswordHash)
	return security.SignValue(s.cfg.CookieSecret, value), nil
}

func (s *Service) CleanupExpired(ctx context.Context) error {
	now := s.now()
	expiredShares, err := s.repo.ListExpiredFileShares(ctx, now)
	if err != nil {
		return err
	}

	for _, share := range expiredShares {
		if err := s.repo.SoftDeleteShare(ctx, share.ID, now); err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		removeShareFiles(share.Files)
	}

	return s.repo.DeleteExpiredSessions(ctx, now)
}

func (s *Service) RecordAccess(ctx context.Context, shareID, action, ip, userAgent string, statusCode int) {
	logID, err := security.RandomToken(12)
	if err != nil {
		return
	}

	_ = s.repo.LogAccess(ctx, models.AccessLog{
		ID:         logID,
		ShareID:    shareID,
		Action:     action,
		IP:         ip,
		UserAgent:  userAgent,
		StatusCode: statusCode,
		CreatedAt:  s.now(),
	})
}

func (s *Service) buildShare(input models.ShareInput, shareID string) (models.Share, error) {
	expiresHours := input.ExpiresHours
	if expiresHours < 0 {
		return models.Share{}, errors.New("请选择过期时间")
	}

	expiresAt := s.now().Add(time.Duration(expiresHours) * time.Hour)
	if expiresHours == 0 {
		expiresAt = models.NeverExpiresAt
	}

	share := models.Share{
		ID:        shareID,
		Title:     strings.TrimSpace(input.Title),
		ExpiresAt: expiresAt,
		CreatedAt: s.now(),
	}

	if password := strings.TrimSpace(input.AccessPassword); password != "" {
		hash, err := security.HashPassword(password)
		if err != nil {
			return models.Share{}, err
		}
		share.AccessPasswordHash = hash
		encrypted, err := security.EncryptValue(s.cfg.CookieSecret, password)
		if err != nil {
			return models.Share{}, err
		}
		share.PickcodeEncrypted = encrypted
	}

	return share, nil
}

func (s *Service) toSummary(share models.Share) *models.ShareSummary {
	totalSize := int64(0)
	for _, file := range share.Files {
		totalSize += file.Size
	}
	shareURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/s/" + share.ID
	if share.PickcodeEncrypted != "" {
		if pickcode, err := security.DecryptValue(s.cfg.CookieSecret, share.PickcodeEncrypted); err == nil && pickcode != "" {
			shareURL += "?pickcode=" + url.QueryEscape(pickcode)
		}
	}
	return &models.ShareSummary{
		ID:          share.ID,
		Title:       share.Title,
		FileName:    share.FileName,
		HasPassword: share.HasPassword(),
		ExpiresAt:   share.ExpiresAt,
		CreatedAt:   share.CreatedAt,
		Status:      share.Status(s.now()),
		ShareURL:    shareURL,
		Files:       share.Files,
		FileCount:   len(share.Files),
		TotalSize:   totalSize,
		DeletedAt:   share.DeletedAt,
	}
}

func (s *Service) toSummaries(shares []models.Share) []models.ShareSummary {
	summaries := make([]models.ShareSummary, 0, len(shares))
	for _, share := range shares {
		summaries = append(summaries, *s.toSummary(share))
	}
	return summaries
}

func (s *Service) fillDownloadCounts(ctx context.Context, summaries []models.ShareSummary) error {
	shareIDs := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		shareIDs = append(shareIDs, summary.ID)
	}

	counts, err := s.repo.CountSuccessfulDownloadsByShareIDs(ctx, shareIDs)
	if err != nil {
		return err
	}
	for index := range summaries {
		summaries[index].DownloadCount = counts[summaries[index].ID]
	}
	return nil
}

func saveUploadedFile(fileHeader *multipart.FileHeader, storagePath string) error {
	source, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(storagePath)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}

func removeFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func removeShareFiles(files []models.ShareFile) {
	for _, file := range files {
		_ = os.Remove(file.StoragePath)
	}
}

func generateShareID() (string, error) {
	return security.RandomAlphaNumeric(12)
}
