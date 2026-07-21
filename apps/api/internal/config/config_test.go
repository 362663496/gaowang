package config

import (
	"errors"
	"strings"
	"testing"
)

func Test_Load_returns_defaults_when_optional_env_missing(t *testing.T) {
	// Given
	t.Setenv("DATABASE_URL", "host=localhost user=gaowang dbname=gaowang sslmode=disable")
	t.Setenv("AUTH_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("API_ADDR", "")
	t.Setenv("BACKUP_RETENTION_DAYS", "")
	t.Setenv("BACKUP_ATTACHMENT_LIMIT_MB", "")
	t.Setenv("SMTP_PORT", "")

	// When
	cfg, err := Load()

	// Then
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIAddr != ":8080" {
		t.Fatalf("APIAddr = %q, want %q", cfg.APIAddr, ":8080")
	}
	if cfg.BackupRetentionDays != 7 {
		t.Fatalf("BackupRetentionDays = %d, want 7", cfg.BackupRetentionDays)
	}
	if cfg.BackupAttachmentLimitMB != 20 {
		t.Fatalf("BackupAttachmentLimitMB = %d, want 20", cfg.BackupAttachmentLimitMB)
	}
	if cfg.SMTPPort != 587 {
		t.Fatalf("SMTPPort = %d, want 587", cfg.SMTPPort)
	}
}

func Test_Load_returns_error_when_database_url_missing(t *testing.T) {
	// Given
	t.Setenv("DATABASE_URL", "")

	// When
	_, err := Load()

	// Then
	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("Load() error = %v, want %v", err, ErrMissingDatabaseURL)
	}
}

func Test_Load_returns_error_when_integer_env_invalid(t *testing.T) {
	// Given
	t.Setenv("DATABASE_URL", "host=localhost user=gaowang dbname=gaowang sslmode=disable")
	t.Setenv("AUTH_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("BACKUP_RETENTION_DAYS", "seven")

	// When
	_, err := Load()

	// Then
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "BACKUP_RETENTION_DAYS must be an integer") {
		t.Fatalf("Load() error = %v, want BACKUP_RETENTION_DAYS integer error", err)
	}
}

func Test_Load_returns_error_when_auth_secret_too_short(t *testing.T) {
	// Given
	t.Setenv("DATABASE_URL", "host=localhost user=gaowang dbname=gaowang sslmode=disable")
	t.Setenv("AUTH_SECRET", "too-short")

	// When
	_, err := Load()

	// Then
	if !errors.Is(err, ErrAuthSecretTooShort) {
		t.Fatalf("Load() error = %v, want %v", err, ErrAuthSecretTooShort)
	}
}

func Test_Load_returns_error_when_auth_secret_missing(t *testing.T) {
	// Given
	t.Setenv("DATABASE_URL", "host=localhost user=gaowang dbname=gaowang sslmode=disable")
	t.Setenv("AUTH_SECRET", "")

	// When
	_, err := Load()

	// Then
	if !errors.Is(err, ErrAuthSecretTooShort) {
		t.Fatalf("Load() error = %v, want %v", err, ErrAuthSecretTooShort)
	}
}

func Test_Load_reads_session_and_initial_admin_fields(t *testing.T) {
	t.Setenv("DATABASE_URL", "host=localhost user=gaowang dbname=gaowang sslmode=disable")
	t.Setenv("AUTH_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("INITIAL_ADMIN_NAME", "Root")
	t.Setenv("INITIAL_ADMIN_EMAIL", "root@example.com")
	t.Setenv("INITIAL_ADMIN_PASSWORD", "password123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.SessionCookieSecure {
		t.Fatal("SessionCookieSecure = false, want true")
	}
	if cfg.InitialAdminName != "Root" || cfg.InitialAdminEmail != "root@example.com" || cfg.InitialAdminPassword != "password123" {
		t.Fatalf("initial admin fields = %+v", cfg)
	}
}

func Test_Load_returns_error_when_session_cookie_secure_invalid(t *testing.T) {
	t.Setenv("DATABASE_URL", "host=localhost user=gaowang dbname=gaowang sslmode=disable")
	t.Setenv("AUTH_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("SESSION_COOKIE_SECURE", "maybe")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SESSION_COOKIE_SECURE must be a boolean") {
		t.Fatalf("Load() error = %v, want SESSION_COOKIE_SECURE boolean error", err)
	}
}
