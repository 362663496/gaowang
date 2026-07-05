package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gaowang/apps/api/internal/config"
	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_AuditLogs_records_failed_login_without_password(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	db := newAuditTestDB(t)
	router := apihttp.NewRouter(config.Config{}, db)
	body := `{"login":"missing@example.com","password":"wrong-password"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, request)

	// Then
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
	// Given
	gin.SetMode(gin.TestMode)
	db := newAuditTestDB(t)
	admin := createAuditUser(t, db, models.RoleAdmin)
	staff := createAuditUser(t, db, models.RoleStaff)
	router := apihttp.NewRouter(config.Config{}, db)

	createShopResponse := postAuditJSON(t, router, admin.ID, models.RoleAdmin, "/api/v1/shops", map[string]string{
		"name": "Main",
		"note": "central",
	})
	if createShopResponse.Code != http.StatusCreated {
		t.Fatalf("create shop status = %d, want %d; body = %s", createShopResponse.Code, http.StatusCreated, createShopResponse.Body.String())
	}

	// When
	adminResponse := getAudit(t, router, admin.ID, models.RoleAdmin, "/api/v1/audit-logs?action=shop.create")
	staffResponse := getAudit(t, router, staff.ID, models.RoleStaff, "/api/v1/audit-logs")

	// Then
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

func newAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Shop{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func createAuditUser(t *testing.T, db *gorm.DB, role models.Role) models.User {
	t.Helper()
	hash, err := services.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := models.User{Name: uuid.NewString(), Email: uuid.NewString() + "@example.com", PasswordHash: hash, Role: role, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func postAuditJSON(t *testing.T, router http.Handler, userID uuid.UUID, role models.Role, path string, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Dev-User-ID", userID.String())
	request.Header.Set("X-Dev-Role", string(role))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func getAudit(t *testing.T, router http.Handler, userID uuid.UUID, role models.Role, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("X-Dev-User-ID", userID.String())
	request.Header.Set("X-Dev-Role", string(role))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
