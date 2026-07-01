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
	stamp := time.Now().Format("20060102-150405")
	sqlPath := filepath.Join(s.BackupDir, fmt.Sprintf("gaowang-%s.sql", stamp))
	gzPath := filepath.Join(s.BackupDir, backupFilename(stamp))

	cmd := exec.CommandContext(ctx, "pg_dump", s.DatabaseURL, "-f", sqlPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("pg_dump failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if err := gzipFile(sqlPath, gzPath); err != nil {
		return "", 0, err
	}
	_ = os.Remove(sqlPath)

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

	output, err := os.Create(dst)
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

func smtpClient(cfg MailConfig) (*smtp.Client, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	if cfg.TLSMode == "smtps" {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial: %w", err)
		}
		return smtp.NewClient(conn, cfg.Host)
	}
	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
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
