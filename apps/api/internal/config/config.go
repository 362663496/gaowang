package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const minAuthSecretBytes = 32

var (
	ErrMissingDatabaseURL = errors.New("DATABASE_URL is required")
	ErrAuthSecretTooShort = errors.New("AUTH_SECRET must be at least 32 bytes")
)

type Config struct {
	APIAddr                 string
	DatabaseURL             string
	AuthSecret              string
	UploadDir               string
	BackupDir               string
	BackupRetentionDays     int
	BackupAttachmentLimitMB int
	SMTPHost                string
	SMTPPort                int
	SMTPUsername            string
	SMTPPassword            string
	SMTPFrom                string
	SMTPTo                  string
	SMTPTLS                 string
	InitialAdminName        string
	InitialAdminEmail       string
	InitialAdminPassword    string
	SessionCookieSecure     bool
}

func Load() (Config, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, ErrMissingDatabaseURL
	}

	backupRetentionDays, err := envInt("BACKUP_RETENTION_DAYS", 7)
	if err != nil {
		return Config{}, err
	}

	backupAttachmentLimitMB, err := envInt("BACKUP_ATTACHMENT_LIMIT_MB", 20)
	if err != nil {
		return Config{}, err
	}

	smtpPort, err := envInt("SMTP_PORT", 587)
	if err != nil {
		return Config{}, err
	}

	sessionCookieSecure, err := envBool("SESSION_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		APIAddr:                 envString("API_ADDR", ":8080"),
		DatabaseURL:             databaseURL,
		AuthSecret:              envString("AUTH_SECRET", ""),
		UploadDir:               envString("UPLOAD_DIR", "/app/uploads"),
		BackupDir:               envString("BACKUP_DIR", "/app/backups"),
		BackupRetentionDays:     backupRetentionDays,
		BackupAttachmentLimitMB: backupAttachmentLimitMB,
		SMTPHost:                envString("SMTP_HOST", "smtp.example.com"),
		SMTPPort:                smtpPort,
		SMTPUsername:            envString("SMTP_USERNAME", "backup@example.com"),
		SMTPPassword:            envString("SMTP_PASSWORD", "example-smtp-password"),
		SMTPFrom:                envString("SMTP_FROM", "backup@example.com"),
		SMTPTo:                  envString("SMTP_TO", "owner@example.com"),
		SMTPTLS:                 envString("SMTP_TLS", "starttls"),
		InitialAdminName:        envString("INITIAL_ADMIN_NAME", ""),
		InitialAdminEmail:       envString("INITIAL_ADMIN_EMAIL", ""),
		InitialAdminPassword:    envString("INITIAL_ADMIN_PASSWORD", ""),
		SessionCookieSecure:     sessionCookieSecure,
	}

	if len([]byte(cfg.AuthSecret)) < minAuthSecretBytes {
		return Config{}, ErrAuthSecretTooShort
	}

	return cfg, nil
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func envBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return value, nil
}
