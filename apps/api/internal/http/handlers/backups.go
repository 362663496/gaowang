package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const backupRecipientSettingKey = "backup.email_recipient"

type BackupHandler struct {
	DB  *gorm.DB
	Cfg config.Config
}

func (h BackupHandler) Run(c *gin.Context) {
	started := time.Now()
	job := models.BackupJob{StartedAt: started, Status: models.BackupStatusRunning, Recipient: h.backupRecipient()}
	_ = h.DB.Create(&job).Error

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	path, size, err := services.BackupService{
		DatabaseURL: h.Cfg.DatabaseURL, BackupDir: h.Cfg.BackupDir, AttachmentLimitMB: h.Cfg.BackupAttachmentLimitMB,
	}.Run(ctx)

	finished := time.Now()
	job.FinishedAt = &finished
	job.FilePath = path
	job.FileSize = size
	if err != nil {
		job.Status = models.BackupStatusFailed
		job.ErrorMessage = err.Error()
		_ = h.DB.Save(&job).Error
		writeError(c, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
		return
	}
	job.Status = models.BackupStatusSuccess
	h.sendMail(ctx, &job)
	_ = h.DB.Save(&job).Error
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

func (h BackupHandler) sendMail(ctx context.Context, job *models.BackupJob) {
	if h.Cfg.SMTPHost == "" || job.Recipient == "" || h.Cfg.SMTPFrom == "" {
		job.EmailStatus = "not_configured"
		return
	}
	cfg := services.MailConfig{
		Host: h.Cfg.SMTPHost, Port: h.Cfg.SMTPPort, Username: h.Cfg.SMTPUsername, Password: h.Cfg.SMTPPassword,
		From: h.Cfg.SMTPFrom, To: job.Recipient, TLSMode: h.Cfg.SMTPTLS,
	}
	if !services.ShouldAttachBackup(job.FileSize, h.Cfg.BackupAttachmentLimitMB) {
		if err := services.SendBackupNoticeMail(ctx, cfg, job.FilePath, job.FileSize); err != nil {
			job.EmailStatus = "failed"
			job.ErrorMessage = err.Error()
			return
		}
		job.EmailStatus = "sent_without_attachment"
		return
	}
	if err := services.SendBackupMail(ctx, cfg, job.FilePath); err != nil {
		job.EmailStatus = "failed"
		job.ErrorMessage = err.Error()
		return
	}
	job.EmailStatus = "sent"
}

func (h BackupHandler) backupRecipient() string {
	var setting models.Setting
	if h.DB != nil && h.DB.First(&setting, "key = ?", backupRecipientSettingKey).Error == nil {
		if value := strings.TrimSpace(setting.Value); value != "" {
			return value
		}
	}
	return strings.TrimSpace(h.Cfg.SMTPTo)
}
