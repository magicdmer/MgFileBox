package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"mgfilebox/internal/models"
)

func TestWriteUploadedFileRemovesPartialFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.txt")
	writeError := errors.New("simulated write interruption")
	source := io.MultiReader(strings.NewReader("partially written data"), iotest.ErrReader(writeError))
	if err := writeUploadedFile(source, path); !errors.Is(err, writeError) {
		t.Fatalf("expected original error, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("partial file was not removed: %v", err)
	}
	if err := writeUploadedFile(strings.NewReader("complete"), path); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "complete" {
		t.Fatalf("successful upload changed: %q, %v", content, err)
	}
}

func TestFailedUploadRollsBackEarlierFiles(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	makeForm := func(memory int64) *multipart.Form {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("files", "test.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("test contents")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		form, err := multipart.NewReader(&body, writer.Boundary()).ReadForm(memory)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { form.RemoveAll() })
		return form
	}
	first := makeForm(1024)
	second := makeForm(0)
	if err := second.RemoveAll(); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreateFileShare(context.Background(), models.ShareInput{ExpiresHours: 24}, []*multipart.FileHeader{first.File["files"][0], second.File["files"][0]})
	if err == nil {
		t.Fatal("expected unavailable source to fail")
	}
	files, err := os.ReadDir(svc.cfg.UploadDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("upload left residual files: %v, %v", files, err)
	}
	shares, err := svc.ListShares(context.Background())
	if err != nil || len(shares) != 0 {
		t.Fatalf("failed upload created a share: %v, %v", shares, err)
	}
}
