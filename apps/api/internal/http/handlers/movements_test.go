package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"gaowang/apps/api/internal/config"
	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Test_MovementRoutes_preview_update_and_mark_latest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newMovementTestDB(t)
	admin := models.User{Name: "Admin", Email: "movement-admin@example.com", PasswordHash: "secret-hash", Role: models.RoleAdmin, Enabled: true}
	product := models.Product{
		Name: "Tea", Code: "MOVE-TEA", DefaultPurchaseCents: 100, DefaultSaleCents: 250, Enabled: true,
	}
	for _, value := range []any{&admin, &product} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	service := services.InventoryService{DB: db}
	if _, err := service.CreateInbound(services.InboundInput{ProductID: product.ID, Quantity: 5, OperatorID: admin.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	var inbound models.StockMovement
	if err := db.First(&inbound, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load inbound: %v", err)
	}
	token := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(config.Config{AuthSecret: testAuthSecret}, db)

	list := doJSON(t, router, http.MethodGet, "/api/v1/stock-movements", token, nil)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "secret-hash") {
		t.Fatalf("list status/body = %d %s", list.Code, list.Body.String())
	}
	var listBody struct {
		Items []struct {
			ID       string `json:"ID"`
			IsLatest bool   `json:"IsLatest"`
			Operator struct {
				Name string `json:"name"`
			} `json:"Operator"`
		} `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode movement list: %v", err)
	}
	if len(listBody.Items) != 1 || !listBody.Items[0].IsLatest || listBody.Items[0].Operator.Name != admin.Name {
		t.Fatalf("movement list = %+v", listBody.Items)
	}

	payload := map[string]any{
		"expected_revision": inbound.Revision, "quantity": 6,
		"shop_id": nil, "note": "修正备注", "change_reason": "录入错误",
	}
	preview := doJSON(t, router, http.MethodPost, "/api/v1/stock-movements/"+inbound.ID.String()+"/preview", token, payload)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"result_quantity":6`) {
		t.Fatalf("preview status/body = %d %s", preview.Code, preview.Body.String())
	}
	update := doJSON(t, router, http.MethodPatch, "/api/v1/stock-movements/"+inbound.ID.String(), token, payload)
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"Revision":2`) {
		t.Fatalf("update status/body = %d %s", update.Code, update.Body.String())
	}
	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Quantity != 6 || snapshot.InventoryValueCents != 600 || snapshot.MovingAverageCostCents != 100 {
		t.Fatalf("snapshot = %d/%d/%d", snapshot.Quantity, snapshot.InventoryValueCents, snapshot.MovingAverageCostCents)
	}

	invalid := map[string]any{
		"expected_revision": 2, "quantity": 6, "unit_cents": 120,
		"note": "", "change_reason": "尝试改单价",
	}
	strict := doJSON(t, router, http.MethodPost, "/api/v1/stock-movements/"+inbound.ID.String()+"/preview", token, invalid)
	if strict.Code != http.StatusBadRequest || !strings.Contains(strict.Body.String(), "unknown field") {
		t.Fatalf("strict request status/body = %d %s", strict.Code, strict.Body.String())
	}

	if _, err := service.CreateAdjustment(services.AdjustmentInput{ProductID: product.ID, QuantityDelta: -1, Reason: "盘点", OperatorID: admin.ID}); err != nil {
		t.Fatalf("create newer movement: %v", err)
	}
	list = doJSON(t, router, http.MethodGet, "/api/v1/stock-movements", token, nil)
	listBody.Items = nil
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode movement list after adjustment: %v", err)
	}
	if len(listBody.Items) != 2 || listBody.Items[0].ID == inbound.ID.String() || !listBody.Items[0].IsLatest || listBody.Items[1].IsLatest {
		t.Fatalf("latest movement flags = %+v", listBody.Items)
	}
	stalePayload := map[string]any{
		"expected_revision": 2, "quantity": 6,
		"shop_id": nil, "note": "修正备注", "change_reason": "过期编辑",
	}
	stale := doJSON(t, router, http.MethodPatch, "/api/v1/stock-movements/"+inbound.ID.String(), token, stalePayload)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "MOVEMENT_STALE") {
		t.Fatalf("stale update status/body = %d %s", stale.Code, stale.Body.String())
	}
}

func Test_MovementUpdateRoutes_require_independent_permission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newMovementTestDB(t)
	staff := createTestUser(t, db, "Staff", "movement-staff@example.com", "password123", models.RoleStaff)
	product := models.Product{Name: "Coffee", Code: "MOVE-COFFEE", DefaultPurchaseCents: 100, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if _, err := (services.InventoryService{DB: db}).CreateInbound(services.InboundInput{ProductID: product.ID, Quantity: 1, OperatorID: staff.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	var movement models.StockMovement
	if err := db.First(&movement, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load movement: %v", err)
	}
	setUserPermissions(t, db, staff.ID, services.PermMovementRead)
	token := createSessionToken(t, db, staff.ID)
	router := apihttp.NewRouter(testConfig(), db)
	payload := map[string]any{
		"expected_revision": movement.Revision, "quantity": 1,
		"note": "", "change_reason": "测试权限",
	}
	if response := doJSON(t, router, http.MethodGet, "/api/v1/stock-movements", token, nil); response.Code != http.StatusOK {
		t.Fatalf("read status = %d body=%s", response.Code, response.Body.String())
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/stock-movements/" + movement.ID.String() + "/preview"},
		{http.MethodPatch, "/api/v1/stock-movements/" + movement.ID.String()},
	} {
		response := doJSON(t, router, request.method, request.path, token, payload)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d body=%s", request.method, request.path, response.Code, response.Body.String())
		}
	}
}

func Test_MovementList_filters_created_at_by_shanghai_day_and_uses_current_product_prices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newMovementTestDB(t)
	admin := models.User{Name: "Admin", Email: "movement-filter@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	product := models.Product{
		Name: "Tea", Code: "MOVE-FILTER", DefaultPurchaseCents: 120, DefaultSaleCents: 300, Enabled: true,
	}
	shop := models.Shop{Name: "Main", Enabled: true}
	for _, value := range []any{&admin, &product, &shop} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	before := models.StockMovement{
		Type: models.MovementTypeInbound, ProductID: product.ID, QuantityDelta: 1,
		PurchaseAmountCents: 1, OperatorID: admin.ID, CreatedAt: time.Date(2026, 6, 30, 15, 59, 59, 0, time.UTC),
	}
	start := models.StockMovement{
		Type: models.MovementTypeSalesOutbound, ProductID: product.ID, ShopID: &shop.ID, QuantityDelta: -2,
		RevenueCents: 1, CostAmountCents: 1, GrossProfitCents: 0,
		OperatorID: admin.ID, CreatedAt: time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC),
	}
	end := models.StockMovement{
		Type: models.MovementTypeAdjustment, ProductID: product.ID, QuantityDelta: 3,
		CostAmountCents: 1, Reason: "盘点", OperatorID: admin.ID,
		CreatedAt: time.Date(2026, 7, 1, 15, 59, 59, 0, time.UTC),
	}
	after := models.StockMovement{
		Type: models.MovementTypeSalesOutbound, ProductID: product.ID, ShopID: &shop.ID, QuantityDelta: -1,
		OperatorID: admin.ID, CreatedAt: time.Date(2026, 7, 1, 16, 0, 0, 0, time.UTC),
	}
	for _, movement := range []*models.StockMovement{&before, &start, &end, &after} {
		if err := db.Create(movement).Error; err != nil {
			t.Fatalf("create movement: %v", err)
		}
	}
	beijingNoon := time.Date(2026, 7, 1, 4, 0, 0, 0, time.UTC)
	if err := db.Model(&before).Update("updated_at", beijingNoon).Error; err != nil {
		t.Fatalf("update outside movement: %v", err)
	}

	token := createSessionToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)
	response := doJSON(t, router, http.MethodGet, "/api/v1/stock-movements?from=2026-07-01&to=2026-07-01&page_size=100", token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items []struct {
			ID                  string `json:"ID"`
			Type                string `json:"Type"`
			PurchaseAmountCents int64  `json:"PurchaseAmountCents"`
			RevenueCents        int64  `json:"RevenueCents"`
			CostAmountCents     int64  `json:"CostAmountCents"`
			GrossProfitCents    int64  `json:"GrossProfitCents"`
			IsLatest            bool   `json:"IsLatest"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode filtered list: %v", err)
	}
	if len(body.Items) != 2 || body.Items[0].ID != end.ID.String() || body.Items[1].ID != start.ID.String() {
		t.Fatalf("filtered items = %+v, want end/start boundaries", body.Items)
	}
	if body.Items[0].CostAmountCents != 360 || body.Items[0].IsLatest {
		t.Fatalf("adjustment projection/latest = %+v, want cost 360 and not latest", body.Items[0])
	}
	if body.Items[1].RevenueCents != 600 || body.Items[1].CostAmountCents != 240 || body.Items[1].GrossProfitCents != 360 {
		t.Fatalf("sale projection = %+v, want 600/240/360", body.Items[1])
	}

	combined := doJSON(t, router, http.MethodGet,
		"/api/v1/stock-movements?from=2026-07-01&to=2026-07-01&type=sales_outbound&product_id="+product.ID.String()+"&shop_id="+shop.ID.String(),
		token, nil,
	)
	if combined.Code != http.StatusOK {
		t.Fatalf("combined list status = %d body=%s", combined.Code, combined.Body.String())
	}
	body.Items = nil
	if err := json.Unmarshal(combined.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode combined list: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != start.ID.String() {
		t.Fatalf("combined filtered items = %+v, want start sale only", body.Items)
	}
}

func newMovementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return openHandlerTestDB(t,
		&models.User{}, &models.Session{}, &models.UserPermission{}, &models.Shop{}, &models.Product{},
		&models.InventorySnapshot{}, &models.StockMovement{}, &models.AuditLog{},
	)
}
