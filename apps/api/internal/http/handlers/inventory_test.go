package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_InboundRequest_accepts_request_without_price(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	body := `{"product_id":"2e6ecf8c-4291-4cd8-b96f-1d35bfca449f","quantity":1}`
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	// When
	var req inboundRequest
	err := context.ShouldBindJSON(&req)

	// Then
	if err != nil {
		t.Fatalf("bind inbound request: %v", err)
	}
	if req.ProductID == "" || req.Quantity != 1 {
		t.Fatalf("request = %+v, want product and quantity", req)
	}
}

func Test_OutboundRequest_accepts_request_without_price(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	body := `{"product_id":"2e6ecf8c-4291-4cd8-b96f-1d35bfca449f","shop_id":"fe6f64b5-36aa-4642-adaa-58cf20f979bc","quantity":1}`
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	// When
	var req outboundRequest
	err := context.ShouldBindJSON(&req)

	// Then
	if err != nil {
		t.Fatalf("bind outbound request: %v", err)
	}
	if req.ProductID == "" || req.ShopID == "" || req.Quantity != 1 {
		t.Fatalf("request = %+v, want product, shop, and quantity", req)
	}
}

func Test_InventoryHandlers_persist_normalized_note_and_reject_overlong_note_without_writes(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	operator := models.User{Name: "Admin", Email: "handler-notes@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	product := models.Product{Name: "Tea", Code: "HANDLER-NOTE", Enabled: true}
	shop := models.Shop{Name: "Main", Enabled: true}
	if err := database.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := database.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := database.Create(&shop).Error; err != nil {
		t.Fatalf("create shop: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := InventoryHandler{DB: database}
	router.POST("/inbound", func(c *gin.Context) {
		c.Set("current_user_id", operator.ID)
		handler.CreateInbound(c)
	})
	router.POST("/outbound", func(c *gin.Context) {
		c.Set("current_user_id", operator.ID)
		handler.CreateSalesOutbound(c)
	})
	request := httptest.NewRequest(http.MethodPost, "/inbound", strings.NewReader(`{"product_id":"`+product.ID.String()+`","quantity":2,"note":"  到货检查完成  "}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("inbound status = %d body=%s", response.Code, response.Body.String())
	}
	var movement models.StockMovement
	if err := database.First(&movement, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load movement: %v", err)
	}
	var audit models.AuditLog
	if err := database.First(&audit, "action = ?", "inventory.inbound").Error; err != nil {
		t.Fatalf("load audit: %v", err)
	}
	var metadata map[string]string
	if err := json.Unmarshal(audit.Metadata, &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if movement.Reason != "到货检查完成" || metadata["note"] != movement.Reason {
		t.Fatalf("movement/audit note = %q/%q", movement.Reason, metadata["note"])
	}

	overlong, err := json.Marshal(map[string]any{
		"product_id": product.ID.String(), "shop_id": shop.ID.String(), "quantity": 1, "note": strings.Repeat("界", 501),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request = httptest.NewRequest(http.MethodPost, "/outbound", strings.NewReader(string(overlong)))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "STOCK_OPERATION_FAILED") {
		t.Fatalf("overlong status = %d body=%s", response.Code, response.Body.String())
	}
	var snapshot models.InventorySnapshot
	if err := database.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	var movementCount, auditCount int64
	database.Model(&models.StockMovement{}).Where("product_id = ?", product.ID).Count(&movementCount)
	database.Model(&models.AuditLog{}).Count(&auditCount)
	if snapshot.Quantity != 2 || movementCount != 1 || auditCount != 1 {
		t.Fatalf("state after rejected note = quantity/movements/audits %d/%d/%d", snapshot.Quantity, movementCount, auditCount)
	}
}
