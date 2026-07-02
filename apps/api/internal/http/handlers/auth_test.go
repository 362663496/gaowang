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
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_ChangePassword_updates_current_users_password(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newAuthTestDB(t)
	user := createAuthTestUser(t, db, "old-password")
	router := apihttp.NewRouter(config.Config{}, db)

	response := postPasswordChange(t, router, user.ID, map[string]string{
		"current_password": "old-password",
		"new_password":     "new-password",
	})

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	var changed models.User
	if err := db.First(&changed, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if !services.PasswordMatches(changed.PasswordHash, "new-password") {
		t.Fatal("new password does not match stored hash")
	}
}

func Test_ChangePassword_rejects_wrong_current_password(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newAuthTestDB(t)
	user := createAuthTestUser(t, db, "old-password")
	router := apihttp.NewRouter(config.Config{}, db)

	response := postPasswordChange(t, router, user.ID, map[string]string{
		"current_password": "wrong-password",
		"new_password":     "new-password",
	})

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	var unchanged models.User
	if err := db.First(&unchanged, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if !services.PasswordMatches(unchanged.PasswordHash, "old-password") {
		t.Fatal("password changed after wrong current password")
	}
}

func newAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func createAuthTestUser(t *testing.T, db *gorm.DB, password string) models.User {
	t.Helper()
	hash, err := services.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := models.User{Name: "Admin", Email: "admin@example.com", PasswordHash: hash, Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func postPasswordChange(t *testing.T, router http.Handler, userID uuid.UUID, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Dev-User-ID", userID.String())
	request.Header.Set("X-Dev-Role", "admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
