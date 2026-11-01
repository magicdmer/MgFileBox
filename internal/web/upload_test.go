package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mgfilebox/internal/config"
	"mgfilebox/internal/models"
	"mgfilebox/internal/repository"
	"mgfilebox/internal/security"
	"mgfilebox/internal/service"
)

func TestUploadTotalSizeLimitAndTemporaryCleanup(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	dir := t.TempDir()
	tempDir := filepath.Join(dir, "multipart")
	uploadDir := filepath.Join(dir, "uploads")
	for _, path := range []string{tempDir, uploadDir} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	repo, err := repository.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	cfg := &config.Config{UploadDir: uploadDir, CookieSecret: "test-secret", MaxUploadSize: 16}
	svc := service.New(repo, cfg)
	server, err := NewServer(cfg, svc, "test")
	if err != nil {
		t.Fatal(err)
	}
	server.engine.MaxMultipartMemory = 8 // Force disk-backed multipart files even at the tiny test limit.
	if err := repo.CreateAdminSession(context.Background(), models.AdminSession{ID: "upload-session", TokenHash: security.SHA256Hex("token"), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	pageRequest := httptest.NewRequest(http.MethodGet, "/upload", nil)
	pageRequest.AddCookie(&http.Cookie{Name: adminCookieName, Value: "token"})
	pageResponse := httptest.NewRecorder()
	server.engine.ServeHTTP(pageResponse, pageRequest)
	if pageResponse.Code != http.StatusOK || !strings.Contains(pageResponse.Body.String(), `data-max-upload-size="16"`) || !strings.Contains(pageResponse.Body.String(), `title="上传文件总大小上限：16 B">上限 16 B</span>`) {
		t.Fatal("upload page does not expose the running server's size limit for client validation")
	}
	for _, tc := range []struct {
		name    string
		sizes   []int
		chunked bool
		status  int
	}{
		{"exact single-file limit", []int{16}, false, http.StatusOK},
		{"single-file excess", []int{17}, false, http.StatusRequestEntityTooLarge},
		{"exact multi-file limit", []int{8, 8}, false, http.StatusOK},
		{"combined excess", []int{9, 8}, false, http.StatusRequestEntityTooLarge},
		{"chunked excess", []int{17}, true, http.StatusRequestEntityTooLarge},
		{"chunked body cap", []int{(1 << 20) + 17}, true, http.StatusRequestEntityTooLarge},
		{"known-length body cap", []int{(1 << 20) + 17}, false, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			beforeFiles, err := os.ReadDir(uploadDir)
			if err != nil {
				t.Fatal(err)
			}
			beforeShares, err := svc.ListShares(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			if err := writer.WriteField("expires_hours", "24"); err != nil {
				t.Fatal(err)
			}
			for _, size := range tc.sizes {
				part, err := writer.CreateFormFile("files", "test.txt")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := part.Write(bytes.Repeat([]byte("a"), size)); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/shares/file", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "token"})
			if tc.chunked {
				req.ContentLength = -1
				req.TransferEncoding = []string{"chunked"}
			}
			response := httptest.NewRecorder()
			bodyLength := body.Len()
			server.engine.ServeHTTP(response, req)
			if !tc.chunked && int64(bodyLength) > cfg.MaxUploadSize+(1<<20) && body.Len() != bodyLength {
				t.Fatal("server read an obviously oversized body instead of rejecting its Content-Length")
			}
			if response.Code != tc.status {
				t.Fatalf("expected %d, got %d: %s", tc.status, response.Code, response.Body.String())
			}
			temporary, err := os.ReadDir(tempDir)
			if err != nil || len(temporary) != 0 {
				t.Fatalf("multipart temporary files remain: %v, %v", temporary, err)
			}
			if tc.status != http.StatusOK {
				afterFiles, err := os.ReadDir(uploadDir)
				if err != nil || len(afterFiles) != len(beforeFiles) {
					t.Fatalf("rejected upload left stored files: %v, %v", afterFiles, err)
				}
				afterShares, err := svc.ListShares(context.Background())
				if err != nil || len(afterShares) != len(beforeShares) {
					t.Fatalf("rejected upload created a record: %v, %v", afterShares, err)
				}
			}
		})
	}
}
