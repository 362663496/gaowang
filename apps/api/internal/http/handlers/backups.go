package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const backupRecipientSettingKey = services.BackupEmailRecipientKey

type BackupHandler struct {
	DB  *gorm.DB
	Cfg config.Config
}

type updateBackupScheduleRequest struct {
	Enabled *bool  `json:"enabled"`
	Time    string `json:"time"`
}

func (h BackupHandler) Run(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	job, err := services.RunBackupJob(ctx, h.DB, h.backupRunInput(services.BackupTriggerManual))
	if errors.Is(err, services.ErrBackupInProgress) {
		writeError(c, http.StatusConflict, "BACKUP_IN_PROGRESS", "已有备份正在执行")
		return
	}
	if err != nil {
		if job.ID != uuid.Nil {
			recordAudit(c, h.DB, "backup.run_failed", "backup", job.ID.String(), backupAuditMetadata(job, services.BackupTriggerManual))
		}
		writeError(c, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}
	recordAudit(c, h.DB, "backup.run_succeeded", "backup", job.ID.String(), backupAuditMetadata(job, services.BackupTriggerManual))
	c.JSON(http.StatusCreated, gin.H{"job": job})
}

func (h BackupHandler) Latest(c *gin.Context) {
	var job models.BackupJob
	if err := h.DB.Order("created_at desc").First(&job).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"job": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": job})
}

func (h BackupHandler) GetSchedule(c *gin.Context) {
	schedule, err := services.LoadBackupSchedule(h.DB, time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load backup schedule")
		return
	}
	c.JSON(http.StatusOK, gin.H{"schedule": schedule})
}

func (h BackupHandler) UpdateSchedule(c *gin.Context) {
	var req updateBackupScheduleRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(c, http.StatusBadRequest, "VALIDATION", "请指定是否启用定时备份")
		return
	}
	if err := services.ValidateBackupTime(req.Time); err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION", err.Error())
		return
	}
	if err := services.SaveBackupSchedule(h.DB, *req.Enabled, req.Time); err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to save backup schedule")
		return
	}
	recordAudit(c, h.DB, "backup.schedule_updated", "backup", services.BackupScheduleEnabledKey, map[string]string{
		"enabled": strconv.FormatBool(*req.Enabled),
		"time":    req.Time,
	})
	schedule, err := services.LoadBackupSchedule(h.DB, time.Now())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load backup schedule")
		return
	}
	c.JSON(http.StatusOK, gin.H{"schedule": schedule})
}

func (h BackupHandler) backupRunInput(trigger string) services.BackupRunInput {
	recipient := h.backupRecipient()
	return services.BackupRunInput{
		DatabaseURL:       h.Cfg.DatabaseURL,
		BackupDir:         h.Cfg.BackupDir,
		AttachmentLimitMB: h.Cfg.BackupAttachmentLimitMB,
		Mail: services.MailConfig{
			Host: h.Cfg.SMTPHost, Port: h.Cfg.SMTPPort, Username: h.Cfg.SMTPUsername, Password: h.Cfg.SMTPPassword,
			From: h.Cfg.SMTPFrom, To: recipient, TLSMode: h.Cfg.SMTPTLS,
		},
		Trigger: trigger,
	}
}

func (h BackupHandler) backupRecipient() string {
	return services.BackupRecipient(h.DB, h.Cfg.SMTPTo)
}

func backupAuditMetadata(job models.BackupJob, trigger string) map[string]string {
	return map[string]string{
		"status":       string(job.Status),
		"email_status": job.EmailStatus,
		"trigger":      trigger,
	}
}
