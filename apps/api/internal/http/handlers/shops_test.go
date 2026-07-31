package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
)

func Test_ShopUpdate_persists_fields_records_audit_and_requires_permission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Shop{})...)
	staff := createTestUser(t, db, "Staff", "shop-editor@example.com", "password123", models.RoleStaff)
	shop := models.Shop{Name: "Old Shop", Note: "old note", Enabled: true}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("create shop: %v", err)
	}
	token := createSessionToken(t, db, staff.ID)
	router := apihttp.NewRouter(testConfig(), db)
	payload := map[string]string{"name": "New Shop", "note": "new note"}

	setStaffPermissions(t, db, services.PermShopRead)
	denied := doJSON(t, router, http.MethodPut, "/api/v1/shops/"+shop.ID.String(), token, payload)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("update without permission status = %d, want 403; body = %s", denied.Code, denied.Body.String())
	}

	setStaffPermissions(t, db, services.PermShopUpdate)
	if list := doJSON(t, router, http.MethodGet, "/api/v1/shops", token, nil); list.Code != http.StatusOK {
		t.Fatalf("shop.update dependency did not grant read: status = %d body = %s", list.Code, list.Body.String())
	}
	response := doJSON(t, router, http.MethodPut, "/api/v1/shops/"+shop.ID.String(), token, payload)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Item models.Shop `json:"item"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Item.Name != "New Shop" || body.Item.Note != "new note" || !body.Item.Enabled {
		t.Fatalf("updated shop = %+v", body.Item)
	}
	var audits int64
	if err := db.Model(&models.AuditLog{}).Where("action = ? AND resource_id = ?", "shop.update", shop.ID.String()).Count(&audits).Error; err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if audits != 1 {
		t.Fatalf("audit count = %d, want 1", audits)
	}
}
