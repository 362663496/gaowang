package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gaowang/apps/api/internal/config"
	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_Settings_returns_and_updates_backup_email_recipient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newSettingsTestDB(t)
	router := apihttp.NewRouter(config.Config{SMTPTo: "env@example.com"}, db)

	getResponse := settingsRequest(t, router, http.MethodGet, nil)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d; body = %s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	var got struct {
		Settings struct {
			BackupEmailRecipient string `json:"backup_email_recipient"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(getResponse.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if got.Settings.BackupEmailRecipient != "env@example.com" {
		t.Fatalf("recipient = %q, want env@example.com", got.Settings.BackupEmailRecipient)
	}

	postResponse := settingsRequest(t, router, http.MethodPost, map[string]string{"backup_email_recipient": "ops@example.com"})
	if postResponse.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want %d; body = %s", postResponse.Code, http.StatusOK, postResponse.Body.String())
	}

	getResponse = settingsRequest(t, router, http.MethodGet, nil)
	if err := json.Unmarshal(getResponse.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode second GET: %v", err)
	}
	if got.Settings.BackupEmailRecipient != "ops@example.com" {
		t.Fatalf("recipient = %q, want ops@example.com", got.Settings.BackupEmailRecipient)
	}
}

func newSettingsTestDB(t *testing.T) *gorm.DB {
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

func settingsRequest(t *testing.T, router http.Handler, method string, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	request := httptest.NewRequest(method, "/api/v1/settings", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Dev-User-ID", uuid.NewString())
	request.Header.Set("X-Dev-Role", "admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
