package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestSharingEndpoints(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	dir := t.TempDir()
	repo, err := repository.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	cfg := &config.Config{UploadDir: dir, BaseURL: "http://localhost:8080", CookieSecret: "test-secret"}
	svc := service.New(repo, cfg)
	server, err := NewServer(cfg, svc, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.CreateAdminSession(ctx, models.AdminSession{ID: "session", TokenHash: security.SHA256Hex("token"), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("original file"), 0o644); err != nil {
		t.Fatal(err)
	}
	share := models.Share{ID: "old-link", Title: "文件", Description: "原说明", FileName: "test.txt", StoragePath: path, ExpiresHours: 6, CreatedAt: time.Now().Add(-24 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}
	if err := repo.CreateShare(ctx, share); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, values url.Values, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if admin {
			req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "token"})
		}
		response := httptest.NewRecorder()
		server.engine.ServeHTTP(response, req)
		return response
	}
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/settings"}, {http.MethodPost, "/cancel"}, {http.MethodPost, "/reshare"},
	} {
		if response := request(endpoint.method, "/api/admin/shares/old-link"+endpoint.path, nil, false); response.Code != http.StatusFound {
			t.Fatalf("endpoint allowed anonymous access: %s %d", endpoint.path, response.Code)
		}
	}
	settings := request(http.MethodGet, "/api/admin/shares/old-link/settings", nil, true)
	if settings.Code != http.StatusOK || settings.Header().Get("Cache-Control") != "no-store" || !strings.Contains(settings.Body.String(), `"expiresHours":6`) {
		t.Fatalf("settings response: %d %s", settings.Code, settings.Body.String())
	}
	if response := request(http.MethodPost, "/api/admin/shares/old-link/reshare", url.Values{"expires_hours": {"-1"}}, true); response.Code != http.StatusBadRequest {
		t.Fatal("invalid expiry accepted")
	}
	response := request(http.MethodPost, "/api/admin/shares/old-link/reshare", url.Values{"description": {"新说明"}, "access_password": {"new-password"}, "expires_hours": {"72"}}, true)
	var payload struct {
		Data models.ShareSummary `json:"data"`
	}
	if response.Code != http.StatusOK {
		t.Fatalf("reshare failed: %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(payload.Data.ShareURL)
	if err != nil {
		t.Fatal(err)
	}
	if link.Path == "/s/old-link" || payload.Data.ID != "old-link" {
		t.Fatalf("wrong new link: %s", payload.Data.ShareURL)
	}
	if response := request(http.MethodGet, "/s/old-link/download", nil, false); response.Code != http.StatusNotFound {
		t.Fatal("old download remains accessible")
	}
	if response := request(http.MethodGet, link.Path+"/download", nil, false); response.Code != http.StatusForbidden {
		t.Fatal("new share should require password")
	}
	unlock := request(http.MethodPost, link.Path+"/unlock", url.Values{"password": {"new-password"}}, false)
	if unlock.Code != http.StatusFound || unlock.Header().Get("Location") != link.Path {
		t.Fatalf("unlock did not redirect to new public link: %d %v", unlock.Code, unlock.Header())
	}
	req := httptest.NewRequest(http.MethodGet, link.Path+"/download", nil)
	for _, cookie := range unlock.Result().Cookies() {
		if cookie.Path != link.Path {
			t.Fatalf("wrong cookie scope: %s", cookie.Path)
		}
		req.AddCookie(cookie)
	}
	download := httptest.NewRecorder()
	server.engine.ServeHTTP(download, req)
	if download.Code != http.StatusOK || download.Body.String() != "original file" {
		t.Fatalf("download after unlock failed: %d %s", download.Code, download.Body.String())
	}
	if response := request(http.MethodPost, "/api/admin/shares/old-link/reshare", url.Values{"expires_hours": {"24"}}, true); response.Code != http.StatusConflict {
		t.Fatal("active share accepted duplicate reshare")
	}
	if response := request(http.MethodPost, "/api/admin/shares/old-link/cancel", nil, true); response.Code != http.StatusOK {
		t.Fatal("cancel failed")
	}
	if response := request(http.MethodGet, link.Path+"/download", nil, false); response.Code != http.StatusGone {
		t.Fatal("cancelled link still downloads")
	}
	if response := request(http.MethodGet, link.Path, nil, false); !strings.Contains(response.Body.String(), "这条分享已被取消") {
		t.Fatal("cancelled page status missing")
	}
	if response := request(http.MethodGet, "/api/admin/shares/old-link/download", nil, true); response.Code != http.StatusOK || response.Body.String() != "original file" {
		t.Fatal("cancellation affected administrator download")
	}
	if response := request(http.MethodPost, "/api/admin/shares/old-link/reshare", url.Values{"expires_hours": {"0"}, "description": {""}, "access_password": {""}}, true); response.Code != http.StatusOK {
		t.Fatalf("reshare cancelled file failed: %s", response.Body.String())
	}
	if response := request(http.MethodGet, link.Path+"/download", nil, false); response.Code != http.StatusNotFound {
		t.Fatal("cancelled link revived after reshare")
	}
}
