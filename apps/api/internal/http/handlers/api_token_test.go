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

func Test_APIToken_create_replace_revoke_and_bearer_access(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{})...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	session := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	empty := doJSON(t, router, http.MethodGet, "/api/v1/auth/api-token", session, nil)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"token":null`) {
		t.Fatalf("empty token GET = %d %s", empty.Code, empty.Body.String())
	}

	created := doJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", session, map[string]any{})
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body.String())
	}
	var createdBody struct {
		Secret string `json:"secret"`
		Token  struct {
			Prefix string `json:"prefix"`
		} `json:"token"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil || !strings.HasPrefix(createdBody.Secret, "gw_") {
		t.Fatalf("create body = %s err=%v", created.Body.String(), err)
	}

	listed := doJSON(t, router, http.MethodGet, "/api/v1/auth/api-token", session, nil)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), createdBody.Secret) {
		t.Fatalf("list leaked secret: %s", listed.Body.String())
	}

	products := doBearerJSON(t, router, http.MethodGet, "/api/v1/products", createdBody.Secret, nil)
	if products.Code != http.StatusOK {
		t.Fatalf("bearer products = %d %s", products.Code, products.Body.String())
	}

	users := doBearerJSON(t, router, http.MethodGet, "/api/v1/users", createdBody.Secret, nil)
	if users.Code != http.StatusForbidden {
		t.Fatalf("bearer users = %d, want 403; body=%s", users.Code, users.Body.String())
	}
	selfRotate := doBearerJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", createdBody.Secret, map[string]any{})
	if selfRotate.Code != http.StatusForbidden {
		t.Fatalf("bearer rotate token = %d, want 403; body=%s", selfRotate.Code, selfRotate.Body.String())
	}

	replaced := doJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", session, map[string]any{})
	var replacedBody struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(replaced.Body.Bytes(), &replacedBody); err != nil {
		t.Fatalf("decode replace: %v", err)
	}
	old := doBearerJSON(t, router, http.MethodGet, "/api/v1/products", createdBody.Secret, nil)
	if old.Code != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want 401", old.Code)
	}
	if fresh := doBearerJSON(t, router, http.MethodGet, "/api/v1/products", replacedBody.Secret, nil); fresh.Code != http.StatusOK {
		t.Fatalf("new token status = %d %s", fresh.Code, fresh.Body.String())
	}

	revoke := doJSON(t, router, http.MethodDelete, "/api/v1/auth/api-token", session, nil)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d %s", revoke.Code, revoke.Body.String())
	}
	if after := doBearerJSON(t, router, http.MethodGet, "/api/v1/products", replacedBody.Secret, nil); after.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d", after.Code)
	}
}

func Test_APIToken_cookie_csrf_still_required_and_mcp_rejects_cookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	session := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Host = testRequestHost
	request.Header.Set("Origin", "http://evil.example")
	request.AddCookie(&http.Cookie{Name: services.SessionCookieName, Value: session})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("csrf status = %d, want 403; body=%s", response.Code, response.Body.String())
	}

	mcp := doJSON(t, router, http.MethodGet, "/api/v1/mcp", session, nil)
	if mcp.Code != http.StatusUnauthorized {
		t.Fatalf("cookie mcp status = %d, want 401; body=%s", mcp.Code, mcp.Body.String())
	}
}

func Test_APIToken_staff_without_permission_gets_403_on_business_route(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{})...)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	session := createSessionToken(t, db, staff.ID)
	router := apihttp.NewRouter(testConfig(), db)
	created := doJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", session, map[string]any{})
	var body struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	products := doBearerJSON(t, router, http.MethodGet, "/api/v1/products", body.Secret, nil)
	if products.Code != http.StatusForbidden {
		t.Fatalf("staff bearer products = %d, want 403; body=%s", products.Code, products.Body.String())
	}
}
