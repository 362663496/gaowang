package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
)

func Test_AuditLogs_records_failed_login_without_password(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Shop{})...)
	router := apihttp.NewRouter(testConfig(), db)

	response := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"login":    "missing@example.com",
		"password": "wrong-password",
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	var log models.AuditLog
	if err := db.First(&log, "action = ?", "auth.login_failed").Error; err != nil {
		t.Fatalf("load audit log: %v", err)
	}
	if strings.Contains(string(log.Metadata), "wrong-password") {
		t.Fatalf("metadata leaked password: %s", string(log.Metadata))
	}
}

func Test_AuditLogs_lists_admin_filtered_records(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Shop{})...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	adminToken := createSessionToken(t, db, admin.ID)
	staffToken := createSessionToken(t, db, staff.ID)
	router := apihttp.NewRouter(testConfig(), db)

	createShopResponse := doJSON(t, router, http.MethodPost, "/api/v1/shops", adminToken, map[string]string{
		"name": "Main",
		"note": "central",
	})
	if createShopResponse.Code != http.StatusCreated {
		t.Fatalf("create shop status = %d, want %d; body = %s", createShopResponse.Code, http.StatusCreated, createShopResponse.Body.String())
	}

	adminResponse := doJSON(t, router, http.MethodGet, "/api/v1/audit-logs?action=shop.create", adminToken, nil)
	staffResponse := doJSON(t, router, http.MethodGet, "/api/v1/audit-logs", staffToken, nil)

	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want %d; body = %s", adminResponse.Code, http.StatusOK, adminResponse.Body.String())
	}
	var body struct {
		Items []struct {
			Action       string `json:"action"`
			ResourceType string `json:"resource_type"`
			Actor        *struct {
				Email string `json:"email"`
			} `json:"actor"`
		} `json:"items"`
	}
	if err := json.Unmarshal(adminResponse.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode audit list: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1; body = %s", len(body.Items), adminResponse.Body.String())
	}
	if body.Items[0].Action != "shop.create" || body.Items[0].ResourceType != "shop" {
		t.Fatalf("audit row = %+v, want shop.create/shop", body.Items[0])
	}
	if body.Items[0].Actor == nil || body.Items[0].Actor.Email == "" {
		t.Fatalf("actor missing from audit row: %+v", body.Items[0])
	}
	if staffResponse.Code != http.StatusForbidden {
		t.Fatalf("staff status = %d, want %d", staffResponse.Code, http.StatusForbidden)
	}
}
