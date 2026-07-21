package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
)

func Test_Settings_returns_and_updates_backup_email_recipient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Setting{})...)
	user := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	token := createSessionToken(t, db, user.ID)
	cfg := testConfig()
	cfg.SMTPTo = "env@example.com"
	router := apihttp.NewRouter(cfg, db)

	getResponse := doJSON(t, router, http.MethodGet, "/api/v1/settings", token, nil)
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

	postResponse := doJSON(t, router, http.MethodPost, "/api/v1/settings", token, map[string]string{"backup_email_recipient": "ops@example.com"})
	if postResponse.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want %d; body = %s", postResponse.Code, http.StatusOK, postResponse.Body.String())
	}

	getResponse = doJSON(t, router, http.MethodGet, "/api/v1/settings", token, nil)
	if err := json.Unmarshal(getResponse.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode second GET: %v", err)
	}
	if got.Settings.BackupEmailRecipient != "ops@example.com" {
		t.Fatalf("recipient = %q, want ops@example.com", got.Settings.BackupEmailRecipient)
	}
}
