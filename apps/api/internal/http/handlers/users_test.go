package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/http/handlers"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Test_UserDelete_soft_deletes_and_revokes_access(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{}, &models.Shop{}, &models.StockMovement{})...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	setUserPermissions(t, db, staff.ID, services.PermProductRead)
	adminToken := createSessionToken(t, db, admin.ID)
	staffToken := createSessionToken(t, db, staff.ID)
	product := models.Product{Name: "Tea", Code: "TEA", Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	movement := models.StockMovement{Type: models.MovementTypeInbound, ProductID: product.ID, QuantityDelta: 1, OperatorID: staff.ID}
	if err := db.Create(&movement).Error; err != nil {
		t.Fatalf("create historical movement: %v", err)
	}
	history := models.AuditLog{ActorID: &staff.ID, Action: "inventory.inbound", ResourceType: "product", ResourceID: uuid.NewString()}
	if err := db.Create(&history).Error; err != nil {
		t.Fatalf("create historical audit: %v", err)
	}
	router := apihttp.NewRouter(testConfig(), db)

	response := doJSON(t, router, http.MethodDelete, "/api/v1/users/"+staff.ID.String(), adminToken, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d body=%s", response.Code, response.Body.String())
	}
	repeated := doJSON(t, router, http.MethodDelete, "/api/v1/users/"+staff.ID.String(), adminToken, nil)
	if repeated.Code != http.StatusNotFound {
		t.Fatalf("repeated DELETE status = %d body=%s", repeated.Code, repeated.Body.String())
	}
	for _, body := range []map[string]string{
		{"name": staff.Name, "email": "replacement@example.com", "password": "password123", "role": string(models.RoleStaff)},
		{"name": "Replacement", "email": staff.Email, "password": "password123", "role": string(models.RoleStaff)},
	} {
		if create := doJSON(t, router, http.MethodPost, "/api/v1/users", adminToken, body); create.Code != http.StatusBadRequest {
			t.Fatalf("reusing deleted identity = %d body=%s", create.Code, create.Body.String())
		}
	}

	var deleted models.User
	if err := db.First(&deleted, "id = ?", staff.ID).Error; err != nil {
		t.Fatalf("load deleted user: %v", err)
	}
	if deleted.Enabled || deleted.DeletedAt == nil {
		t.Fatalf("deleted user = %+v", deleted)
	}
	for name, model := range map[string]any{"sessions": &models.Session{}, "permissions": &models.UserPermission{}} {
		var count int64
		if err := db.Model(model).Where("user_id = ?", staff.ID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s after delete = %d, err=%v", name, count, err)
		}
	}
	if me := doJSON(t, router, http.MethodGet, "/api/v1/auth/me", staffToken, nil); me.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user session status = %d body=%s", me.Code, me.Body.String())
	}
	if login := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"login": staff.Email, "password": "password123"}); login.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user login status = %d body=%s", login.Code, login.Body.String())
	}
	list := doJSON(t, router, http.MethodGet, "/api/v1/users?all=true", adminToken, nil)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), staff.ID.String()) {
		t.Fatalf("user list = %d body=%s", list.Code, list.Body.String())
	}
	permissions := doJSON(t, router, http.MethodGet, "/api/v1/permissions", adminToken, nil)
	if permissions.Code != http.StatusOK || strings.Contains(permissions.Body.String(), staff.ID.String()) {
		t.Fatalf("permission list = %d body=%s", permissions.Code, permissions.Body.String())
	}
	update := doJSON(t, router, http.MethodPut, "/api/v1/permissions", adminToken, map[string]any{"user_id": staff.ID.String(), "permissions": []string{}})
	if update.Code != http.StatusNotFound {
		t.Fatalf("deleted permission target status = %d body=%s", update.Code, update.Body.String())
	}

	var audit models.AuditLog
	if err := db.Where("action = ? AND resource_id = ?", "user.delete", staff.ID.String()).First(&audit).Error; err != nil {
		t.Fatalf("load delete audit: %v", err)
	}
	var metadata map[string]string
	if err := json.Unmarshal(audit.Metadata, &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if audit.ActorID == nil || *audit.ActorID != admin.ID || metadata["email"] != staff.Email || metadata["role"] != string(staff.Role) {
		t.Fatalf("audit = %+v metadata=%v", audit, metadata)
	}
	var historical models.AuditLog
	if err := db.Preload("Actor").First(&historical, "id = ?", history.ID).Error; err != nil {
		t.Fatalf("load historical audit: %v", err)
	}
	if historical.Actor == nil || historical.Actor.ID != staff.ID {
		t.Fatalf("historical actor lost: %+v", historical.Actor)
	}
	var historicalMovement models.StockMovement
	if err := db.Preload("Operator").First(&historicalMovement, "id = ?", movement.ID).Error; err != nil {
		t.Fatalf("load historical movement: %v", err)
	}
	if historicalMovement.Operator.ID != staff.ID {
		t.Fatalf("historical operator lost: %+v", historicalMovement.Operator)
	}
}

func Test_UserDelete_rejects_self_last_admin_and_missing_users(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	adminToken := createSessionToken(t, db, admin.ID)
	staffToken := createSessionToken(t, db, staff.ID)
	router := apihttp.NewRouter(testConfig(), db)

	self := doJSON(t, router, http.MethodDelete, "/api/v1/users/"+admin.ID.String(), adminToken, nil)
	if self.Code != http.StatusConflict || !strings.Contains(self.Body.String(), "USER_DELETE_SELF") {
		t.Fatalf("self delete = %d body=%s", self.Code, self.Body.String())
	}
	invalid := doJSON(t, router, http.MethodDelete, "/api/v1/users/bad", adminToken, nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid ID = %d body=%s", invalid.Code, invalid.Body.String())
	}
	missing := doJSON(t, router, http.MethodDelete, "/api/v1/users/"+uuid.NewString(), adminToken, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing user = %d body=%s", missing.Code, missing.Body.String())
	}
	forbidden := doJSON(t, router, http.MethodDelete, "/api/v1/users/"+admin.ID.String(), staffToken, nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("staff delete = %d body=%s", forbidden.Code, forbidden.Body.String())
	}

	direct := gin.New()
	direct.DELETE("/users/:id", func(c *gin.Context) {
		c.Set("current_user_id", staff.ID)
		c.Next()
	}, handlers.UserHandler{DB: db}.Delete)
	last := doJSON(t, direct, http.MethodDelete, "/users/"+admin.ID.String(), "", nil)
	if last.Code != http.StatusConflict || !strings.Contains(last.Body.String(), "LAST_ADMIN") {
		t.Fatalf("last admin delete = %d body=%s", last.Code, last.Body.String())
	}
}

func Test_UserDelete_rolls_back_when_audit_fails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, authModels()...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	setUserPermissions(t, db, staff.ID, services.PermProductRead)
	adminToken := createSessionToken(t, db, admin.ID)
	createSessionToken(t, db, staff.ID)
	router := apihttp.NewRouter(testConfig(), db)
	if err := db.Migrator().DropTable(&models.AuditLog{}); err != nil {
		t.Fatalf("drop audit table: %v", err)
	}

	response := doJSON(t, router, http.MethodDelete, "/api/v1/users/"+staff.ID.String(), adminToken, nil)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "USER_DELETE_FAILED") {
		t.Fatalf("DELETE status = %d body=%s", response.Code, response.Body.String())
	}
	var unchanged models.User
	if err := db.First(&unchanged, "id = ?", staff.ID).Error; err != nil {
		t.Fatalf("load staff: %v", err)
	}
	if !unchanged.Enabled || unchanged.DeletedAt != nil {
		t.Fatalf("user changed despite rollback: %+v", unchanged)
	}
	for name, model := range map[string]any{"sessions": &models.Session{}, "permissions": &models.UserPermission{}} {
		var count int64
		if err := db.Model(model).Where("user_id = ?", staff.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s after rollback = %d, err=%v", name, count, err)
		}
	}
}
