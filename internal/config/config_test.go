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
