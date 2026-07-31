package services

import (
	"strings"
	"testing"
	"time"

	"gaowang/apps/api/internal/models"
)

func Test_Lark_inventory_summary_uses_current_prices_and_includes_missing_snapshots(t *testing.T) {
	db := newInventoryTestDB(t)
	archivedAt := time.Now()
	products := []models.Product{
		{Name: "茶 A", Code: "TEA-A", DefaultPurchaseCents: 100, LowStockThreshold: 2, Enabled: true},
		{Name: "茶 B", Code: "TEA-B", DefaultPurchaseCents: 250, LowStockThreshold: 1, Enabled: true},
		{Name: "茶 C", Code: "TEA-C", DefaultPurchaseCents: 100, Enabled: true},
		{Name: "已归档", Code: "OLD", DefaultPurchaseCents: 1000, Enabled: false, ArchivedAt: &archivedAt},
	}
	for index := range products {
		if err := db.Create(&products[index]).Error; err != nil {
			t.Fatalf("create product %d: %v", index, err)
		}
	}
	for _, snapshot := range []models.InventorySnapshot{
		{ProductID: products[0].ID, Quantity: 2},
		{ProductID: products[2].ID, Quantity: 3},
		{ProductID: products[3].ID, Quantity: 100},
	} {
		if err := db.Create(&snapshot).Error; err != nil {
			t.Fatalf("create snapshot: %v", err)
		}
	}

	summary, err := queryLarkInventorySummary(db)
	if err != nil {
		t.Fatalf("query summary: %v", err)
	}
	if summary.ProductCount != 3 || summary.Quantity != 5 || summary.ValueCents != 500 ||
		summary.LowStockCount != 2 || summary.ZeroStockCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	card, err := larkInventorySummaryCard(summary)
	if err != nil || !strings.Contains(card, `"template":"orange"`) || !strings.Contains(card, "¥5.00") {
		t.Fatalf("summary card = %s err=%v", card, err)
	}
}

func Test_Lark_movements_filter_sort_limit_and_render_each_product_image(t *testing.T) {
	db := newInventoryTestDB(t)
	operator := models.User{Name: "操作员", Email: "lark-movement@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "一号店", Enabled: true}
	tea := models.Product{Name: "绿茶", Code: "TEA-1", Enabled: true}
	coffee := models.Product{Name: "咖啡", Code: "COFFEE-1", Enabled: true}
	for _, value := range []any{&operator, &shop, &tea, &coffee} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("create fixture: %v", err)
		}
	}
	base := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 6; index++ {
		movement := models.StockMovement{
			Type:          models.MovementTypeInbound,
			ProductID:     tea.ID,
			ShopID:        &shop.ID,
			QuantityDelta: int64(index + 1),
			OperatorID:    operator.ID,
			CreatedAt:     base.Add(time.Duration(index) * time.Minute),
		}
		if err := db.Create(&movement).Error; err != nil {
			t.Fatalf("create movement %d: %v", index, err)
		}
	}
	other := models.StockMovement{
		Type: models.MovementTypeAdjustment, ProductID: coffee.ID, QuantityDelta: 1,
		OperatorID: operator.ID, CreatedAt: base.Add(10 * time.Minute),
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other movement: %v", err)
	}

	movements, more, err := queryLarkMovements(db, "tea-1")
	if err != nil {
		t.Fatalf("query movements: %v", err)
	}
	if len(movements) != 5 || !more || movements[0].QuantityDelta != 6 || movements[4].QuantityDelta != 2 {
		t.Fatalf("movements = %+v more=%t", movements, more)
	}
	for _, movement := range movements {
		if movement.Product.Code != "TEA-1" || movement.Operator.Name != "操作员" || movement.Shop == nil || movement.Shop.Name != "一号店" {
			t.Fatalf("movement associations = %+v", movement)
		}
	}
	card, err := larkMovementsCard(movements, more, []string{"img_1", "img_2", "img_3", "img_4", "img_5"})
	if err != nil || strings.Count(card, `"img_key":"img_`) != 5 || strings.Count(card, `"tag":"hr"`) != 4 ||
		!strings.Contains(card, "结果超过 5 条") || strings.Contains(card, "fit_horizontal") {
		t.Fatalf("movement card = %s err=%v", card, err)
	}
}

func Test_Lark_today_changes_uses_shanghai_day_boundaries(t *testing.T) {
	db := newInventoryTestDB(t)
	operator := models.User{Name: "操作员", Email: "lark-today@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	product := models.Product{Name: "红茶", Code: "TEA-2", Enabled: true}
	for _, value := range []any{&operator, &product} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("create fixture: %v", err)
		}
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, larkLocation)
	startUTC := time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
	fixtures := []models.StockMovement{
		{Type: models.MovementTypeInbound, QuantityDelta: 99, CreatedAt: startUTC.Add(-time.Nanosecond)},
		{Type: models.MovementTypeInbound, QuantityDelta: 5, CreatedAt: startUTC},
		{Type: models.MovementTypeSalesOutbound, QuantityDelta: -2, CreatedAt: startUTC.Add(time.Hour)},
		{Type: models.MovementTypeAdjustment, QuantityDelta: 3, CreatedAt: startUTC.Add(2 * time.Hour)},
		{Type: models.MovementTypeAdjustment, QuantityDelta: -1, CreatedAt: startUTC.Add(3 * time.Hour)},
		{Type: models.MovementTypeSalesOutbound, QuantityDelta: -88, CreatedAt: startUTC.Add(24 * time.Hour)},
	}
	for index := range fixtures {
		fixtures[index].ProductID = product.ID
		fixtures[index].OperatorID = operator.ID
		if err := db.Create(&fixtures[index]).Error; err != nil {
			t.Fatalf("create movement %d: %v", index, err)
		}
	}

	changes, err := queryLarkTodayChanges(db, now)
	if err != nil {
		t.Fatalf("query today changes: %v", err)
	}
	if changes.InboundCount != 1 || changes.InboundQuantity != 5 ||
		changes.OutboundCount != 1 || changes.OutboundQuantity != 2 ||
		changes.AdjustmentCount != 2 || changes.AdjustmentQuantity != 2 {
		t.Fatalf("today changes = %+v", changes)
	}
}

func Test_Lark_empty_queries_return_zero_or_empty_cards(t *testing.T) {
	db := newInventoryTestDB(t)
	summary, err := queryLarkInventorySummary(db)
	if err != nil || summary != (larkInventorySummary{}) {
		t.Fatalf("empty summary = %+v err=%v", summary, err)
	}
	movements, more, err := queryLarkMovements(db, "missing")
	if err != nil || more || len(movements) != 0 {
		t.Fatalf("empty movements = %+v more=%t err=%v", movements, more, err)
	}
	movementCard, err := larkMovementsCard(movements, more, nil)
	if err != nil || !strings.Contains(movementCard, "没有找到匹配的流水记录") {
		t.Fatalf("empty movement card = %s err=%v", movementCard, err)
	}
	changes, err := queryLarkTodayChanges(db, time.Now())
	if err != nil || changes != (larkTodayChanges{}) {
		t.Fatalf("empty today changes = %+v err=%v", changes, err)
	}
}

func Test_Lark_multi_product_card_keeps_each_image_adjacent_and_degrades_one_image(t *testing.T) {
	rows := []larkProductRow{
		{Product: models.Product{Name: "红茶", Code: "TEA-RED", Enabled: true}, Quantity: 3},
		{Product: models.Product{Name: "绿茶", Code: "TEA-GREEN", Enabled: true}, Quantity: 4},
	}
	card, err := larkQueryCard(larkCommand{Action: larkActionInventory}, rows, false, []string{"img_red", ""})
	if err != nil || !strings.Contains(card, "红茶") || !strings.Contains(card, "绿茶") ||
		strings.Count(card, `"img_key":"img_red"`) != 1 || strings.Count(card, `"tag":"hr"`) != 1 || strings.Contains(card, "fit_horizontal") {
		t.Fatalf("multi-product card = %s err=%v", card, err)
	}
}
