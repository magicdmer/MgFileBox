package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mgfilebox/internal/config"
	"mgfilebox/internal/models"
	"mgfilebox/internal/repository"
	"mgfilebox/internal/security"
)

func TestLoginAndValidateSession(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	token, err := svc.Login(context.Background(), "secret-pass")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if err := svc.ValidateAdminSession(context.Background(), token); err != nil {
		t.Fatalf("session should be valid: %v", err)
	}

	if err := svc.Logout(context.Background(), token); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	if err := svc.ValidateAdminSession(context.Background(), token); err == nil {
		t.Fatalf("session should be invalid after logout")
	}
}

func TestSharePasswordFlow(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	share, err := svc.buildShare(models.ShareInput{
		Title:          "测试文件",
		AccessPassword: "friend-pass",
		ExpiresHours:   24,
	}, "share-password-test")
	if err != nil {
		t.Fatalf("build share failed: %v", err)
	}
	share.FileName = "demo.txt"
	share.StoragePath = filepath.Join(svc.cfg.UploadDir, "demo.txt")

	if err := os.WriteFile(share.StoragePath, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write share file failed: %v", err)
	}

	if err := svc.repo.CreateShare(context.Background(), share); err != nil {
		t.Fatalf("create share failed: %v", err)
	}

	stored, err := svc.GetShare(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get share failed: %v", err)
	}

	if svc.CanAccessShare(stored, "") {
		t.Fatalf("share should require password before unlock")
	}
	if stored.PickcodeEncrypted == "" {
		t.Fatalf("expected encrypted pickcode to be stored")
	}
	if summary := svc.toSummary(*stored); summary.ShareURL != "http://localhost:8080/s/share-password-test?pickcode=friend-pass" {
		t.Fatalf("unexpected pickcode URL: %s", summary.ShareURL)
	}

	cookieValue, err := svc.VerifySharePassword(stored, "friend-pass")
	if err != nil {
		t.Fatalf("verify share password failed: %v", err)
	}

	if !svc.CanAccessShare(stored, cookieValue) {
		t.Fatalf("share should be accessible after unlock")
	}
}

func TestCleanupExpiredRemovesFile(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	past := time.Now().Add(-2 * time.Hour)
	svc.now = func() time.Time { return past }

	filePath := filepath.Join(svc.cfg.UploadDir, "expired.txt")
	if err := os.WriteFile(filePath, []byte("temp"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	share, err := svc.buildShare(models.ShareInput{
		Title:        "过期文件",
		ExpiresHours: 1,
	}, "expired123")
	if err != nil {
		t.Fatalf("build share failed: %v", err)
	}
	share.FileName = "expired.txt"
	share.StoragePath = filePath

	if err := svc.repo.CreateShare(context.Background(), share); err != nil {
		t.Fatalf("create expired share failed: %v", err)
	}

	svc.now = func() time.Time { return time.Now() }
	if err := svc.CleanupExpired(context.Background()); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	updated, err := svc.GetShare(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get updated share failed: %v", err)
	}
	if updated.DeletedAt == nil {
		t.Fatalf("expected share to be soft deleted")
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, got err=%v", err)
	}
}

func TestBuildShareWithNeverExpires(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	share, err := svc.buildShare(models.ShareInput{
		Title:        "长期分享",
		ExpiresHours: 0,
	}, "never-expire")
	if err != nil {
		t.Fatalf("build share failed: %v", err)
	}

	if !share.NeverExpires() {
		t.Fatalf("expected share to never expire")
	}

	if share.IsExpired(time.Now()) {
		t.Fatalf("never-expiring share should not be expired")
	}
}

func TestMultiFileShareSummary(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	createdAt := time.Now()
	share := models.Share{
		ID:        "multi-file-test",
		Title:     "多文件分享",
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(24 * time.Hour),
		Files: []models.ShareFile{
			{ID: "file-one", FileName: "one.txt", StoragePath: filepath.Join(svc.cfg.UploadDir, "one.txt"), Size: 1024, CreatedAt: createdAt},
			{ID: "file-two", FileName: "two.txt", StoragePath: filepath.Join(svc.cfg.UploadDir, "two.txt"), Size: 2048, CreatedAt: createdAt.Add(time.Nanosecond)},
		},
	}
	share.FileName = share.Files[0].FileName
	share.StoragePath = share.Files[0].StoragePath

	if err := svc.repo.CreateShare(context.Background(), share); err != nil {
		t.Fatalf("create multi-file share failed: %v", err)
	}

	summaries, err := svc.ListShares(context.Background())
	if err != nil {
		t.Fatalf("list shares failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].FileCount != 2 || summaries[0].TotalSize != 3072 {
		t.Fatalf("unexpected file summary: count=%d size=%d", summaries[0].FileCount, summaries[0].TotalSize)
	}
}

func TestDeletedShareIsHiddenFromList(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	createdAt := time.Now()
	filePath := filepath.Join(svc.cfg.UploadDir, "delete-me.txt")
	if err := os.WriteFile(filePath, []byte("delete me"), 0o644); err != nil {
		t.Fatalf("write share file: %v", err)
	}
	share := models.Share{
		ID:          "delete-list-test",
		Title:       "待删除分享",
		FileName:    "delete-me.txt",
		StoragePath: filePath,
		CreatedAt:   createdAt,
		ExpiresAt:   createdAt.Add(24 * time.Hour),
	}
	if err := svc.repo.CreateShare(context.Background(), share); err != nil {
		t.Fatalf("create share: %v", err)
	}
	if err := svc.DeleteShare(context.Background(), share.ID); err != nil {
		t.Fatalf("delete share: %v", err)
	}

	summaries, err := svc.ListShares(context.Background())
	if err != nil {
		t.Fatalf("list shares: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected deleted share to be hidden, got %d summaries", len(summaries))
	}

	deletedSummaries, err := svc.ListDeletedShares(context.Background())
	if err != nil {
		t.Fatalf("list deleted shares: %v", err)
	}
	if len(deletedSummaries) != 1 {
		t.Fatalf("expected one deleted share, got %d", len(deletedSummaries))
	}
	if deletedSummaries[0].DeletedAt == nil {
		t.Fatalf("expected deleted time in summary")
	}
}

func TestPurgeDeletedShareRemovesMetadata(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	createdAt := time.Now()
	filePath := filepath.Join(svc.cfg.UploadDir, "purge-me.txt")
	if err := os.WriteFile(filePath, []byte("purge me"), 0o644); err != nil {
		t.Fatalf("write share file: %v", err)
	}
	share := models.Share{
		ID:          "purge-metadata-test",
		Title:       "待清除记录",
		FileName:    "purge-me.txt",
		StoragePath: filePath,
		CreatedAt:   createdAt,
		ExpiresAt:   createdAt.Add(24 * time.Hour),
	}
	if err := svc.repo.CreateShare(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}
	if err := svc.DeleteShare(ctx, share.ID); err != nil {
		t.Fatalf("delete share: %v", err)
	}
	if err := svc.PurgeDeletedShare(ctx, share.ID); err != nil {
		t.Fatalf("purge deleted share: %v", err)
	}

	deletedSummaries, err := svc.ListDeletedShares(ctx)
	if err != nil {
		t.Fatalf("list deleted shares: %v", err)
	}
	if len(deletedSummaries) != 0 {
		t.Fatalf("expected deleted list to be empty, got %d", len(deletedSummaries))
	}
	if _, err := svc.GetShare(ctx, share.ID); err == nil {
		t.Fatalf("expected purged share to be removed")
	}
}

func TestCountSuccessfulDownloads(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	ctx := context.Background()
	shareID := "download-count-test"
	createdAt := time.Now()
	share := models.Share{
		ID:        shareID,
		Title:     "下载计数",
		FileName:  "count.txt",
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(24 * time.Hour),
	}
	if err := svc.repo.CreateShare(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	svc.RecordAccess(ctx, shareID, "view", "127.0.0.1", "test", 200)
	svc.RecordAccess(ctx, shareID, "download", "127.0.0.1", "test", 200)
	svc.RecordAccess(ctx, shareID, "download", "127.0.0.1", "test", 403)
	svc.RecordAccess(ctx, shareID, "download", "127.0.0.1", "test", 200)

	count, err := svc.CountSuccessfulDownloads(ctx, shareID)
	if err != nil {
		t.Fatalf("count downloads: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 successful downloads, got %d", count)
	}

	summaries, err := svc.ListShares(ctx)
	if err != nil {
		t.Fatalf("list shares: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].DownloadCount != 2 {
		t.Fatalf("expected summary download count 2, got %d", summaries[0].DownloadCount)
	}
}

func TestCleanupExpiredKeepsNeverExpiresShare(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	filePath := filepath.Join(svc.cfg.UploadDir, "forever.txt")
	if err := os.WriteFile(filePath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	share, err := svc.buildShare(models.ShareInput{
		Title:        "永不过期文件",
		ExpiresHours: 0,
	}, "forever123")
	if err != nil {
		t.Fatalf("build share failed: %v", err)
	}
	share.FileName = "forever.txt"
	share.StoragePath = filePath

	if err := svc.repo.CreateShare(context.Background(), share); err != nil {
		t.Fatalf("create never-expiring share failed: %v", err)
	}

	if err := svc.CleanupExpired(context.Background()); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	updated, err := svc.GetShare(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get updated share failed: %v", err)
	}
	if updated.DeletedAt != nil {
		t.Fatalf("expected never-expiring share to remain active")
	}

	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected file to remain, got err=%v", err)
	}
}

func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()

	tempDir := t.TempDir()
	adminHash, err := security.HashPassword("secret-pass")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}

	cfg := &config.Config{
		BaseURL:           "http://localhost:8080",
		DataDir:           tempDir,
		UploadDir:         filepath.Join(tempDir, "uploads"),
		DBPath:            filepath.Join(tempDir, "app.db"),
		AdminPasswordHash: adminHash,
		CookieSecret:      "test-secret",
		SessionTTL:        24 * time.Hour,
	}

	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}

	repo, err := repository.New(cfg.DBPath)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	svc := New(repo, cfg)
	return svc, func() {
		repo.Close()
	}
}
