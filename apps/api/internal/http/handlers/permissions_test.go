package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Test_Permissions_admin_can_read_and_update_user_grants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{})...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	staffA := createTestUser(t, db, "Staff A", "a@example.com", "password123", models.RoleStaff)
	staffB := createTestUser(t, db, "Staff B", "b@example.com", "password123", models.RoleStaff)
	setUserPermissions(t, db, staffB.ID, services.PermAuditRead)
	token := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	getResponse := doJSON(t, router, http.MethodGet, "/api/v1/permissions", token, nil)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", getResponse.Code, getResponse.Body.String())
	}
	var listBody struct {
		Catalog []services.PermissionDef `json:"catalog"`
		Users   []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
		} `json:"users"`
	}
	if err := json.Unmarshal(getResponse.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(listBody.Catalog) == 0 || len(listBody.Users) != 2 {
		t.Fatalf("permission response = %+v", listBody)
	}
	if listBody.Users[0].ID != staffA.ID.String() || listBody.Users[1].ID != staffB.ID.String() || !reflect.DeepEqual(listBody.Users[1].Permissions, []string{services.PermAuditRead}) {
		t.Fatalf("staff users = %+v", listBody.Users)
	}

	putResponse := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{
		"user_id":     staffA.ID.String(),
		"permissions": []string{services.PermProductCreate, services.PermProductDelete},
	})
	if putResponse.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", putResponse.Code, putResponse.Body.String())
	}
	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(putResponse.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{services.PermProductCreate, services.PermProductDelete, services.PermProductRead}
	if !reflect.DeepEqual(body.Permissions, want) {
		t.Fatalf("permissions = %v, want %v", body.Permissions, want)
	}
	getResponse = doJSON(t, router, http.MethodGet, "/api/v1/permissions", token, nil)
	listBody.Users = nil
	if err := json.Unmarshal(getResponse.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode permissions after reload: %v", err)
	}
	if len(listBody.Users) != 2 || !reflect.DeepEqual(listBody.Users[0].Permissions, want) {
		t.Fatalf("permissions after reload = %s", getResponse.Body.String())
	}

	staffAToken := createSessionToken(t, db, staffA.ID)
	staffBToken := createSessionToken(t, db, staffB.ID)
	for _, tc := range []struct {
		name        string
		token       string
		permissions []string
		productCode int
	}{
		{name: "staff A", token: staffAToken, permissions: want, productCode: http.StatusOK},
		{name: "staff B", token: staffBToken, permissions: []string{services.PermAuditRead}, productCode: http.StatusForbidden},
	} {
		me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", tc.token, nil)
		var authBody struct {
			Permissions []string `json:"permissions"`
		}
		if err := json.Unmarshal(me.Body.Bytes(), &authBody); err != nil || !reflect.DeepEqual(authBody.Permissions, tc.permissions) {
			t.Fatalf("%s /auth/me = %s, decode error = %v", tc.name, me.Body.String(), err)
		}
		products := doJSON(t, router, http.MethodGet, "/api/v1/products", tc.token, nil)
		if products.Code != tc.productCode {
			t.Fatalf("%s product status = %d, want %d", tc.name, products.Code, tc.productCode)
		}
	}

	var audit models.AuditLog
	if err := db.Where("action = ?", "permission.updated").First(&audit).Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	var metadata struct {
		UserID string   `json:"user_id"`
		Before []string `json:"before"`
		After  []string `json:"after"`
	}
	if err := json.Unmarshal(audit.Metadata, &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if audit.ResourceID != staffA.ID.String() || metadata.UserID != staffA.ID.String() || len(metadata.Before) != 0 || !reflect.DeepEqual(metadata.After, want) {
		t.Fatalf("audit = %+v metadata = %+v", audit, metadata)
	}
}

func Test_Permissions_reject_unknown_and_admin_only_keys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	token := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	unknown := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{
		"user_id":     staff.ID.String(),
		"permissions": []string{"nope.read"},
	})
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown status = %d body=%s", unknown.Code, unknown.Body.String())
	}

	adminOnly := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{
		"user_id":     staff.ID.String(),
		"permissions": []string{services.PermUserRead},
	})
	if adminOnly.Code != http.StatusBadRequest {
		t.Fatalf("admin-only status = %d body=%s", adminOnly.Code, adminOnly.Body.String())
	}

	invalidID := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{"user_id": "bad", "permissions": []string{}})
	if invalidID.Code != http.StatusBadRequest {
		t.Fatalf("invalid ID status = %d body=%s", invalidID.Code, invalidID.Body.String())
	}
	missing := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{"user_id": uuid.NewString(), "permissions": []string{}})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing user status = %d body=%s", missing.Code, missing.Body.String())
	}
	adminTarget := doJSON(t, router, http.MethodPut, "/api/v1/permissions", token, map[string]any{"user_id": admin.ID.String(), "permissions": []string{}})
	if adminTarget.Code != http.StatusBadRequest {
		t.Fatalf("admin target status = %d body=%s", adminTarget.Code, adminTarget.Body.String())
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
	setUserPermissions(t, db, staff.ID, services.PermProductCreate, services.PermProductUpdate, services.PermProductToggle)
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
