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

func TestCancelAndReshareKeepsFilesAndReplacesLink(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now()
	svc.now = func() time.Time { return now }
	input := models.ShareInput{Title: "分享测试", Description: "原说明", AccessPassword: "same-password", ExpiresHours: 6}
	share, err := svc.buildShare(input, "original-link")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.txt", "two.txt"} {
		path := filepath.Join(svc.cfg.UploadDir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		share.Files = append(share.Files, models.ShareFile{ID: name, FileName: name, StoragePath: path, CreatedAt: now})
	}
	if err := svc.repo.CreateShare(ctx, share); err != nil {
		t.Fatal(err)
	}
	oldCookie, err := svc.VerifySharePassword(&share, input.AccessPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reshare(ctx, share.ID, input); !errors.Is(err, ErrShareActive) {
		t.Fatalf("active share should reject reshare: %v", err)
	}
	if err := svc.CancelShare(ctx, share.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.GetPublicShare(ctx, share.LinkID())
	if err != nil || cancelled.Status(now) != "cancelled" || svc.CanAccessShare(cancelled, oldCookie) {
		t.Fatalf("cancellation failed: %v, %v", cancelled, err)
	}
	settings, err := svc.GetShareSettings(ctx, share.ID)
	if err != nil || settings.Description != input.Description || settings.AccessPassword != input.AccessPassword || settings.ExpiresHours != 6 {
		t.Fatalf("settings not preserved: %+v, %v", settings, err)
	}
	input.Description = "新说明\n第二行"
	input.ExpiresHours = 72
	result, err := svc.Reshare(ctx, share.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := svc.GetShare(ctx, share.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.ID != share.ID || renewed.LinkID() == share.LinkID() || renewed.Status(now) != "active" || renewed.Description != input.Description || !renewed.ExpiresAt.Equal(now.Add(72*time.Hour)) {
		t.Fatalf("unexpected renewed share: %+v", renewed)
	}
	if result.ID != share.ID || result.FileCount != 2 || result.ShareURL == svc.toSummary(share).ShareURL {
		t.Fatalf("unexpected summary: %+v", result)
	}
	if _, err := svc.GetPublicShare(ctx, share.LinkID()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("old link should be gone: %v", err)
	}
	if svc.CanAccessShare(renewed, oldCookie) {
		t.Fatal("old unlock cookie still works")
	}
	newCookie, err := svc.VerifySharePassword(renewed, input.AccessPassword)
	if err != nil || !svc.CanAccessShare(renewed, newCookie) {
		t.Fatalf("new unlock failed: %v", err)
	}
	for i, file := range renewed.Files {
		if file.ID != share.Files[i].ID || file.StoragePath != share.Files[i].StoragePath {
			t.Fatal("reshare changed stored file identity")
		}
		if content, err := os.ReadFile(file.StoragePath); err != nil || string(content) != file.FileName {
			t.Fatalf("file changed: %q, %v", content, err)
		}
	}
	files, err := os.ReadDir(svc.cfg.UploadDir)
	if err != nil || len(files) != 2 {
		t.Fatalf("reshare duplicated files: %v, %v", files, err)
	}
	// Reopening the database must keep rotated links and cancelled state intact.
	if err := svc.repo.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := repository.New(svc.cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	svc.repo = reopened
	if _, err := svc.GetPublicShare(ctx, renewed.LinkID()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetPublicShare(ctx, share.LinkID()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("migration revived old link")
	}
	now = now.Add(73 * time.Hour)
	input.AccessPassword = ""
	input.ExpiresHours = 0
	if _, err := svc.Reshare(ctx, share.ID, input); err != nil {
		t.Fatalf("expired share should be shareable: %v", err)
	}
	forever, err := svc.GetShare(ctx, share.ID)
	if err != nil || !forever.NeverExpires() || forever.HasPassword() || !svc.CanAccessShare(forever, "") {
		t.Fatalf("password removal / never expiry failed: %v, %v", forever, err)
	}
	if _, err := svc.GetPublicShare(ctx, renewed.LinkID()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("previous generation is still accessible")
	}
	if err := svc.CancelShare(ctx, share.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(share.Files[0].StoragePath); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reshare(ctx, share.ID, input); err == nil {
		t.Fatal("reshare accepted a missing file")
	}
	unchanged, err := svc.GetShare(ctx, share.ID)
	if err != nil || unchanged.LinkID() != forever.LinkID() || !unchanged.Cancelled {
		t.Fatalf("failed reshare changed public link or state: %v, %v", unchanged, err)
	}
	if err := svc.DeleteShare(ctx, share.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reshare(ctx, share.ID, input); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("deleted share should not be reshareable: %v", err)
	}
	if err := svc.CancelShare(ctx, share.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("cancel accepted deleted share: %v", err)
	}
}
