package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mgfilebox/internal/models"
	"mgfilebox/internal/repository"
)

func TestDeleteShareWithLockedFileCanRetry(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now()
	share := models.Share{
		ID: "locked-delete", Title: "删除重试", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		Files: []models.ShareFile{
			{ID: "first", FileName: "first.txt", StoragePath: filepath.Join(svc.cfg.UploadDir, "first.txt"), CreatedAt: now},
			{ID: "locked", FileName: "locked.txt", StoragePath: filepath.Join(svc.cfg.UploadDir, "locked.txt"), CreatedAt: now.Add(time.Second)},
			{ID: "missing", FileName: "missing.txt", StoragePath: filepath.Join(svc.cfg.UploadDir, "missing.txt"), CreatedAt: now.Add(2 * time.Second)},
		},
	}
	for _, file := range share.Files[:2] {
		if err := os.WriteFile(file.StoragePath, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.repo.CreateShare(ctx, share); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeDeletedShare(ctx, share.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("purge must refuse active share: %v", err)
	}
	locked, err := os.Open(share.Files[1].StoragePath)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	var deletionError *FileDeletionError
	if err := svc.DeleteShare(ctx, share.ID); !errors.As(err, &deletionError) || len(deletionError.Failures) != 1 {
		t.Fatalf("expected one locked file failure, got %v", err)
	}
	stored, err := svc.GetShare(ctx, share.ID)
	if err != nil || stored.DeletedAt != nil {
		t.Fatalf("failed deletion must retain original record: %v, %v", stored, err)
	}
	if _, err := os.Stat(share.Files[0].StoragePath); !os.IsNotExist(err) {
		t.Fatalf("unlocked file should have been removed: %v", err)
	}
	if content, err := os.ReadFile(share.Files[1].StoragePath); err != nil || string(content) != "keep" {
		t.Fatalf("locked file should remain intact: %q, %v", content, err)
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteShare(ctx, share.ID); err != nil {
		t.Fatalf("retry after releasing lock: %v", err)
	}
	stored, err = svc.GetShare(ctx, share.ID)
	if err != nil || stored.DeletedAt == nil {
		t.Fatalf("successful retry should mark deleted: %v, %v", stored, err)
	}
	for _, file := range share.Files {
		if _, err := os.Stat(file.StoragePath); !os.IsNotExist(err) {
			t.Fatalf("file %s remains after successful delete: %v", file.FileName, err)
		}
	}
}
