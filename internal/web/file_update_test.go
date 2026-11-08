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

func TestSoftwareUpdateEndpointAndDownloadVersions(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	dir := t.TempDir()
	repo, err := repository.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	cfg := &config.Config{UploadDir: dir, CookieSecret: "test", MaxUploadSize: 16}
	svc := service.New(repo, cfg)
	server, err := NewServer(cfg, svc, "test")
	if err != nil {
		t.Fatal(err)
	}
	server.engine.MaxMultipartMemory = 1
	ctx := context.Background()
	if err := repo.CreateAdminSession(ctx, models.AdminSession{ID: "session", TokenHash: security.SHA256Hex("token"), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "app-v1.exe")
	if err := os.WriteFile(path, []byte("old-binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	share := models.Share{ID: "software", PublicID: "public-link", Title: "app-v1.exe", FileName: "app-v1.exe", StoragePath: path, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.CreateShare(ctx, share); err != nil {
		t.Fatal(err)
	}
	run := func(req *http.Request, admin bool) *httptest.ResponseRecorder {
		if admin {
			req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "token"})
		}
		response := httptest.NewRecorder()
		server.engine.ServeHTTP(response, req)
		return response
	}
	endpoint := "/api/admin/shares/software/files/software-legacy/replace"
	if response := run(httptest.NewRequest("POST", endpoint, nil), false); response.Code != http.StatusFound {
		t.Fatalf("anonymous replacement allowed: %d", response.Code)
	}
	page := run(httptest.NewRequest("GET", "/admin/shares", nil), true)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `data-replace-file="software-legacy"`) || !strings.Contains(page.Body.String(), `data-max-upload-size="16"`) {
		t.Fatalf("missing update controls: %d", page.Code)
	}
	downloadURL := "/s/public-link/download?file=software-legacy"
	old := run(httptest.NewRequest("GET", downloadURL, nil), false)
	if old.Code != 200 || old.Body.String() != "old-binary" || old.Header().Get("ETag") == "" {
		t.Fatal("initial download failed")
	}
	update := func(contents []string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for _, content := range contents {
			part, err := writer.CreateFormFile("file", "app-v2.exe")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		writer.Close()
		req := httptest.NewRequest("POST", endpoint, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return run(req, true)
	}
	for _, contents := range [][]string{{strings.Repeat("x", 17)}, {"one", "two"}, {}} {
		response := update(contents)
		if response.Code < 400 {
			t.Fatalf("invalid update accepted: %d", response.Code)
		}
		unchanged := run(httptest.NewRequest("GET", downloadURL, nil), false)
		if unchanged.Body.String() != "old-binary" {
			t.Fatal("failed update changed download")
		}
	}
	if response := update([]string{"new-binary"}); response.Code != 200 {
		t.Fatalf("replacement failed: %d %s", response.Code, response.Body.String())
	}
	request := httptest.NewRequest("GET", downloadURL, nil)
	request.Header.Set("Range", "bytes=4-")
	request.Header.Set("If-Range", old.Header().Get("ETag"))
	current := run(request, false)
	if current.Code != 200 || current.Body.String() != "new-binary" || current.Header().Get("ETag") == old.Header().Get("ETag") ||
		current.Header().Get("Cache-Control") != "no-store" || !strings.Contains(current.Header().Get("Content-Disposition"), "app-v2.exe") {
		t.Fatalf("stale range mixed versions: %d %s %v", current.Code, current.Body.String(), current.Header())
	}
	request = httptest.NewRequest("GET", downloadURL, nil)
	request.Header.Set("Range", "bytes=4-")
	request.Header.Set("If-Range", current.Header().Get("ETag"))
	if response := run(request, false); response.Code != 206 || response.Body.String() != "binary" {
		t.Fatalf("same-version resume failed: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest("GET", downloadURL, nil)
	request.Header.Set("Range", "bytes=4-")
	request.Header.Set("If-Range", time.Now().UTC().Format(http.TimeFormat))
	if response := run(request, false); response.Code != 200 || response.Body.String() != "new-binary" {
		t.Fatal("date validator resumed across ambiguous versions")
	}
}
