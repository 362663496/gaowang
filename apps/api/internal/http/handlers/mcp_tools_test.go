package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func Test_MCP_lists_movements_by_cursor_without_gaps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, token, router := openMCPDB(t)
	product := seedMCPProduct(t, db, "绿茶", "TEA-GREEN", 100, 0)
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	want := map[string]struct{}{}
	for i := range 5 {
		movement := seedMCPMovement(t, db, models.StockMovement{
			Type: models.MovementTypeInbound, ProductID: product.ID, QuantityDelta: int64(i + 1),
			OperatorID: token.userID, CreatedAt: base.Add(time.Duration(i) * time.Hour),
		})
		want[movement.ID.String()] = struct{}{}
	}

	seen := map[string]struct{}{}
	var cursor any
	pages := 0
	for {
		args := map[string]any{"limit": 2, "order": "asc"}
		if cursor != nil {
			args["cursor"] = cursor
		}
		payload, isError, raw := callMCP(t, router, token.secret, "list_stock_movements", args)
		if isError {
			t.Fatalf("list error: %s", raw)
		}
		if asInt(payload["total_count"]) != 5 {
			t.Fatalf("total_count = %v, want 5; body=%s", payload["total_count"], raw)
		}
		items, _ := payload["items"].([]any)
		if len(items) == 0 || len(items) > 2 {
			t.Fatalf("page size = %d, want 1..2; body=%s", len(items), raw)
		}
		for _, item := range items {
			row := item.(map[string]any)
			id := row["id"].(string)
			if _, ok := seen[id]; ok {
				t.Fatalf("duplicate movement %s", id)
			}
			seen[id] = struct{}{}
			if _, ok := row["product"]; ok {
				t.Fatalf("default movement row embedded product: %s", raw)
			}
			if row["product_code"] != "TEA-GREEN" {
				t.Fatalf("slim movement = %#v", row)
			}
			if _, ok := row["note"]; !ok {
				t.Fatalf("slim movement missing note: %#v", row)
			}
		}
		pages++
		cursor = payload["next_cursor"]
		if cursor == nil {
			break
		}
		if pages > 5 {
			t.Fatalf("cursor did not finish: %s", raw)
		}
	}
	if pages != 3 || len(seen) != 5 {
		t.Fatalf("pages=%d seen=%d want 3/5", pages, len(seen))
	}
	for id := range want {
		if _, ok := seen[id]; !ok {
			t.Fatalf("missing movement %s", id)
		}
	}
}

func Test_MCP_summarize_movements_matches_movement_sum(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, token, router := openMCPDB(t)
	mainShop := seedMCPShop(t, db, "总店")
	branch := seedMCPShop(t, db, "分店")
	green := seedMCPProduct(t, db, "绿茶", "TEA-GREEN", 100, 0)
	black := seedMCPProduct(t, db, "红茶", "TEA-BLACK", 50, 0)
	base := time.Date(2026, 1, 1, 16, 30, 0, 0, time.UTC)
	seedMCPMovement(t, db, models.StockMovement{Type: models.MovementTypeSalesOutbound, ProductID: green.ID, ShopID: &mainShop.ID, QuantityDelta: -2, OperatorID: token.userID, CreatedAt: base})
	seedMCPMovement(t, db, models.StockMovement{Type: models.MovementTypeSalesOutbound, ProductID: green.ID, ShopID: &mainShop.ID, QuantityDelta: -1, OperatorID: token.userID, CreatedAt: base.Add(time.Hour)})
	seedMCPMovement(t, db, models.StockMovement{Type: models.MovementTypeSalesOutbound, ProductID: black.ID, ShopID: &branch.ID, QuantityDelta: -4, OperatorID: token.userID, CreatedAt: base.Add(2 * time.Hour)})
	seedMCPMovement(t, db, models.StockMovement{Type: models.MovementTypeInbound, ProductID: green.ID, QuantityDelta: 9, OperatorID: token.userID, CreatedAt: base.Add(3 * time.Hour)})

	summary, isError, raw := callMCP(t, router, token.secret, "summarize_movements", map[string]any{
		"type": "sales_outbound", "group_by": []string{"shop"},
	})
	if isError {
		t.Fatalf("summary error: %s", raw)
	}
	listed, isError, raw := callMCP(t, router, token.secret, "list_stock_movements", map[string]any{
		"type": "sales_outbound", "limit": 1000, "verbose": true,
	})
	if isError {
		t.Fatalf("list error: %s", raw)
	}
	type shopTotal struct {
		quantity int64
		count    int64
		cost     int64
	}
	fromList := map[string]shopTotal{}
	for _, item := range listed["items"].([]any) {
		row := item.(map[string]any)
		shop := row["shop"].(map[string]any)
		name := shop["Name"].(string)
		total := fromList[name]
		delta := asInt(row["quantity_delta"])
		if delta < 0 {
			delta = -delta
		}
		total.quantity += delta
		total.count++
		total.cost += asInt(row["cost_amount_cents"])
		fromList[name] = total
	}
	fromSummary := map[string]shopTotal{}
	for _, item := range summary["items"].([]any) {
		row := item.(map[string]any)
		fromSummary[row["shop_name"].(string)] = shopTotal{
			quantity: asInt(row["quantity"]), count: asInt(row["record_count"]), cost: asInt(row["cost_amount_cents"]),
		}
	}
	if len(fromSummary) != len(fromList) {
		t.Fatalf("summary shops = %#v list shops = %#v", fromSummary, fromList)
	}
	var quantity, count, cost int64
	for name, want := range fromList {
		got := fromSummary[name]
		if got != want {
			t.Fatalf("shop %s summary = %+v list = %+v", name, got, want)
		}
		quantity += want.quantity
		count += want.count
		cost += want.cost
	}
	totals := summary["totals"].(map[string]any)
	if asInt(totals["quantity"]) != quantity || asInt(totals["record_count"]) != count || asInt(totals["cost_amount_cents"]) != cost {
		t.Fatalf("totals = %#v want %d/%d/%d", totals, quantity, count, cost)
	}
	if quantity != 7 || count != 3 || cost != 500 {
		t.Fatalf("expected sales 7/3/500, got %d/%d/%d", quantity, count, cost)
	}

	byDate, isError, raw := callMCP(t, router, token.secret, "summarize_movements", map[string]any{
		"type": "sales_outbound", "group_by": []string{"date"}, "from": "2026-01-02", "to": "2026-01-02",
	})
	if isError {
		t.Fatalf("date summary error: %s", raw)
	}
	dateItems := byDate["items"].([]any)
	if len(dateItems) != 1 || dateItems[0].(map[string]any)["date"] != "2026-01-02" || asInt(dateItems[0].(map[string]any)["quantity"]) != 7 {
		t.Fatalf("shanghai date summary = %#v", dateItems)
	}
}

func Test_MCP_batch_outbound_is_all_or_nothing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, token, router := openMCPDB(t)
	shop := seedMCPShop(t, db, "总店")
	short := seedMCPProduct(t, db, "缺货", "BR-1213G-4", 100, 1)
	okProduct := seedMCPProduct(t, db, "充足", "BR-1214G-3", 200, 8)

	rejected, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound_batch", map[string]any{
		"shop_query": "总店",
		"items": []map[string]any{
			{"product_code": okProduct.Code, "quantity": 2},
			{"product_code": short.Code, "quantity": 5},
		},
	})
	if isError || rejected["ok"] != false {
		t.Fatalf("batch rejection = %#v error=%t raw=%s", rejected, isError, raw)
	}
	foundShort := false
	for _, item := range rejected["items"].([]any) {
		row := item.(map[string]any)
		if row["product_code"] == short.Code {
			foundShort = row["error"] == "库存不足" && asInt(row["available"]) == 1 && asInt(row["requested"]) == 5
		}
	}
	if !foundShort {
		t.Fatalf("missing stock failure: %#v", rejected["items"])
	}
	if countMCP(t, db, &models.StockMovement{}) != 0 || snapshotQty(t, db, short.ID) != 1 || snapshotQty(t, db, okProduct.ID) != 8 {
		t.Fatalf("rejected batch changed stock")
	}

	accepted, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound_batch", map[string]any{
		"shop_id": shop.ID.String(),
		"items": []map[string]any{
			{"product_code": okProduct.Code, "quantity": 2},
			{"product_code": short.Code, "quantity": 1},
		},
	})
	if isError || accepted["ok"] != true || asInt(accepted["total_quantity"]) != 3 || asInt(accepted["total_cost_cents"]) != 500 {
		t.Fatalf("batch success = %#v error=%t raw=%s", accepted, isError, raw)
	}
	if countMCP(t, db, &models.StockMovement{}) != 2 || snapshotQty(t, db, okProduct.ID) != 6 || snapshotQty(t, db, short.ID) != 0 {
		t.Fatalf("batch quantities ok=%d short=%d movements=%d", snapshotQty(t, db, okProduct.ID), snapshotQty(t, db, short.ID), countMCP(t, db, &models.StockMovement{}))
	}

	dry, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound_batch", map[string]any{
		"shop_query": "总店", "dry_run": true,
		"items": []map[string]any{{"product_code": okProduct.Code, "quantity": 1}},
	})
	if isError || dry["dry_run"] != true || asInt(dry["items"].([]any)[0].(map[string]any)["quantity_after"]) != 5 {
		t.Fatalf("dry run = %#v raw=%s", dry, raw)
	}
	if snapshotQty(t, db, okProduct.ID) != 6 || countMCP(t, db, &models.StockMovement{}) != 2 {
		t.Fatalf("dry run wrote stock")
	}
}

func Test_MCP_request_id_does_not_apply_twice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, token, router := openMCPDB(t)
	seedMCPShop(t, db, "总店")
	product := seedMCPProduct(t, db, "绿茶", "TEA-GREEN", 100, 10)
	args := map[string]any{"product_code": "TEA-GREEN", "shop_query": "总店", "quantity": 2, "request_id": "req-1"}

	first, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound", args)
	if isError || first["duplicate"] == true || asInt(first["quantity_after"]) != 8 {
		t.Fatalf("first outbound = %#v raw=%s", first, raw)
	}
	second, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound", args)
	if isError || second["duplicate"] != true || asInt(second["quantity_after"]) != 8 || snapshotQty(t, db, product.ID) != 8 {
		t.Fatalf("replay = %#v qty=%d raw=%s", second, snapshotQty(t, db, product.ID), raw)
	}
	args["quantity"] = 3
	conflict, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound", args)
	if !isError || !strings.Contains(raw, "IDEMPOTENCY_CONFLICT") || snapshotQty(t, db, product.ID) != 8 {
		t.Fatalf("conflict = %#v error=%t qty=%d raw=%s", conflict, isError, snapshotQty(t, db, product.ID), raw)
	}

	if err := db.Model(&models.MCPIdempotencyKey{}).Where("request_id = ?", "req-1").Update("created_at", time.Now().Add(-6*24*time.Hour)).Error; err != nil {
		t.Fatalf("age request: %v", err)
	}
	args["quantity"] = 2
	kept, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound", args)
	if isError || kept["duplicate"] != true || snapshotQty(t, db, product.ID) != 8 {
		t.Fatalf("six-day request was not retained: %#v qty=%d raw=%s", kept, snapshotQty(t, db, product.ID), raw)
	}
	if err := db.Model(&models.MCPIdempotencyKey{}).Where("request_id = ?", "req-1").Update("created_at", time.Now().Add(-8*24*time.Hour)).Error; err != nil {
		t.Fatalf("age request: %v", err)
	}
	args["quantity"] = 2
	retried, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound", args)
	if isError || retried["duplicate"] == true || snapshotQty(t, db, product.ID) != 6 {
		t.Fatalf("expired request replayed instead of writing: %#v qty=%d raw=%s", retried, snapshotQty(t, db, product.ID), raw)
	}
}

func Test_MCP_inventory_codes_and_verbose_shape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, token, router := openMCPDB(t)
	seedMCPProduct(t, db, "绿茶", "BR-1213G-4", 100, 4)
	seedMCPProduct(t, db, "红茶", "OTHER", 80, 2)

	payload, isError, raw := callMCP(t, router, token.secret, "get_inventory", map[string]any{
		"codes": []string{"BR-1213G-4", "ORIENT-LOU-3"},
	})
	if isError {
		t.Fatalf("codes error: %s", raw)
	}
	items := payload["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("codes items = %#v", items)
	}
	found := items[0].(map[string]any)
	missing := items[1].(map[string]any)
	if found["product_code"] != "BR-1213G-4" || asInt(found["quantity"]) != 4 || found["not_found"] == true || strings.Contains(raw, "ImagePath") {
		t.Fatalf("found code row = %#v raw=%s", found, raw)
	}
	if missing["product_code"] != "ORIENT-LOU-3" || missing["not_found"] != true {
		t.Fatalf("missing code row = %#v", missing)
	}

	union, isError, raw := callMCP(t, router, token.secret, "get_inventory", map[string]any{
		"q": "红茶", "codes": []string{"BR-1213G-4", "MISSING"},
	})
	if isError {
		t.Fatalf("union error: %s", raw)
	}
	codes := map[string]bool{}
	for _, item := range union["items"].([]any) {
		row := item.(map[string]any)
		codes[row["product_code"].(string)] = row["not_found"] == true
	}
	if len(codes) != 3 || codes["OTHER"] || codes["BR-1213G-4"] || !codes["MISSING"] {
		t.Fatalf("union = %#v", codes)
	}

	verbose, isError, raw := callMCP(t, router, token.secret, "get_inventory", map[string]any{"q": "绿茶", "verbose": true})
	if isError {
		t.Fatalf("verbose error: %s", raw)
	}
	row := verbose["items"].([]any)[0].(map[string]any)
	product, _ := row["product"].(map[string]any)
	if product == nil || product["Code"] != "BR-1213G-4" || row["inventory_value_cents"] == nil {
		t.Fatalf("verbose inventory = %#v", row)
	}
}

func Test_MCP_verbose_write_matches_previous_shape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, token, router := openMCPDB(t)
	seedMCPShop(t, db, "总店")
	seedMCPProduct(t, db, "绿茶", "TEA-GREEN", 100, 5)
	payload, isError, raw := callMCP(t, router, token.secret, "create_sales_outbound", map[string]any{
		"product_code": "TEA-GREEN", "shop_query": "总店", "quantity": 1, "verbose": true,
	})
	if isError {
		t.Fatalf("verbose outbound error: %s", raw)
	}
	product, _ := payload["product"].(map[string]any)
	shop, _ := payload["shop"].(map[string]any)
	if product["Code"] != "TEA-GREEN" || shop["Name"] != "总店" || asInt(payload["quantity_after"]) != 4 || payload["product_code"] != nil {
		t.Fatalf("verbose outbound = %#v", payload)
	}
	list := doBearerJSON(t, router, http.MethodPost, "/api/v1/mcp", token.secret, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{},
	})
	body := list.Body.String()
	for _, fragment := range []string{"summarize_movements", "create_sales_outbound_batch", "request_id", "单位为分", "上海时区", "最多 1000"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("tools/list missing %q", fragment)
		}
	}
}

type mcpToken struct {
	secret string
	userID uuid.UUID
}

func openMCPDB(t *testing.T) (*gorm.DB, mcpToken, http.Handler) {
	t.Helper()
	db := openHandlerTestDB(t, append(authModels(), &models.Product{}, &models.Shop{}, &models.InventorySnapshot{}, &models.StockMovement{}, &models.MCPIdempotencyKey{})...)
	admin := createTestUser(t, db, "Admin", "mcp-admin@example.com", "password123", models.RoleAdmin)
	router := apihttp.NewRouter(testConfig(), db)
	session := createSessionToken(t, db, admin.ID)
	created := doJSON(t, router, http.MethodPut, "/api/v1/auth/api-token", session, map[string]any{})
	var body struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil || body.Secret == "" {
		t.Fatalf("create token: %v body=%s", err, created.Body.String())
	}
	return db, mcpToken{secret: body.Secret, userID: admin.ID}, router
}

func seedMCPProduct(t *testing.T, db *gorm.DB, name string, code string, price int64, quantity int64) models.Product {
	t.Helper()
	product := models.Product{Name: name, Code: code, Enabled: true, DefaultPurchaseCents: price, ImagePath: "private/" + code + ".png"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: product.ID, Quantity: quantity}).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	return product
}

func seedMCPShop(t *testing.T, db *gorm.DB, name string) models.Shop {
	t.Helper()
	shop := models.Shop{Name: name, Enabled: true}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("seed shop: %v", err)
	}
	return shop
}

func seedMCPMovement(t *testing.T, db *gorm.DB, movement models.StockMovement) models.StockMovement {
	t.Helper()
	if err := db.Create(&movement).Error; err != nil {
		t.Fatalf("seed movement: %v", err)
	}
	return movement
}

func callMCP(t *testing.T, router http.Handler, token string, name string, args map[string]any) (map[string]any, bool, string) {
	t.Helper()
	response := doBearerJSON(t, router, http.MethodPost, "/api/v1/mcp", token, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("mcp status = %d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Result struct {
			IsError           bool           `json:"isError"`
			StructuredContent map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode mcp: %v body=%s", err, response.Body.String())
	}
	return envelope.Result.StructuredContent, envelope.Result.IsError, response.Body.String()
}

func asInt(value any) int64 {
	switch n := value.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

func snapshotQty(t *testing.T, db *gorm.DB, productID uuid.UUID) int64 {
	t.Helper()
	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", productID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	return snapshot.Quantity
}

func countMCP(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return count
}
