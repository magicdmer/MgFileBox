package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSetsMissingValues(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	if err := os.WriteFile(envPath, []byte("MG_TEST_DOTENV=value-from-file\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv("MG_TEST_DOTENV", "")

	if err := loadDotEnv(envPath); err != nil {
		t.Fatalf("load .env: %v", err)
	}

	if value := os.Getenv("MG_TEST_DOTENV"); value != "value-from-file" {
		t.Fatalf("unexpected env value: %q", value)
	}
}

func TestLoadDotEnvDoesNotOverrideExistingValues(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	if err := os.WriteFile(envPath, []byte("MG_TEST_DOTENV=from-file\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv("MG_TEST_DOTENV", "from-env")

	if err := loadDotEnv(envPath); err != nil {
		t.Fatalf("load .env: %v", err)
	}

	if value := os.Getenv("MG_TEST_DOTENV"); value != "from-env" {
		t.Fatalf("expected existing env to win, got %q", value)
	}
}

func TestLoadRejectsInvalidUploadLimits(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ADMIN_PASSWORD_HASH", "not-used-for-login-in-this-test")
	t.Setenv("COOKIE_SECRET", "test-secret")
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	t.Setenv("UPLOAD_DIR", filepath.Join(dir, "uploads"))
	t.Setenv("DB_PATH", filepath.Join(dir, "test.db"))
	for _, limit := range []string{"0", "-1", "9223372036854775807"} {
		t.Run(limit, func(t *testing.T) {
			t.Setenv("MAX_UPLOAD_SIZE_MB", limit)
			if _, err := Load(); err == nil {
				t.Fatalf("invalid upload limit %s accepted", limit)
			}
		})
	}
	t.Setenv("MAX_UPLOAD_SIZE_MB", "1")
	cfg, err := Load()
	if err != nil || cfg.MaxUploadSize != 1<<20 {
		t.Fatalf("valid upload limit rejected: %v, %v", cfg, err)
	}
}
