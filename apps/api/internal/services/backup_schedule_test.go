package services

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_BackupSchedule_is_due_once_per_shanghai_slot(t *testing.T) {
	db := newBackupScheduleDB(t)
	if err := SaveBackupSchedule(db, true, "02:00"); err != nil {
		t.Fatalf("save schedule: %v", err)
	}
	location := backupScheduleLocation
	before := time.Date(2026, 9, 27, 1, 59, 0, 0, location)
	schedule, err := LoadBackupSchedule(db, before)
	if err != nil || schedule.Due || schedule.NextRunAt == nil || !schedule.NextRunAt.Equal(time.Date(2026, 9, 27, 2, 0, 0, 0, location)) {
		t.Fatalf("before slot = %+v err=%v", schedule, err)
	}

	dueAt := time.Date(2026, 9, 27, 3, 0, 0, 0, location)
	schedule, err = LoadBackupSchedule(db, dueAt)
	if err != nil || !schedule.Due {
		t.Fatalf("after slot = %+v err=%v, want due", schedule, err)
	}

	calls := 0
	restoreDump(t, func(context.Context, BackupService) (string, int64, error) {
		calls++
		return filepath.Join(t.TempDir(), "backup.sql.gz"), 12, nil
	})
	if err := TickScheduledBackup(context.Background(), db, BackupSchedulerConfig{RecipientFallback: "ops@example.com"}, dueAt); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if err := TickScheduledBackup(context.Background(), db, BackupSchedulerConfig{RecipientFallback: "ops@example.com"}, dueAt.Add(time.Minute)); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if calls != 1 {
		t.Fatalf("dump calls = %d, want 1", calls)
	}
	var job models.BackupJob
	if err := db.Order("created_at desc").First(&job).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if job.Trigger != BackupTriggerScheduled || job.Status != models.BackupStatusSuccess || job.EmailStatus != "not_configured" || job.ScheduleSlot == nil || *job.ScheduleSlot != "2026-09-27T02:00" {
		t.Fatalf("job = %+v, want one successful scheduled slot", job)
	}
	var audits int64
	if err := db.Model(&models.AuditLog{}).Where("action = ?", "backup.run_succeeded").Count(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("scheduled audits = %d err=%v", audits, err)
	}

	if err := SaveBackupSchedule(db, true, "04:00"); err != nil {
		t.Fatalf("change time: %v", err)
	}
	later := time.Date(2026, 9, 27, 5, 0, 0, 0, location)
	if err := TickScheduledBackup(context.Background(), db, BackupSchedulerConfig{}, later); err != nil {
		t.Fatalf("changed slot tick: %v", err)
	}
	if calls != 2 {
		t.Fatalf("dump calls after time change = %d, want 2", calls)
	}
}

func Test_BackupSchedule_skips_disabled_busy_and_invalid_time(t *testing.T) {
	db := newBackupScheduleDB(t)
	now := time.Date(2026, 9, 27, 3, 0, 0, 0, backupScheduleLocation)
	if err := SaveBackupSchedule(db, false, "02:00"); err != nil {
		t.Fatalf("save disabled: %v", err)
	}
	calls := 0
	restoreDump(t, func(context.Context, BackupService) (string, int64, error) {
		calls++
		return "backup.sql.gz", 1, nil
	})
	if err := TickScheduledBackup(context.Background(), db, BackupSchedulerConfig{}, now); err != nil || calls != 0 {
		t.Fatalf("disabled tick calls=%d err=%v", calls, err)
	}
	if err := SaveBackupSchedule(db, true, "25:99"); err == nil {
		t.Fatal("invalid time was saved")
	}

	running := models.BackupJob{StartedAt: now.Add(-time.Minute), Status: models.BackupStatusRunning, Trigger: BackupTriggerManual}
	if err := db.Create(&running).Error; err != nil {
		t.Fatalf("create running job: %v", err)
	}
	if err := SaveBackupSchedule(db, true, "02:00"); err != nil {
		t.Fatalf("save enabled: %v", err)
	}
	if err := TickScheduledBackup(context.Background(), db, BackupSchedulerConfig{}, now); err != nil || calls != 0 {
		t.Fatalf("busy tick calls=%d err=%v", calls, err)
	}

	stale := models.BackupJob{StartedAt: now.Add(-backupRunningWindow - time.Minute), Status: models.BackupStatusRunning, Trigger: BackupTriggerManual}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("create stale job: %v", err)
	}
	if err := releaseStaleBackupJobs(db, now); err != nil {
		t.Fatalf("release stale: %v", err)
	}
	if err := db.First(&stale, "id = ?", stale.ID).Error; err != nil || stale.Status != models.BackupStatusFailed || stale.ErrorMessage != "备份进程中断" {
		t.Fatalf("stale job = %+v err=%v", stale, err)
	}
	if err := db.First(&running, "id = ?", running.ID).Error; err != nil || running.Status != models.BackupStatusRunning {
		t.Fatalf("recent job = %+v, want still running", running)
	}
}

func Test_RunBackupJob_rejects_overlap_and_records_manual_success(t *testing.T) {
	backupRunMu.Lock()
	_, err := RunBackupJob(context.Background(), nil, BackupRunInput{})
	backupRunMu.Unlock()
	if !errors.Is(err, ErrBackupInProgress) {
		t.Fatalf("overlap err = %v, want in progress", err)
	}

	db := newBackupScheduleDB(t)
	restoreDump(t, func(context.Context, BackupService) (string, int64, error) {
		return filepath.Join(t.TempDir(), "manual.sql.gz"), 8, nil
	})
	job, err := RunBackupJob(context.Background(), db, BackupRunInput{Mail: MailConfig{To: "ops@example.com"}, Trigger: BackupTriggerManual})
	if err != nil {
		t.Fatalf("manual backup: %v", err)
	}
	if job.Trigger != BackupTriggerManual || job.Status != models.BackupStatusSuccess || job.ScheduleSlot != nil || job.EmailStatus != "not_configured" {
		t.Fatalf("manual job = %+v", job)
	}
	other := models.BackupJob{StartedAt: time.Now(), Status: models.BackupStatusSuccess, Trigger: BackupTriggerManual}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("second manual job: %v", err)
	}
}

func newBackupScheduleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.BackupJob{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func restoreDump(t *testing.T, replacement func(context.Context, BackupService) (string, int64, error)) {
	t.Helper()
	previous := dumpDatabase
	dumpDatabase = replacement
	t.Cleanup(func() { dumpDatabase = previous })
}
