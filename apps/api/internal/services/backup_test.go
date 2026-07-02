package services

import "testing"

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
