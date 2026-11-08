package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"

	"mgfilebox/internal/models"
	"mgfilebox/internal/repository"
)

func updateHeader(t *testing.T, name, content string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	form, err := multipart.NewReader(&body, writer.Boundary()).ReadForm(1024)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { form.RemoveAll() })
	return form.File["file"][0]
}

func TestReplaceSoftwareKeepsLinkAndSettings(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	input := models.ShareInput{Description: "安装说明", AccessPassword: "1234", ExpiresHours: 24}
	summary, err := svc.CreateFileShare(ctx, input, []*multipart.FileHeader{updateHeader(t, "app-v1.exe", "old binary")})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := svc.GetShare(ctx, summary.ID)
	cookie, _ := svc.VerifySharePassword(before, input.AccessPassword)
	oldDownload, _, err := svc.OpenShareFile(ctx, before.ID, before.Files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer oldDownload.Close()
	if err := svc.ReplaceShareFile(ctx, before.ID, before.Files[0].ID, updateHeader(t, "app-v2.exe", "new complete binary")); err != nil {
		t.Fatal(err)
	}
	after, err := svc.GetPublicShare(ctx, before.LinkID())
	if err != nil {
		t.Fatal(err)
	}
	if svc.toSummary(*after).ShareURL != summary.ShareURL || after.Files[0].ID != before.Files[0].ID ||
		after.Description != before.Description || !after.ExpiresAt.Equal(before.ExpiresAt) ||
		!after.CreatedAt.Equal(before.CreatedAt) || !svc.CanAccessShare(after, cookie) {
		t.Fatal("replacement changed link, identity, settings or existing unlock")
	}
	if after.Title != "app-v2.exe" || after.FileName != "app-v2.exe" || after.StoragePath != after.Files[0].StoragePath || after.Files[0].Size != 19 {
		t.Fatalf("metadata not updated: %+v", after)
	}
	if data, err := os.ReadFile(after.StoragePath); err != nil || string(data) != "new complete binary" {
		t.Fatalf("new binary: %q %v", data, err)
	}
	if data, err := io.ReadAll(oldDownload); err != nil || string(data) != "old binary" {
		t.Fatalf("in-flight download was damaged: %q %v", data, err)
	}
	oldDownload.Close()
	if err := svc.CleanupExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(before.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("old version was not cleaned: %v", err)
	}
	// A stale concurrent update must not overwrite the winning replacement.
	if err := svc.repo.ReplaceShareFile(ctx, before.Files[0], before.Files[0]); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("stale update accepted: %v", err)
	}
}

func TestReplacementFailuresAndInactiveShares(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	ctx := context.Background()
	summary, err := svc.CreateFileShare(ctx, models.ShareInput{Title: "我的软件", ExpiresHours: 24}, []*multipart.FileHeader{
		updateHeader(t, "app.exe", "one"), updateHeader(t, "readme.txt", "two"),
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := svc.GetShare(ctx, summary.ID)
	id := before.Files[1].ID
	svc.cfg.MaxUploadSize = 3
	if err := svc.ReplaceShareFile(ctx, before.ID, id, updateHeader(t, "readme.txt", "too large")); !errors.Is(err, ErrUploadTooLarge) {
		t.Fatalf("size limit: %v", err)
	}
	broken := updateHeader(t, "broken", "x")
	broken.Filename = "broken"
	// Force a write failure without altering either existing file.
	uploadDir := svc.cfg.UploadDir
	svc.cfg.UploadDir = filepath.Join(uploadDir, "missing", "directory")
	if err := svc.ReplaceShareFile(ctx, before.ID, id, broken); err == nil {
		t.Fatal("write failure accepted")
	}
	svc.cfg.UploadDir = uploadDir
	if err := svc.CancelShare(ctx, before.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplaceShareFile(ctx, before.ID, id, updateHeader(t, "new.txt", "new")); err != nil {
		t.Fatal(err)
	}
	after, _ := svc.GetShare(ctx, before.ID)
	if !after.Cancelled || after.LinkID() != before.LinkID() || after.Title != before.Title ||
		after.Files[0].StoragePath != before.Files[0].StoragePath || after.Files[1].FileName != "new.txt" {
		t.Fatalf("multi-file replacement changed unrelated data: %+v", after)
	}
	if err := svc.DeleteShare(ctx, before.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplaceShareFile(ctx, before.ID, id, updateHeader(t, "new.txt", "new")); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("deleted share accepted: %v", err)
	}
}
