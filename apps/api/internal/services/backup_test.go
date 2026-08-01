package services

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"testing"
)

func Test_ShouldAttachBackup_obeys_size_limit(t *testing.T) {
	// Then
	if !ShouldAttachBackup(10*1024*1024, 20) {
		t.Fatal("10MB should fit a 20MB limit")
	}
	if ShouldAttachBackup(21*1024*1024, 20) {
		t.Fatal("21MB should not fit a 20MB limit")
	}
}

func Test_BackupFilename_includes_stamp(t *testing.T) {
	// When
	got := backupFilename("20260701-120000")

	// Then
	if got != "gaowang-20260701-120000.sql.gz" {
		t.Fatalf("filename = %q, want gaowang-20260701-120000.sql.gz", got)
	}
}

func Test_PgDumpDatabaseURL_removes_gorm_timezone_option(t *testing.T) {
	got := pgDumpDatabaseURL("host=127.0.0.1 user=gaowang dbname=gaowang TimeZone=Asia/Shanghai sslmode=disable")

	if got != "host=127.0.0.1 user=gaowang dbname=gaowang sslmode=disable" {
		t.Fatalf("database url = %q", got)
	}
}

func Test_BackupMailMessage_wraps_attachment_for_smtp(t *testing.T) {
	want := bytes.Repeat([]byte("backup-data"), 512)
	message, err := backupMailMessage(MailConfig{From: "from@example.com", To: "to@example.com"}, "backup.sql.gz", want)
	if err != nil {
		t.Fatalf("build backup mail: %v", err)
	}
	for _, line := range bytes.Split(message, []byte("\r\n")) {
		if len(line) > 998 {
			t.Fatalf("SMTP line length = %d, want <= 998", len(line))
		}
	}

	parsed, err := mail.ReadMessage(bytes.NewReader(message))
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("parse content type: media_type=%q err=%v", mediaType, err)
	}
	parts := multipart.NewReader(parsed.Body, params["boundary"])
	textPart, err := parts.NextPart()
	if err != nil {
		t.Fatalf("read text part: %v", err)
	}
	if _, err := io.Copy(io.Discard, textPart); err != nil {
		t.Fatalf("discard text part: %v", err)
	}
	attachment, err := parts.NextPart()
	if err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	encoded, err := io.ReadAll(attachment)
	if err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	for _, line := range bytes.Split(encoded, []byte("\r\n")) {
		if len(line) > 76 {
			t.Fatalf("base64 line length = %d, want <= 76", len(line))
		}
	}
	got, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(encoded)))
	if err != nil {
		t.Fatalf("decode attachment: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("decoded attachment differs from source")
	}
}
