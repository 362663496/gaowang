package services

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BackupService struct {
	DatabaseURL       string
	BackupDir         string
	AttachmentLimitMB int
}

type MailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       string
	TLSMode  string
}

func ShouldAttachBackup(fileSize int64, limitMB int) bool {
	return fileSize <= int64(limitMB)*1024*1024
}

func backupFilename(stamp string) string {
	return fmt.Sprintf("gaowang-%s.sql.gz", stamp)
}

func (s BackupService) Run(ctx context.Context) (string, int64, error) {
	if err := os.MkdirAll(s.BackupDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create backup dir: %w", err)
	}
	stamp := time.Now().Format("20060102-150405.000000000")
	sqlPath := filepath.Join(s.BackupDir, fmt.Sprintf("gaowang-%s.sql", stamp))
	gzPath := filepath.Join(s.BackupDir, backupFilename(stamp))

	cmd := exec.CommandContext(ctx, "pg_dump", pgDumpDatabaseURL(s.DatabaseURL), "-f", sqlPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("pg_dump failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if err := gzipFile(sqlPath, gzPath); err != nil {
		_ = os.Remove(gzPath)
		return "", 0, err
	}
	if err := os.Remove(sqlPath); err != nil {
		return "", 0, fmt.Errorf("remove plain sql backup: %w", err)
	}

	info, err := os.Stat(gzPath)
	if err != nil {
		return "", 0, fmt.Errorf("stat backup: %w", err)
	}
	return gzPath, info.Size(), nil
}

func gzipFile(src string, dst string) (err error) {
	input, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open sql backup: %w", err)
	}
	defer func() { err = closeWithError(err, input) }()

	output, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create gzip backup: %w", err)
	}
	defer func() { err = closeWithError(err, output) }()

	writer := gzip.NewWriter(output)
	defer func() { err = closeWithError(err, writer) }()
	if _, err := io.Copy(writer, input); err != nil {
		return fmt.Errorf("gzip backup: %w", err)
	}
	return nil
}

func pgDumpDatabaseURL(databaseURL string) string {
	fields := strings.Fields(databaseURL)
	filtered := fields[:0]
	for _, field := range fields {
		key, _, _ := strings.Cut(field, "=")
		if strings.EqualFold(key, "timezone") {
			continue
		}
		filtered = append(filtered, field)
	}
	return strings.Join(filtered, " ")
}

func SendBackupMail(ctx context.Context, cfg MailConfig, filePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read backup attachment: %w", err)
	}
	message, err := backupMailMessage(cfg, filepath.Base(filePath), data)
	if err != nil {
		return err
	}
	return sendMailMessage(ctx, cfg, message)
}

func SendBackupNoticeMail(ctx context.Context, cfg MailConfig, filePath string, fileSize int64) error {
	body := fmt.Sprintf("Database backup was created locally but was too large to attach.\n\nFile: %s\nSize: %d bytes", filePath, fileSize)
	return sendMailMessage(ctx, cfg, simpleMailMessage(cfg, "Gaowang database backup created", body))
}

func sendMailMessage(ctx context.Context, cfg MailConfig, message []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := smtpClient(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(cfg.To); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := wc.Write(message); err != nil {
		_ = wc.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close data: %w", err)
	}
	return client.Quit()
}

func backupMailMessage(cfg MailConfig, filename string, data []byte) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")
	textPart, err := writer.CreatePart(textHeader)
	if err != nil {
		return nil, fmt.Errorf("create text part: %w", err)
	}
	if _, err := textPart.Write([]byte("Database backup is attached.")); err != nil {
		return nil, fmt.Errorf("write mail body: %w", err)
	}
	fileHeader := textproto.MIMEHeader{}
	fileHeader.Set("Content-Type", "application/gzip")
	fileHeader.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	fileHeader.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, fmt.Errorf("create attachment: %w", err)
	}
	encoder := base64.NewEncoder(base64.StdEncoding, part)
	if _, err := encoder.Write(data); err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("encode attachment: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close attachment encoder: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close mail writer: %w", err)
	}

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Gaowang database backup\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%s\r\n\r\n", cfg.From, cfg.To, writer.Boundary())
	return append([]byte(headers), body.Bytes()...), nil
}

func simpleMailMessage(cfg MailConfig, subject string, body string) []byte {
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n", cfg.From, cfg.To, subject)
	return []byte(headers + body)
}

func smtpClient(cfg MailConfig) (*smtp.Client, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	dialer := net.Dialer{Timeout: 15 * time.Second}
	if cfg.TLSMode == "smtps" {
		conn, err := tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial: %w", err)
		}
		return smtp.NewClient(conn, cfg.Host)
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smtp client: %w", err)
	}
	if cfg.TLSMode == "starttls" {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	}
	return client, nil
}

func closeWithError(current error, closer io.Closer) error {
	if err := closer.Close(); err != nil && current == nil {
		return err
	}
	return current
}
