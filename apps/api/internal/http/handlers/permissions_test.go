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

func Test_Permissions_admin_can_read_and_update_staff_grants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	token := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	getResponse := doJSON(t, router, http.MethodGet, "/api/v1/permissions", token, nil)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", getResponse.Code, getResponse.Body.String())
	}

	putResponse := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{
		"permissions": []string{services.PermProductCreate, services.PermProductDelete},
	})
	if putResponse.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", putResponse.Code, putResponse.Body.String())
	}
	var body struct {
		StaffPermissions []string `json:"staff_permissions"`
	}
	if err := json.Unmarshal(putResponse.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	joined := strings.Join(body.StaffPermissions, ",")
	for _, key := range []string{services.PermProductCreate, services.PermProductDelete, services.PermProductRead} {
		if !strings.Contains(joined, key) {
			t.Fatalf("staff permissions missing %s: %v", key, body.StaffPermissions)
		}
	}

	var audits int64
	if err := db.Model(&models.AuditLog{}).Where("action = ?", "permission.updated").Count(&audits).Error; err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if audits != 1 {
		t.Fatalf("audit count = %d, want 1", audits)
	}
}

func Test_Permissions_reject_unknown_and_admin_only_keys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	token := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	unknown := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{
		"permissions": []string{"nope.read"},
	})
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown status = %d body=%s", unknown.Code, unknown.Body.String())
	}

	adminOnly := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{
		"permissions": []string{services.PermUserRead},
	})
	if adminOnly.Code != http.StatusBadRequest {
		t.Fatalf("admin-only status = %d body=%s", adminOnly.Code, adminOnly.Body.String())
	}
}

func Test_ZeroPermissionStaff_denied_on_business_routes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{}, &models.Shop{}, &models.InventorySnapshot{}, &models.StockMovement{}, &models.Setting{}, &models.BackupJob{})...)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	token := createSessionToken(t, db, staff.ID)
	router := apihttp.NewRouter(testConfig(), db)

	// Account routes remain available.
	me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", token, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", me.Code, me.Body.String())
	}

	routes := router.Routes()
	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/api/v1/") {
			continue
		}
		path := route.Path
		if path == "/api/v1/health" || path == "/api/v1/auth/login" || path == "/api/v1/auth/me" || path == "/api/v1/auth/logout" || path == "/api/v1/auth/password" {
			continue
		}
		if strings.Contains(path, ":") {
			// Parameterized paths are covered by concrete IDs below where needed.
			continue
		}
		method := route.Method
		var response *httptest.ResponseRecorder
		switch method {
		case http.MethodGet:
			response = doJSON(t, router, method, path, token, nil)
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			response = doJSON(t, router, method, path, token, map[string]any{})
		default:
			continue
		}
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want 403; body = %s", method, path, response.Code, response.Body.String())
		}
	}
}

func Test_Staff_product_delete_independent_of_create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{})...)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	setStaffPermissions(t, db, services.PermProductCreate, services.PermProductUpdate, services.PermProductToggle)
	token := createSessionToken(t, db, staff.ID)
	product := models.Product{Name: "Tea", Code: "TEA", Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	router := apihttp.NewRouter(testConfig(), db)

	list := doJSON(t, router, http.MethodGet, "/api/v1/products", token, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	del := doJSON(t, router, http.MethodDelete, "/api/v1/products/"+product.ID.String(), token, nil)
	if del.Code != http.StatusForbidden {
		t.Fatalf("delete status = %d, want 403; body = %s", del.Code, del.Body.String())
	}
}

func Test_SameOrigin_rejects_cross_origin_mutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	token := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Host = testRequestHost
	request.Header.Set("Origin", "http://evil.example")
	request.AddCookie(&http.Cookie{Name: services.SessionCookieName, Value: token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", response.Code, response.Body.String())
	}
}
