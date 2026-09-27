package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
)

func Test_MCP_lists_tools_and_rejects_ambiguous_outbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{}, &models.Shop{}, &models.InventorySnapshot{}, &models.StockMovement{})...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	green := models.Product{Name: "绿茶", Code: "TEA-GREEN", Enabled: true, DefaultPurchaseCents: 100}
	leaf := models.Product{Name: "绿茶", Code: "TEA-LEAF", Enabled: true, DefaultPurchaseCents: 100}
	if err := db.Create(&green).Error; err != nil {
		t.Fatalf("seed green: %v", err)
	}
	if err := db.Create(&leaf).Error; err != nil {
		t.Fatalf("seed leaf: %v", err)
	}
	shop := models.Shop{Name: "总店", Enabled: true}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("seed shop: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: green.ID, Quantity: 10}).Error; err != nil {
		t.Fatalf("seed stock: %v", err)
	}

	session := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)
	created := doJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", session, map[string]any{})
	var tokenBody struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &tokenBody); err != nil {
		t.Fatalf("decode token: %v", err)
	}

	list := doBearerJSON(t, router, http.MethodPost, "/api/v1/mcp", tokenBody.Secret, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	if list.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d body=%s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), "get_inventory") || !strings.Contains(list.Body.String(), "create_sales_outbound") {
		t.Fatalf("tools/list missing expected tools: %s", list.Body.String())
	}

	outbound := doBearerJSON(t, router, http.MethodPost, "/api/v1/mcp", tokenBody.Secret, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "create_sales_outbound",
			"arguments": map[string]any{
				"product_query": "绿茶",
				"shop_query":    "总店",
				"quantity":      1,
			},
		},
	})
	if outbound.Code != http.StatusOK {
		t.Fatalf("outbound status = %d body=%s", outbound.Code, outbound.Body.String())
	}
	if !strings.Contains(outbound.Body.String(), "isError") && !strings.Contains(outbound.Body.String(), "ambiguous") && !strings.Contains(outbound.Body.String(), "candidates") {
		t.Fatalf("ambiguous outbound body = %s", outbound.Body.String())
	}
	var count int64
	if err := db.Model(&models.StockMovement{}).Count(&count).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if count != 0 {
		t.Fatalf("ambiguous outbound wrote %d movements", count)
	}

	staff := createTestUser(t, db, "Staff", "staff@example.com", "password123", models.RoleStaff)
	setUserPermissions(t, db, staff.ID, services.PermProductRead)
	staffSession := createSessionToken(t, db, staff.ID)
	staffToken := doJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", staffSession, map[string]any{})
	var staffSecret struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(staffToken.Body.Bytes(), &staffSecret); err != nil {
		t.Fatalf("decode staff token: %v", err)
	}
	staffList := doBearerJSON(t, router, http.MethodPost, "/api/v1/mcp", staffSecret.Secret, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	if staffList.Code != http.StatusOK {
		t.Fatalf("staff tools/list status = %d body=%s", staffList.Code, staffList.Body.String())
	}
	if strings.Contains(staffList.Body.String(), "create_sales_outbound") {
		t.Fatalf("staff without outbound permission saw outbound tool: %s", staffList.Body.String())
	}
}

func Test_MCP_inbound_by_product_name(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{}, &models.Shop{}, &models.InventorySnapshot{}, &models.StockMovement{})...)
	admin := createTestUser(t, db, "Admin", "admin@example.com", "password123", models.RoleAdmin)
	product := models.Product{Name: "红茶", Code: "TEA-BLACK", Enabled: true, DefaultPurchaseCents: 200}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	session := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)
	created := doJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", session, map[string]any{})
	var tokenBody struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &tokenBody); err != nil {
		t.Fatalf("decode token: %v", err)
	}

	inbound := doBearerJSON(t, router, http.MethodPost, "/api/v1/mcp", tokenBody.Secret, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "create_inbound",
			"arguments": map[string]any{
				"product_query": "红茶",
				"quantity":      5,
			},
		},
	})
	if inbound.Code != http.StatusOK {
		t.Fatalf("inbound status = %d body=%s", inbound.Code, inbound.Body.String())
	}
	if strings.Contains(inbound.Body.String(), `"isError":true`) {
		t.Fatalf("inbound error body=%s", inbound.Body.String())
	}
	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Quantity != 5 {
		t.Fatalf("quantity = %d, want 5; mcp body=%s", snapshot.Quantity, inbound.Body.String())
	}
}
