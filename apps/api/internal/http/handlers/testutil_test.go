package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testAuthSecret = "abcdefghijklmnopqrstuvwxyz123456"
const testRequestHost = "example.com"
const testOrigin = "http://example.com"

func testConfig() config.Config {
	return config.Config{AuthSecret: testAuthSecret}
}

func openHandlerTestDB(t *testing.T, modelsToMigrate ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(modelsToMigrate...); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func authModels() []any {
	return []any{&models.User{}, &models.Session{}, &models.StaffPermission{}, &models.AuditLog{}}
}

func createTestUser(t *testing.T, db *gorm.DB, name string, email string, password string, role models.Role) models.User {
	t.Helper()
	hash, err := services.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := models.User{Name: name, Email: email, PasswordHash: hash, Role: role, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func createSessionToken(t *testing.T, db *gorm.DB, userID uuid.UUID) string {
	t.Helper()
	svc := services.SessionService{DB: db, Secret: testAuthSecret}
	raw, _, err := svc.Create(userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return raw
}

func withAuth(request *http.Request, rawToken string) *http.Request {
	request.Host = testRequestHost
	if request.Header.Get("Origin") == "" {
		request.Header.Set("Origin", testOrigin)
	}
	if rawToken != "" {
		request.AddCookie(&http.Cookie{Name: services.SessionCookieName, Value: rawToken})
	}
	return request
}

func doJSON(t *testing.T, router http.Handler, method string, path string, rawToken string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, body)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	withAuth(request, rawToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func doRaw(t *testing.T, router http.Handler, method string, path string, rawToken string, body string, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	withAuth(request, rawToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func setStaffPermissions(t *testing.T, db *gorm.DB, keys ...string) {
	t.Helper()
	if err := db.Where("1 = 1").Delete(&models.StaffPermission{}).Error; err != nil {
		t.Fatalf("clear staff permissions: %v", err)
	}
	expanded, err := services.ExpandPermissionClosure(keys)
	if err != nil {
		t.Fatalf("expand permissions: %v", err)
	}
	for _, key := range expanded {
		if err := db.Create(&models.StaffPermission{Permission: key}).Error; err != nil {
			t.Fatalf("create permission %s: %v", key, err)
		}
	}
}
