package handlers

import (
	"testing"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_BackupHandler_uses_saved_recipient_before_env_fallback(t *testing.T) {
	db := newBackupHandlerTestDB(t)
	if err := db.Save(&models.Setting{Key: backupRecipientSettingKey, Value: "db@example.com"}).Error; err != nil {
		t.Fatalf("save setting: %v", err)
	}
	handler := BackupHandler{DB: db, Cfg: config.Config{SMTPTo: "env@example.com"}}

	if got := handler.backupRecipient(); got != "db@example.com" {
		t.Fatalf("recipient = %q, want db@example.com", got)
	}
}

func newBackupHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}
