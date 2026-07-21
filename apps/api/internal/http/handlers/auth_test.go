package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
)

func Test_ChangePassword_updates_current_users_password_and_revokes_sessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	user := createTestUser(t, db, "Admin", "admin@example.com", "old-password", models.RoleAdmin)
	tokenA := createSessionToken(t, db, user.ID)
	tokenB := createSessionToken(t, db, user.ID)
	router := apihttp.NewRouter(testConfig(), db)

	response := doJSON(t, router, http.MethodPost, "/api/v1/auth/password", tokenA, map[string]string{
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

	// All sessions for the user must be revoked.
	meA := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", tokenA, nil)
	meB := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", tokenB, nil)
	if meA.Code != http.StatusUnauthorized || meB.Code != http.StatusUnauthorized {
		t.Fatalf("me after password change = %d/%d, want 401/401", meA.Code, meB.Code)
	}
}

func Test_ChangePassword_rejects_wrong_current_password_without_clearing_session(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	user := createTestUser(t, db, "Admin", "admin@example.com", "old-password", models.RoleAdmin)
	token := createSessionToken(t, db, user.ID)
	router := apihttp.NewRouter(testConfig(), db)

	response := doJSON(t, router, http.MethodPost, "/api/v1/auth/password", token, map[string]string{
		"current_password": "wrong-password",
		"new_password":     "new-password",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "INVALID_CREDENTIALS") {
		t.Fatalf("body = %s, want INVALID_CREDENTIALS", response.Body.String())
	}
	me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", token, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("session cleared after wrong current password: status=%d body=%s", me.Code, me.Body.String())
	}
}

func Test_Login_accepts_username_or_email_and_sets_cookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	router := apihttp.NewRouter(testConfig(), db)

	for _, login := range []string{"Admin", "admin@example.com"} {
		response := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"login":    login,
			"password": "password123",
		})
		if response.Code != http.StatusOK {
			t.Fatalf("login %q status = %d, want %d; body = %s", login, response.Code, http.StatusOK, response.Body.String())
		}
		var body struct {
			User struct {
				Name string `json:"name"`
			} `json:"user"`
			Permissions []string `json:"permissions"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode login: %v", err)
		}
		if body.User.Name != "Admin" || len(body.Permissions) == 0 {
			t.Fatalf("login body = %+v", body)
		}
		cookie := response.Result().Cookies()
		found := false
		for _, item := range cookie {
			if item.Name == services.SessionCookieName && item.Value != "" && item.HttpOnly {
				found = true
				var count int64
				if err := db.Model(&models.Session{}).Where("token_hash = ?", services.SessionService{Secret: testAuthSecret}.HashToken(item.Value)).Count(&count).Error; err != nil {
					t.Fatalf("count session: %v", err)
				}
				if count != 1 {
					t.Fatalf("session hash rows = %d, want 1", count)
				}
			}
		}
		if !found {
			t.Fatal("expected HttpOnly session cookie")
		}
	}
}

func Test_Auth_ignores_dev_headers_and_requires_cookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	user := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	router := apihttp.NewRouter(testConfig(), db)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Host = testRequestHost
	request.Header.Set("Origin", testOrigin)
	request.Header.Set("X-Dev-User-ID", user.ID.String())
	request.Header.Set("X-Dev-Role", "admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", response.Code, response.Body.String())
	}
}

func Test_Logout_only_revokes_current_session(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	user := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	tokenA := createSessionToken(t, db, user.ID)
	tokenB := createSessionToken(t, db, user.ID)
	router := apihttp.NewRouter(testConfig(), db)

	logout := doJSON(t, router, http.MethodPost, "/api/v1/auth/logout", tokenA, map[string]any{})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}
	meA := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", tokenA, nil)
	meB := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", tokenB, nil)
	if meA.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out session status = %d, want 401", meA.Code)
	}
	if meB.Code != http.StatusOK {
		t.Fatalf("other session status = %d, want 200; body = %s", meB.Code, meB.Body.String())
	}
}
