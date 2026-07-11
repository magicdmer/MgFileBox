package config

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	Port              string
	BaseURL           string
	DataDir           string
	UploadDir         string
	DBPath            string
	AdminPasswordHash string
	CookieSecret      string
	SessionTTL        time.Duration
	MaxUploadSize     int64
	CleanupInterval   time.Duration
}

func Load() (*Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return nil, err
	}

	dataDir := getEnv("DATA_DIR", filepath.Join(".", "data"))
	uploadDir := getEnv("UPLOAD_DIR", filepath.Join(dataDir, "uploads"))
	dbPath := getEnv("DB_PATH", filepath.Join(dataDir, "app.db"))

	adminHash, err := resolveAdminPasswordHash()
	if err != nil {
		return nil, err
	}

	cookieSecret := getEnv("COOKIE_SECRET", "")
	if cookieSecret == "" {
		cookieSecret, err = randomHex(32)
		if err != nil {
			return nil, fmt.Errorf("generate cookie secret: %w", err)
		}
	}

	sessionTTLHours := getEnvAsInt("SESSION_TTL_HOURS", 24)
	cleanupMinutes := getEnvAsInt("CLEANUP_INTERVAL_MINUTES", 30)
	maxUploadSizeMB := getEnvAsInt("MAX_UPLOAD_SIZE_MB", 512)

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		BaseURL:           getEnv("APP_BASE_URL", "http://localhost:8080"),
		DataDir:           dataDir,
		UploadDir:         uploadDir,
		DBPath:            dbPath,
		AdminPasswordHash: adminHash,
		CookieSecret:      cookieSecret,
		SessionTTL:        time.Duration(sessionTTLHours) * time.Hour,
		MaxUploadSize:     int64(maxUploadSizeMB) * 1024 * 1024,
		CleanupInterval:   time.Duration(cleanupMinutes) * time.Minute,
	}

	for _, dir := range []string{cfg.DataDir, cfg.UploadDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return cfg, nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("parse %s:%d: missing =", path, lineNumber)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("parse %s:%d: empty key", path, lineNumber)
		}
		if os.Getenv(key) != "" {
			continue
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func resolveAdminPasswordHash() (string, error) {
	if value := os.Getenv("ADMIN_PASSWORD_HASH"); value != "" {
		return value, nil
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		return "", fmt.Errorf("missing ADMIN_PASSWORD or ADMIN_PASSWORD_HASH")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash admin password: %w", err)
	}
	return string(hash), nil
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
