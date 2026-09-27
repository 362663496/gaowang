package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	BackupEmailRecipientKey   = "backup.email_recipient"
	BackupScheduleEnabledKey  = "backup.schedule_enabled"
	BackupScheduleTimeKey     = "backup.schedule_time"
	BackupScheduleDefaultTime = "02:00"
	BackupTriggerManual       = "manual"
	BackupTriggerScheduled    = "scheduled"
	backupRunningWindow       = 15 * time.Minute
	backupScheduleTick        = 30 * time.Second
	backupScheduleTimezone    = "Asia/Shanghai"
)

var (
	ErrBackupInProgress    = errors.New("backup already running")
	backupRunMu            sync.Mutex
	backupTimePattern      = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
	backupScheduleLocation = time.FixedZone(backupScheduleTimezone, 8*60*60)
	dumpDatabase           = func(ctx context.Context, svc BackupService) (string, int64, error) {
		return svc.Run(ctx)
	}
)

type BackupRunInput struct {
	DatabaseURL       string
	BackupDir         string
	AttachmentLimitMB int
	Mail              MailConfig
	Trigger           string
	ScheduleSlot      *string
}

type BackupSchedule struct {
	Enabled   bool       `json:"enabled"`
	Time      string     `json:"time"`
	Timezone  string     `json:"timezone"`
	NextRunAt *time.Time `json:"next_run_at"`
	Due       bool       `json:"due"`
}

type BackupSchedulerConfig struct {
	DatabaseURL       string
	BackupDir         string
	AttachmentLimitMB int
	Mail              MailConfig
	RecipientFallback string
}

func BackupRecipient(db *gorm.DB, fallback string) string {
	var setting models.Setting
	if db != nil && db.First(&setting, "key = ?", BackupEmailRecipientKey).Error == nil {
		if value := strings.TrimSpace(setting.Value); value != "" {
			return value
		}
	}
	return strings.TrimSpace(fallback)
}

func ValidateBackupTime(value string) error {
	if !backupTimePattern.MatchString(strings.TrimSpace(value)) {
		return errors.New("备份时间格式应为 HH:MM")
	}
	return nil
}

func SaveBackupSchedule(db *gorm.DB, enabled bool, timeText string) error {
	timeText = strings.TrimSpace(timeText)
	if err := ValidateBackupTime(timeText); err != nil {
		return err
	}
	enabledValue := "false"
	if enabled {
		enabledValue = "true"
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&models.Setting{Key: BackupScheduleEnabledKey, Value: enabledValue}).Error; err != nil {
			return fmt.Errorf("save backup schedule enabled: %w", err)
		}
		if err := tx.Save(&models.Setting{Key: BackupScheduleTimeKey, Value: timeText}).Error; err != nil {
			return fmt.Errorf("save backup schedule time: %w", err)
		}
		return nil
	})
}

func LoadBackupSchedule(db *gorm.DB, now time.Time) (BackupSchedule, error) {
	timeText := strings.TrimSpace(settingValue(db, BackupScheduleTimeKey))
	if timeText == "" {
		timeText = BackupScheduleDefaultTime
	}
	schedule := BackupSchedule{
		Enabled:  settingValue(db, BackupScheduleEnabledKey) == "true",
		Time:     timeText,
		Timezone: backupScheduleTimezone,
	}
	slot, slotTime, reached, ok := backupSlot(now, timeText)
	if !ok || !schedule.Enabled {
		return schedule, nil
	}
	claimed, err := backupSlotClaimed(db, slot)
	if err != nil {
		return BackupSchedule{}, err
	}
	if reached && !claimed {
		schedule.Due = true
		schedule.NextRunAt = &slotTime
		return schedule, nil
	}
	next := slotTime
	if reached {
		next = slotTime.AddDate(0, 0, 1)
	}
	schedule.NextRunAt = &next
	return schedule, nil
}

func RunBackupJob(ctx context.Context, db *gorm.DB, input BackupRunInput) (models.BackupJob, error) {
	if !backupRunMu.TryLock() {
		return models.BackupJob{}, ErrBackupInProgress
	}
	defer backupRunMu.Unlock()
	now := time.Now()
	if err := releaseStaleBackupJobs(db, now); err != nil {
		return models.BackupJob{}, err
	}
	busy, err := backupInProgress(db, now)
	if err != nil {
		return models.BackupJob{}, err
	}
	if busy {
		return models.BackupJob{}, ErrBackupInProgress
	}
	return executeBackup(ctx, db, input, now)
}

func RunBackupScheduler(ctx context.Context, db *gorm.DB, cfg BackupSchedulerConfig) {
	ticker := time.NewTicker(backupScheduleTick)
	defer ticker.Stop()
	tick := func() {
		if err := TickScheduledBackup(ctx, db, cfg, time.Now()); err != nil {
			slog.Error("scheduled backup", slog.Any("err", err))
		}
	}
	tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

func TickScheduledBackup(ctx context.Context, db *gorm.DB, cfg BackupSchedulerConfig, now time.Time) error {
	if !backupRunMu.TryLock() {
		return nil
	}
	defer backupRunMu.Unlock()
	if err := releaseStaleBackupJobs(db, now); err != nil {
		return err
	}
	busy, err := backupInProgress(db, now)
	if err != nil {
		return err
	}
	if busy {
		return nil
	}
	slot, due, err := dueBackupSlot(db, now)
	if err != nil || !due {
		return err
	}
	input := cfg.runInput(db)
	input.ScheduleSlot = &slot
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	job, err := executeBackup(runCtx, db, input, now)
	if isUniqueViolation(err) {
		return nil
	}
	recordScheduledBackupAudit(db, job, err)
	return err
}

func (cfg BackupSchedulerConfig) runInput(db *gorm.DB) BackupRunInput {
	mail := cfg.Mail
	mail.To = BackupRecipient(db, cfg.RecipientFallback)
	return BackupRunInput{
		DatabaseURL:       cfg.DatabaseURL,
		BackupDir:         cfg.BackupDir,
		AttachmentLimitMB: cfg.AttachmentLimitMB,
		Mail:              mail,
		Trigger:           BackupTriggerScheduled,
	}
}

func executeBackup(ctx context.Context, db *gorm.DB, input BackupRunInput, now time.Time) (models.BackupJob, error) {
	if input.Trigger == "" {
		input.Trigger = BackupTriggerManual
	}
	started := now
	if started.IsZero() {
		started = time.Now()
	}
	job := models.BackupJob{
		StartedAt:    started,
		Status:       models.BackupStatusRunning,
		Recipient:    input.Mail.To,
		Trigger:      input.Trigger,
		ScheduleSlot: input.ScheduleSlot,
	}
	if err := db.Create(&job).Error; err != nil {
		if isUniqueViolation(err) {
			return models.BackupJob{}, err
		}
		return models.BackupJob{}, fmt.Errorf("create backup job: %w", err)
	}
	path, size, err := dumpDatabase(ctx, BackupService{
		DatabaseURL: input.DatabaseURL, BackupDir: input.BackupDir, AttachmentLimitMB: input.AttachmentLimitMB,
	})
	finished := time.Now()
	job.FinishedAt = &finished
	job.FilePath = path
	job.FileSize = size
	if err != nil {
		job.Status = models.BackupStatusFailed
		job.ErrorMessage = err.Error()
		_ = db.Save(&job).Error
		return job, err
	}
	job.Status = models.BackupStatusSuccess
	applyBackupMail(ctx, &job, input.Mail, input.AttachmentLimitMB)
	_ = db.Save(&job).Error
	return job, nil
}

func applyBackupMail(ctx context.Context, job *models.BackupJob, mail MailConfig, attachmentLimitMB int) {
	if mail.Host == "" || mail.To == "" || mail.From == "" {
		job.EmailStatus = "not_configured"
		return
	}
	if !ShouldAttachBackup(job.FileSize, attachmentLimitMB) {
		if err := SendBackupNoticeMail(ctx, mail, job.FilePath, job.FileSize); err != nil {
			job.EmailStatus = "failed"
			job.ErrorMessage = err.Error()
			return
		}
		job.EmailStatus = "sent_without_attachment"
		return
	}
	if err := SendBackupMail(ctx, mail, job.FilePath); err != nil {
		job.EmailStatus = "failed"
		job.ErrorMessage = err.Error()
		return
	}
	job.EmailStatus = "sent"
}

func dueBackupSlot(db *gorm.DB, now time.Time) (string, bool, error) {
	schedule, err := LoadBackupSchedule(db, now)
	if err != nil || !schedule.Due {
		return "", false, err
	}
	slot, _, _, ok := backupSlot(now, schedule.Time)
	if !ok {
		return "", false, nil
	}
	return slot, true, nil
}

func backupSlot(now time.Time, timeText string) (string, time.Time, bool, bool) {
	if err := ValidateBackupTime(timeText); err != nil {
		return "", time.Time{}, false, false
	}
	clock, err := time.ParseInLocation("15:04", strings.TrimSpace(timeText), backupScheduleLocation)
	if err != nil {
		return "", time.Time{}, false, false
	}
	local := now.In(backupScheduleLocation)
	slotTime := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, backupScheduleLocation)
	return slotTime.Format("2006-01-02T15:04"), slotTime, !local.Before(slotTime), true
}

func backupSlotClaimed(db *gorm.DB, slot string) (bool, error) {
	if db == nil || slot == "" {
		return false, nil
	}
	var count int64
	err := db.Model(&models.BackupJob{}).Where("schedule_slot = ?", slot).Count(&count).Error
	return count > 0, err
}

func backupInProgress(db *gorm.DB, now time.Time) (bool, error) {
	if db == nil {
		return false, nil
	}
	var count int64
	err := db.Model(&models.BackupJob{}).
		Where("status = ? AND started_at > ?", models.BackupStatusRunning, now.Add(-backupRunningWindow)).
		Count(&count).Error
	return count > 0, err
}

func releaseStaleBackupJobs(db *gorm.DB, now time.Time) error {
	if db == nil {
		return nil
	}
	return db.Model(&models.BackupJob{}).
		Where("status = ? AND started_at <= ?", models.BackupStatusRunning, now.Add(-backupRunningWindow)).
		Updates(map[string]any{
			"status":        models.BackupStatusFailed,
			"finished_at":   now,
			"error_message": "备份进程中断",
		}).Error
}

func settingValue(db *gorm.DB, key string) string {
	if db == nil {
		return ""
	}
	var setting models.Setting
	if err := db.First(&setting, "key = ?", key).Error; err != nil {
		return ""
	}
	return setting.Value
}

func recordScheduledBackupAudit(db *gorm.DB, job models.BackupJob, runErr error) {
	if db == nil || job.ID == uuid.Nil {
		return
	}
	action := "backup.run_succeeded"
	if runErr != nil {
		action = "backup.run_failed"
	}
	metadata, err := json.Marshal(map[string]string{
		"status":       string(job.Status),
		"email_status": job.EmailStatus,
		"trigger":      BackupTriggerScheduled,
	})
	if err != nil {
		return
	}
	_ = db.Create(&models.AuditLog{
		Action:       action,
		ResourceType: "backup",
		ResourceID:   job.ID.String(),
		Metadata:     datatypes.JSON(metadata),
	}).Error
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
