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

func Test_Lark_today_sales_ranking_uses_shanghai_day_and_renders_each_product_image(t *testing.T) {
	db := newInventoryTestDB(t)
	operator := models.User{Name: "操作员", Email: "lark-ranking@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, larkLocation)
	startUTC := time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
	products := make([]models.Product, 7)
	for index := range products {
		products[index] = models.Product{
			Name: "销售商品" + string(rune('A'+index)), Code: "SALE-" + string(rune('A'+index)),
			DefaultPurchaseCents: 100, Enabled: true, ImagePath: "/uploads/sale.png",
		}
		if err := db.Create(&products[index]).Error; err != nil {
			t.Fatalf("create product %d: %v", index, err)
		}
		if err := db.Create(&models.InventorySnapshot{ProductID: products[index].ID, Quantity: int64(10 + index)}).Error; err != nil {
			t.Fatalf("create snapshot %d: %v", index, err)
		}
		movement := models.StockMovement{
			Type: models.MovementTypeSalesOutbound, ProductID: products[index].ID,
			QuantityDelta: -int64(index + 1), OperatorID: operator.ID, CreatedAt: startUTC.Add(time.Duration(index+1) * time.Hour),
		}
		if err := db.Create(&movement).Error; err != nil {
			t.Fatalf("create sale %d: %v", index, err)
		}
	}
	ignored := []models.StockMovement{
		{Type: models.MovementTypeSalesOutbound, ProductID: products[0].ID, QuantityDelta: -99, OperatorID: operator.ID, CreatedAt: startUTC.Add(-time.Nanosecond)},
		{Type: models.MovementTypeSalesOutbound, ProductID: products[6].ID, QuantityDelta: -99, OperatorID: operator.ID, CreatedAt: startUTC.Add(24 * time.Hour)},
		{Type: models.MovementTypeInbound, ProductID: products[0].ID, QuantityDelta: 88, OperatorID: operator.ID, CreatedAt: startUTC.Add(time.Hour)},
	}
	for index := range ignored {
		if err := db.Create(&ignored[index]).Error; err != nil {
			t.Fatalf("create ignored movement %d: %v", index, err)
		}
	}
	archivedAt := now
	archived := models.Product{Name: "已归档热销品", Code: "SALE-OLD", DefaultPurchaseCents: 100, Enabled: false, ArchivedAt: &archivedAt}
	if err := db.Create(&archived).Error; err != nil {
		t.Fatalf("create archived product: %v", err)
	}
	if err := db.Create(&models.StockMovement{
		Type: models.MovementTypeSalesOutbound, ProductID: archived.ID, QuantityDelta: -100,
		OperatorID: operator.ID, CreatedAt: startUTC.Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create archived sale: %v", err)
	}

	rows, more, err := queryLarkTodaySalesRanking(db, now)
	if err != nil || !more || len(rows) != 5 {
		t.Fatalf("sales ranking rows = %+v more=%t err=%v", rows, more, err)
	}
	for index, want := range []string{"SALE-G", "SALE-F", "SALE-E", "SALE-D", "SALE-C"} {
		if rows[index].Code != want || rows[index].SalesQuantity != int64(7-index) {
			t.Fatalf("sales ranking row %d = %+v, want code=%s quantity=%d", index, rows[index], want, 7-index)
		}
	}
	card, err := larkQueryCard(
		larkCommand{Action: larkActionTodaySalesRanking},
		rows,
		more,
		[]string{"img_g", "img_f", "img_e", "img_d", "img_c"},
	)
	if err != nil || !strings.Contains(card, "今日销售排行") || strings.Count(card, "**今日售出**") != 5 ||
		strings.Count(card, `"img_key":"img_`) != 5 || !strings.Contains(card, "库存数量：16 件") ||
		strings.Contains(card, "售价") || strings.Contains(card, "fit_horizontal") {
		t.Fatalf("sales ranking card = %s err=%v", card, err)
	}
}

func Test_Lark_additional_product_cards_keep_images_and_semantic_titles(t *testing.T) {
	rows := []larkProductRow{
		{Product: models.Product{Name: "缺货茶", Code: "ZERO-1", Enabled: true}, Quantity: 0},
		{Product: models.Product{Name: "有货茶", Code: "VALUE-1", Enabled: true, DefaultPurchaseCents: 100}, Quantity: 5},
	}
	tests := []struct {
		action   string
		title    string
		template string
	}{
		{action: larkActionOutOfStock, title: "缺货清单", template: "red"},
		{action: larkActionValueRanking, title: "库存金额排行", template: "blue"},
	}
	for _, test := range tests {
		card, err := larkQueryCard(larkCommand{Action: test.action}, rows, false, []string{"img_zero", "img_value"})
		if err != nil || !strings.Contains(card, test.title) || !strings.Contains(card, `"template":"`+test.template+`"`) ||
			strings.Count(card, `"img_key":"img_`) != 2 {
			t.Fatalf("%s card = %s err=%v", test.action, card, err)
		}
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
	sales, more, err := queryLarkTodaySalesRanking(db, time.Now())
	if err != nil || more || len(sales) != 0 {
		t.Fatalf("empty sales ranking = %+v more=%t err=%v", sales, more, err)
	}
	salesCard, err := larkQueryCard(larkCommand{Action: larkActionTodaySalesRanking}, sales, more, nil)
	if err != nil || !strings.Contains(salesCard, "今天还没有销售出库记录") {
		t.Fatalf("empty sales card = %s err=%v", salesCard, err)
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
