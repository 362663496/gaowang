package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func Test_ReportEndpoints_return_empty_rows_without_sales(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	db := newReportTestDB(t)
	user := createReportUser(t, db)
	token := createSessionToken(t, db, user.ID)
	router := apihttp.NewRouter(testConfig(), db)

	// When
	response := getReport(t, router, token, "/api/v1/reports/product-ranking")

	// Then
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Items []productRankingRow `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(body.Items))
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatalf("body = %s, want empty array", response.Body.String())
	}
}

func Test_ReportEndpoints_group_sales_by_day_product_and_shop(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	db := newReportTestDB(t)
	user := createReportUser(t, db)
	product := models.Product{Name: "Tea", Code: "TEA", ImagePath: "/uploads/tea.png", Enabled: true}
	shop := models.Shop{Name: "Main", Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("create shop: %v", err)
	}
	service := services.InventoryService{DB: db}
	if err := service.CreateInbound(services.InboundInput{ProductID: product.ID, Quantity: 5, UnitCents: 100, OperatorID: user.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err := service.CreateSalesOutbound(services.OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 2, SaleUnitCents: 250, OperatorID: user.ID}); err != nil {
		t.Fatalf("create sale: %v", err)
	}
	createdAt := time.Now().UTC().Add(-time.Hour)
	var sale models.StockMovement
	if err := db.Where("type = ?", models.MovementTypeSalesOutbound).First(&sale).Error; err != nil {
		t.Fatalf("load sale: %v", err)
	}
	if err := db.Model(&models.StockMovement{}).Where("type = ?", models.MovementTypeInbound).Update("created_at", createdAt.Add(-time.Hour)).Error; err != nil {
		t.Fatalf("date inbound: %v", err)
	}
	if err := db.Model(&sale).Update("created_at", createdAt).Error; err != nil {
		t.Fatalf("date sale: %v", err)
	}
	quantity, unit := int64(3), int64(300)
	updated, _, err := service.UpdateMovement(services.MovementUpdateInput{
		MovementID: sale.ID, ExpectedRevision: sale.Revision, Quantity: &quantity, UnitCents: &unit,
		ShopID: &shop.ID, Note: "修正销售", ChangeReason: "数量修正", EditorID: user.ID,
	})
	if err != nil {
		t.Fatalf("update sale: %v", err)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("updated sale date = %s, want %s", updated.CreatedAt, createdAt)
	}
	archivedAt := time.Now().UTC()
	if err := db.Model(&product).Updates(map[string]any{"archived_at": archivedAt, "enabled": false}).Error; err != nil {
		t.Fatalf("archive product: %v", err)
	}
	token := createSessionToken(t, db, user.ID)
	router := apihttp.NewRouter(testConfig(), db)

	// When
	summaryResponse := getReport(t, router, token, "/api/v1/reports/sales-summary")
	trendResponse := getReport(t, router, token, "/api/v1/reports/sales-trend")
	productResponse := getReport(t, router, token, "/api/v1/reports/product-ranking")
	shopResponse := getReport(t, router, token, "/api/v1/reports/shop-ranking")

	// Then
	assertSalesSummaryResponse(t, summaryResponse, 900, 300, 600)
	assertTrendResponse(t, trendResponse, createdAt.Format("2006-01-02"), 900, 600)
	assertProductRankingResponse(t, productResponse, "Tea", "/uploads/tea.png", 900, 3, true)
	assertShopRankingResponse(t, shopResponse, "Main", 900, 3)
}

type productRankingRow struct {
	ProductName      string `json:"product_name"`
	ProductImagePath string `json:"product_image_path"`
	Archived         bool   `json:"archived"`
	RevenueCents     int64  `json:"revenue_cents"`
	GrossProfitCents int64  `json:"gross_profit_cents"`
	QuantitySold     int64  `json:"quantity_sold"`
}

type summaryRow struct {
	RevenueCents     int64 `json:"revenue_cents"`
	CostCents        int64 `json:"cost_cents"`
	GrossProfitCents int64 `json:"gross_profit_cents"`
}

type trendRow struct {
	Day              string `json:"day"`
	RevenueCents     int64  `json:"revenue_cents"`
	GrossProfitCents int64  `json:"gross_profit_cents"`
}

type shopRankingRow struct {
	ShopName         string `json:"shop_name"`
	RevenueCents     int64  `json:"revenue_cents"`
	QuantitySold     int64  `json:"quantity_sold"`
	GrossProfitCents int64  `json:"gross_profit_cents"`
}

func assertTrendResponse(t *testing.T, response *httptest.ResponseRecorder, day string, revenue int64, gross int64) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("trend status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Items []trendRow `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode trend: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("trend rows = %d, want 1; body = %s", len(body.Items), response.Body.String())
	}
	if body.Items[0].Day != day || body.Items[0].RevenueCents != revenue || body.Items[0].GrossProfitCents != gross {
		t.Fatalf("trend row = %+v, want %s/%d/%d", body.Items[0], day, revenue, gross)
	}
}

func assertSalesSummaryResponse(t *testing.T, response *httptest.ResponseRecorder, revenue int64, cost int64, gross int64) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("summary status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Summary summaryRow `json:"summary"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if body.Summary.RevenueCents != revenue || body.Summary.CostCents != cost || body.Summary.GrossProfitCents != gross {
		t.Fatalf("summary = %+v, want %d/%d/%d", body.Summary, revenue, cost, gross)
	}
}

func assertProductRankingResponse(t *testing.T, response *httptest.ResponseRecorder, productName string, imagePath string, revenue int64, quantity int64, archived bool) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("product status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Items []productRankingRow `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode product ranking: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("product rows = %d, want 1; body = %s", len(body.Items), response.Body.String())
	}
	if body.Items[0].ProductName != productName || body.Items[0].ProductImagePath != imagePath || body.Items[0].RevenueCents != revenue || body.Items[0].QuantitySold != quantity || body.Items[0].Archived != archived {
		t.Fatalf("product row = %+v, want %s/%s/%d/%d/archived=%t", body.Items[0], productName, imagePath, revenue, quantity, archived)
	}
}

func assertShopRankingResponse(t *testing.T, response *httptest.ResponseRecorder, shopName string, revenue int64, quantity int64) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("shop status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Items []shopRankingRow `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode shop ranking: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("shop rows = %d, want 1; body = %s", len(body.Items), response.Body.String())
	}
	if body.Items[0].ShopName != shopName || body.Items[0].RevenueCents != revenue || body.Items[0].QuantitySold != quantity {
		t.Fatalf("shop row = %+v, want %s/%d/%d", body.Items[0], shopName, revenue, quantity)
	}
}

func newReportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return openHandlerTestDB(t, &models.User{}, &models.Session{}, &models.StaffPermission{}, &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{}, &models.AuditLog{})
}

func createReportUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	user := models.User{Name: uuid.NewString(), Email: uuid.NewString() + "@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func getReport(t *testing.T, router http.Handler, token string, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	withAuth(request, token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
