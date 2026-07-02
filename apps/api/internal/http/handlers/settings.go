package handlers

import (
	"net/http"
	"strings"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SettingHandler struct {
	DB  *gorm.DB
	Cfg config.Config
}

type appSettingsResponse struct {
	BackupEmailRecipient string `json:"backup_email_recipient"`
}

type updateSettingsRequest struct {
	BackupEmailRecipient string `json:"backup_email_recipient" binding:"required,email"`
}

func (h SettingHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"settings": appSettingsResponse{BackupEmailRecipient: h.backupRecipient()}})
}

func (h SettingHandler) Update(c *gin.Context) {
	var req updateSettingsRequest
	if !bindJSON(c, &req) {
		return
	}
	setting := models.Setting{Key: backupRecipientSettingKey, Value: strings.TrimSpace(req.BackupEmailRecipient)}
	if err := h.DB.Save(&setting).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to save settings")
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": appSettingsResponse{BackupEmailRecipient: setting.Value}})
}

func (h SettingHandler) backupRecipient() string {
	return BackupHandler{DB: h.DB, Cfg: h.Cfg}.backupRecipient()
}
