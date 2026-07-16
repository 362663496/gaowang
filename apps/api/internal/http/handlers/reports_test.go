package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gaowang/apps/api/internal/config"
	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_ReportEndpoints_return_empty_rows_without_sales(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	db := newReportTestDB(t)
	user := createReportUser(t, db)
	router := apihttp.NewRouter(config.Config{}, db)

	// When
	response := getReport(t, router, user.ID, "/api/v1/reports/product-ranking")

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
	product := models.Product{Name: "Tea", Code: "TEA", Enabled: true}
	shop := models.Shop{Name: "Main", Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("create shop: %v", err)
	}
	createdAt := time.Now().UTC().Add(-time.Hour)
	if err := db.Create(&models.StockMovement{
		Type: models.MovementTypeSalesOutbound, ProductID: product.ID, ShopID: &shop.ID,
		QuantityDelta: -2, RevenueCents: 500, CostAmountCents: 200, GrossProfitCents: 300,
		OperatorID: user.ID, CreatedAt: createdAt,
	}).Error; err != nil {
		t.Fatalf("create movement: %v", err)
	}
	archivedAt := time.Now().UTC()
	if err := db.Model(&product).Updates(map[string]any{"archived_at": archivedAt, "enabled": false}).Error; err != nil {
		t.Fatalf("archive product: %v", err)
	}
	router := apihttp.NewRouter(config.Config{}, db)

	// When
	summaryResponse := getReport(t, router, user.ID, "/api/v1/reports/sales-summary")
	trendResponse := getReport(t, router, user.ID, "/api/v1/reports/sales-trend")
	productResponse := getReport(t, router, user.ID, "/api/v1/reports/product-ranking")
	shopResponse := getReport(t, router, user.ID, "/api/v1/reports/shop-ranking")

	// Then
	assertSalesSummaryResponse(t, summaryResponse, 500, 200, 300)
	assertTrendResponse(t, trendResponse, createdAt.Format("2006-01-02"), 500, 300)
	assertProductRankingResponse(t, productResponse, "Tea", 500, 2, true)
	assertShopRankingResponse(t, shopResponse, "Main", 500, 2)
}

type productRankingRow struct {
	ProductName      string `json:"product_name"`
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

func assertProductRankingResponse(t *testing.T, response *httptest.ResponseRecorder, productName string, revenue int64, quantity int64, archived bool) {
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
	if body.Items[0].ProductName != productName || body.Items[0].RevenueCents != revenue || body.Items[0].QuantitySold != quantity || body.Items[0].Archived != archived {
		t.Fatalf("product row = %+v, want %s/%d/%d/archived=%t", body.Items[0], productName, revenue, quantity, archived)
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
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Shop{}, &models.Product{}, &models.StockMovement{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func createReportUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	user := models.User{Name: uuid.NewString(), Email: uuid.NewString() + "@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func getReport(t *testing.T, router http.Handler, userID uuid.UUID, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("X-Dev-User-ID", userID.String())
	request.Header.Set("X-Dev-Role", string(models.RoleAdmin))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
