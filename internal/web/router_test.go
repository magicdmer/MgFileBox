package web

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
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

func TestFileShareDescription(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	dir := t.TempDir()
	cfg := &config.Config{
		UploadDir:     dir,
		CookieSecret:  strings.Repeat("a", 32),
		BaseURL:       "http://localhost:8080",
		MaxUploadSize: 1024 * 1024,
	}
	repo, err := repository.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	svc := service.New(repo, cfg)
	server, err := NewServer(cfg, svc, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.CreateAdminSession(ctx, models.AdminSession{
		ID: "test-session", TokenHash: security.SHA256Hex("test-token"),
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("admin downloads", func(t *testing.T) {
		for _, id := range []string{"download-active", "download-expired", "download-deleted", "download-missing-file"} {
			expiresAt := time.Now().Add(time.Hour)
			if id == "download-expired" {
				expiresAt = time.Now().Add(-time.Hour)
			}
			share := models.Share{
				ID: id, Title: id, AccessPasswordHash: "locked", CreatedAt: time.Now(), ExpiresAt: expiresAt,
				Files: []models.ShareFile{
					{ID: id + "-first", FileName: "first.txt", StoragePath: filepath.Join(dir, id+"-first.txt"), Size: 5, CreatedAt: time.Now()},
					{ID: id + "-second", FileName: "second.txt", StoragePath: filepath.Join(dir, id+"-second.txt"), Size: 6, CreatedAt: time.Now().Add(time.Second)},
				},
			}
			if id != "download-missing-file" {
				for index, file := range share.Files {
					content := "first"
					if index == 1 {
						content = "second"
					}
					if err := os.WriteFile(file.StoragePath, []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := repo.CreateShare(ctx, share); err != nil {
				t.Fatal(err)
			}
			if id == "download-deleted" {
				if err := repo.SoftDeleteShare(ctx, id, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, tc := range []struct {
			name, path, token, content string
			status                     int
		}{
			{"no login", "/api/admin/shares/download-expired/download", "", "", http.StatusFound},
			{"invalid session", "/api/admin/shares/download-expired/download", "invalid", "", http.StatusFound},
			{"active protected", "/api/admin/shares/download-active/download", "test-token", "first", http.StatusOK},
			{"expired protected", "/api/admin/shares/download-expired/download", "test-token", "first", http.StatusOK},
			{"select second file", "/api/admin/shares/download-expired/download?file=download-expired-second", "test-token", "second", http.StatusOK},
			{"foreign file", "/api/admin/shares/download-expired/download?file=download-active-second", "test-token", "", http.StatusNotFound},
			{"deleted", "/api/admin/shares/download-deleted/download", "test-token", "", http.StatusGone},
			{"missing share", "/api/admin/shares/nonexistent/download", "test-token", "", http.StatusNotFound},
			{"missing disk file", "/api/admin/shares/download-missing-file/download", "test-token", "", http.StatusNotFound},
			{"public expired with admin cookie", "/s/download-expired/download", "test-token", "", http.StatusGone},
			{"public protected with admin cookie", "/s/download-active/download", "test-token", "", http.StatusForbidden},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, tc.path, nil)
				if tc.token != "" {
					req.AddCookie(&http.Cookie{Name: adminCookieName, Value: tc.token})
				}
				response := httptest.NewRecorder()
				server.engine.ServeHTTP(response, req)
				if response.Code != tc.status {
					t.Fatalf("expected %d, got %d: %s", tc.status, response.Code, response.Body.String())
				}
				if tc.content != "" && (response.Body.String() != tc.content || !strings.Contains(response.Header().Get("Content-Disposition"), "attachment")) {
					t.Fatalf("unexpected attachment: %q, %v", response.Body.String(), response.Header())
				}
			})
		}
		page := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/shares", nil)
		req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "test-token"})
		server.engine.ServeHTTP(page, req)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "/api/admin/shares/download-expired/download?file=download-expired-second") || strings.Contains(page.Body.String(), "/api/admin/shares/download-deleted/download") {
			t.Fatal("management page must show downloads for expired files and hide them for deleted files")
		}
	})

	t.Run("deletion failures and retries", func(t *testing.T) {
		for _, operation := range []string{"delete", "purge"} {
			t.Run(operation, func(t *testing.T) {
				id := "retry-" + operation
				now := time.Now()
				share := models.Share{ID: id, Title: id, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
				for _, name := range []string{"first.txt", "second.txt"} {
					blockedPath := filepath.Join(dir, id+"-"+name)
					if err := os.Mkdir(blockedPath, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(blockedPath, "blocker"), []byte("blocked"), 0o644); err != nil {
						t.Fatal(err)
					}
					share.Files = append(share.Files, models.ShareFile{ID: id + name, FileName: name, StoragePath: blockedPath, CreatedAt: now})
				}
				if err := repo.CreateShare(ctx, share); err != nil {
					t.Fatal(err)
				}
				if operation == "purge" {
					if err := repo.SoftDeleteShare(ctx, id, now); err != nil {
						t.Fatal(err)
					}
				}
				request := func() *httptest.ResponseRecorder {
					req := httptest.NewRequest(http.MethodPost, "/api/admin/shares/"+id+"/"+operation, nil)
					req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "test-token"})
					response := httptest.NewRecorder()
					server.engine.ServeHTTP(response, req)
					return response
				}
				response := request()
				var payload struct {
					Success bool   `json:"success"`
					Message string `json:"message"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if response.Code != http.StatusConflict || payload.Success || !strings.Contains(payload.Message, "first.txt") || !strings.Contains(payload.Message, "second.txt") || !strings.Contains(payload.Message, "重试") {
					t.Fatalf("expected actionable errors for every failed file: %d %s", response.Code, response.Body.String())
				}
				stored, err := svc.GetShare(ctx, id)
				if err != nil || stored.IsDeleted() != (operation == "purge") {
					t.Fatalf("failure changed record state: %v, %v", stored, err)
				}
				for _, file := range share.Files {
					if err := os.Remove(filepath.Join(file.StoragePath, "blocker")); err != nil {
						t.Fatal(err)
					}
				}
				response = request()
				if response.Code != http.StatusOK {
					t.Fatalf("retry failed: %d %s", response.Code, response.Body.String())
				}
				stored, err = svc.GetShare(ctx, id)
				if operation == "purge" {
					if err != repository.ErrNotFound {
						t.Fatalf("purged record remains: %v", err)
					}
				} else if err != nil || !stored.IsDeleted() {
					t.Fatalf("retry did not mark deleted: %v, %v", stored, err)
				}
				for _, file := range share.Files {
					if _, err := os.Stat(file.StoragePath); !os.IsNotExist(err) {
						t.Fatalf("residual file remains: %v", err)
					}
				}
			})
		}
	})

	t.Run("edit description", func(t *testing.T) {
		share := models.Share{ID: "edit-test", Title: "编辑说明", Description: "原说明", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
		if err := repo.CreateShare(ctx, share); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name, id, description string
			auth                  bool
			status                int
		}{
			{"requires login", share.ID, "未授权修改", false, http.StatusFound},
			{"update", share.ID, " 新说明\n<script>test</script> ", true, http.StatusOK},
			{"reject long input", share.ID, strings.Repeat("字", 2001), true, http.StatusBadRequest},
			{"clear", share.ID, " \n ", true, http.StatusOK},
			{"missing", "missing", "说明", true, http.StatusNotFound},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before, err := svc.GetShare(ctx, share.ID)
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(http.MethodPost, "/api/admin/shares/"+tc.id+"/description", strings.NewReader(url.Values{"description": {tc.description}}.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if tc.auth {
					req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "test-token"})
				}
				response := httptest.NewRecorder()
				server.engine.ServeHTTP(response, req)
				if response.Code != tc.status {
					t.Fatalf("expected %d, got %d: %s", tc.status, response.Code, response.Body.String())
				}
				stored, err := svc.GetShare(ctx, share.ID)
				if err != nil {
					t.Fatal(err)
				}
				want := before.Description
				if tc.status == http.StatusOK {
					want = strings.TrimSpace(tc.description)
				}
				if stored.Description != want {
					t.Fatalf("stored description = %q, want %q", stored.Description, want)
				}
				page := httptest.NewRecorder()
				server.engine.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/s/"+share.ID, nil))
				if want == "" && strings.Contains(page.Body.String(), "share-description-title") {
					t.Fatal("cleared description is still visible")
				}
				if tc.name == "update" && !strings.Contains(page.Body.String(), "新说明\n&lt;script&gt;test&lt;/script&gt;") {
					t.Fatal("share page did not reflect escaped updated description")
				}
			})
		}
		if err := svc.DeleteShare(ctx, share.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdateShareDescription(ctx, share.ID, "deleted"); err != repository.ErrNotFound {
			t.Fatalf("deleted share must not be editable: %v", err)
		}
	})

	for _, tc := range []struct {
		name, description, password string
		wantFailure                 bool
	}{
		{name: "multiline", description: "  使用说明\n<script>alert('test')</script>\n第二行  "},
		{name: "empty", description: " \n "},
		{name: "protected", description: "仅解锁后可见的说明", password: "secret"},
		{name: "too long", description: strings.Repeat("字", 2001), wantFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			for key, value := range map[string]string{
				"description": tc.description, "expires_hours": "24", "access_password": tc.password,
			} {
				if err := writer.WriteField(key, value); err != nil {
					t.Fatal(err)
				}
			}
			part, err := writer.CreateFormFile("files", "demo.txt")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte("hello")); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/shares/file", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "test-token"})
			response := httptest.NewRecorder()
			server.engine.ServeHTTP(response, req)
			if tc.wantFailure {
				if response.Code != http.StatusBadRequest {
					t.Fatalf("expected validation error, got %d: %s", response.Code, response.Body.String())
				}
				return
			}
			if response.Code != http.StatusOK {
				t.Fatalf("upload: %d %s", response.Code, response.Body.String())
			}
			var payload struct {
				Data models.ShareSummary `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSpace(tc.description)
			stored, err := svc.GetShare(ctx, payload.Data.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Description != want || payload.Data.Description != want {
				t.Fatalf("description did not survive upload and database read: %q", stored.Description)
			}
			if _, err := os.Stat(stored.Files[0].StoragePath); err != nil {
				t.Fatalf("uploaded file missing: %v", err)
			}
			page := httptest.NewRecorder()
			server.engine.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/s/"+stored.ID, nil))
			html := page.Body.String()
			if page.Code != http.StatusOK {
				t.Fatalf("share page: %d %s", page.Code, html)
			}
			if tc.password != "" {
				if strings.Contains(html, want) || strings.Contains(html, "share-description-title") {
					t.Fatal("locked share leaked description")
				}
				return
			}
			if want == "" {
				if strings.Contains(html, "share-description-title") {
					t.Fatal("empty description should not render a section")
				}
			} else if !strings.Contains(html, "使用说明\n&lt;script&gt;") || !strings.Contains(html, "第二行") || strings.Contains(html, "<script>alert(") {
				t.Fatal("description must preserve line breaks and escape HTML")
			}
		})
	}
}

func TestLoginLimiterLocksAfterFailures(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Date(2026, 7, 11, 22, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	key := "127.0.0.1"
	for index := 0; index < maxLoginFailures; index++ {
		if !limiter.allow(key) {
			t.Fatalf("attempt %d should be allowed before lockout", index+1)
		}
		limiter.recordFailure(key)
	}

	if limiter.allow(key) {
		t.Fatalf("expected key to be locked after failures")
	}

	now = now.Add(loginLockout)
	if !limiter.allow(key) {
		t.Fatalf("expected key to be allowed after lockout window")
	}
}

func TestLoginLimiterResetClearsFailures(t *testing.T) {
	limiter := newLoginLimiter()
	key := "127.0.0.1"

	for index := 0; index < maxLoginFailures-1; index++ {
		limiter.recordFailure(key)
	}
	limiter.reset(key)

	for index := 0; index < maxLoginFailures; index++ {
		if !limiter.allow(key) {
			t.Fatalf("attempt %d should be allowed after reset", index+1)
		}
		limiter.recordFailure(key)
	}
}
