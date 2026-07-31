package services

import (
	"strings"
	"testing"
	"time"

	"gaowang/apps/api/internal/models"
)

func Test_Lark_analytics_shop_outbound_ranking_keeps_the_requested_dimension(t *testing.T) {
	db := newInventoryTestDB(t)
	operator := models.User{Name: "操作员", Email: "analytics-shop@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shops := []models.Shop{
		{Name: "A店", Enabled: true},
		{Name: "B店", Enabled: true},
		{Name: "C店", Enabled: true},
	}
	product := models.Product{Name: "手表", Code: "WATCH-1", DefaultPurchaseCents: 200, Enabled: true}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := db.Create(&shops).Error; err != nil {
		t.Fatalf("create shops: %v", err)
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	movements := []models.StockMovement{
		{Type: models.MovementTypeSalesOutbound, ProductID: product.ID, ShopID: &shops[0].ID, QuantityDelta: -8},
		{Type: models.MovementTypeSalesOutbound, ProductID: product.ID, ShopID: &shops[1].ID, QuantityDelta: -6},
		{Type: models.MovementTypeSalesOutbound, ProductID: product.ID, ShopID: &shops[1].ID, QuantityDelta: -4},
		{Type: models.MovementTypeInbound, ProductID: product.ID, ShopID: &shops[0].ID, QuantityDelta: 100},
		{Type: models.MovementTypeAdjustment, ProductID: product.ID, ShopID: &shops[2].ID, QuantityDelta: 50},
		{Type: models.MovementTypeSalesOutbound, ProductID: product.ID, QuantityDelta: -99},
	}
	for index := range movements {
		movements[index].OperatorID = operator.ID
		movements[index].CreatedAt = time.Date(2026, 1, index+1, 12, 0, 0, 0, time.UTC)
		if err := db.Create(&movements[index]).Error; err != nil {
			t.Fatalf("create movement %d: %v", index, err)
		}
	}

	plan := larkAnalyticsPlan{
		Metric: larkMetricMovementQuantity, GroupBy: larkGroupShop,
		MovementType: larkMovementOutbound, TimeRange: larkTimeAll, Sort: "desc", Limit: 1,
	}
	rows, more, err := queryLarkAnalytics(db, plan, time.Now())
	if err != nil || !more || len(rows) != 1 || rows[0].Name != "B店" || rows[0].MetricValue != 10 {
		t.Fatalf("shop ranking = %+v more=%t err=%v", rows, more, err)
	}
	card, err := larkAnalyticsCard(plan, rows, more, nil)
	if err != nil || !strings.Contains(card, "全部历史销售出库数量 · 店铺排行") ||
		!strings.Contains(card, "B店") || !strings.Contains(card, "10 件") || strings.Contains(card, "商品排行") {
		t.Fatalf("shop ranking card = %s err=%v", card, err)
	}
}

func Test_Lark_analytics_combines_time_dimension_filters_and_current_prices(t *testing.T) {
	db := newInventoryTestDB(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, larkLocation)
	today := time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
	users := []models.User{
		{Name: "张三", Email: "zhang@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true},
		{Name: "李四", Email: "li@example.com", PasswordHash: "hash", Role: models.RoleStaff, Enabled: true},
	}
	shops := []models.Shop{{Name: "A店", Enabled: true}, {Name: "B店", Enabled: true}}
	products := []models.Product{
		{Name: "红茶", Code: "TEA-RED", ImagePath: "/uploads/red.jpg", DefaultPurchaseCents: 200, Enabled: true},
		{Name: "绿茶", Code: "TEA-GREEN", ImagePath: "/uploads/green.jpg", DefaultPurchaseCents: 100, Enabled: true},
	}
	for index := range users {
		if err := db.Create(&users[index]).Error; err != nil {
			t.Fatalf("create user %d: %v", index, err)
		}
	}
	if err := db.Create(&shops).Error; err != nil {
		t.Fatalf("create shops: %v", err)
	}
	for index := range products {
		if err := db.Create(&products[index]).Error; err != nil {
			t.Fatalf("create product %d: %v", index, err)
		}
		if err := db.Create(&models.InventorySnapshot{ProductID: products[index].ID, Quantity: int64(10 + index)}).Error; err != nil {
			t.Fatalf("create snapshot %d: %v", index, err)
		}
	}
	movements := []models.StockMovement{
		{Type: models.MovementTypeSalesOutbound, ProductID: products[0].ID, ShopID: &shops[0].ID, OperatorID: users[0].ID, QuantityDelta: -3, PurchaseUnitCents: int64Pointer(9999), CreatedAt: today.Add(time.Hour)},
		{Type: models.MovementTypeSalesOutbound, ProductID: products[1].ID, ShopID: &shops[1].ID, OperatorID: users[1].ID, QuantityDelta: -5, CreatedAt: today.Add(2 * time.Hour)},
		{Type: models.MovementTypeSalesOutbound, ProductID: products[0].ID, ShopID: &shops[0].ID, OperatorID: users[0].ID, QuantityDelta: -4, CreatedAt: today.Add(-time.Hour)},
		{Type: models.MovementTypeInbound, ProductID: products[0].ID, ShopID: &shops[0].ID, OperatorID: users[0].ID, QuantityDelta: 10, CreatedAt: today.AddDate(0, 0, -6).Add(time.Hour)},
		{Type: models.MovementTypeInbound, ProductID: products[1].ID, ShopID: &shops[1].ID, OperatorID: users[1].ID, QuantityDelta: 20, CreatedAt: today.AddDate(0, 0, -7).Add(-time.Hour)},
		{Type: models.MovementTypeAdjustment, ProductID: products[1].ID, ShopID: &shops[1].ID, OperatorID: users[1].ID, QuantityDelta: 2, CreatedAt: time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)},
		{Type: models.MovementTypeAdjustment, ProductID: products[1].ID, ShopID: &shops[1].ID, OperatorID: users[1].ID, QuantityDelta: -1, CreatedAt: today.Add(3 * time.Hour)},
	}
	for index := range movements {
		if err := db.Create(&movements[index]).Error; err != nil {
			t.Fatalf("create movement %d: %v", index, err)
		}
	}

	t.Run("today product ranking has each image", func(t *testing.T) {
		plan := larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupProduct, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Sort: "desc", Limit: 5}
		rows, more, err := queryLarkAnalytics(db, plan, now)
		if err != nil || more || len(rows) != 2 || rows[0].Code != "TEA-GREEN" || rows[0].MetricValue != 5 || rows[1].MetricValue != 3 {
			t.Fatalf("product ranking = %+v more=%t err=%v", rows, more, err)
		}
		card, err := larkAnalyticsCard(plan, rows, more, []string{"img_green", "img_red"})
		if err != nil || strings.Count(card, `"img_key":"img_`) != 2 || !strings.Contains(card, "今日销售出库数量 · 商品排行") ||
			!strings.Contains(card, "当前库存") || strings.Contains(card, "售价") {
			t.Fatalf("product analytics card = %s err=%v", card, err)
		}
	})

	t.Run("last seven days excludes older inbound", func(t *testing.T) {
		plan := larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupShop, MovementType: larkMovementInbound, TimeRange: larkTimeLastNDays, Days: 7, Sort: "desc", Limit: 5}
		rows, more, err := queryLarkAnalytics(db, plan, now)
		if err != nil || more || len(rows) != 1 || rows[0].Name != "A店" || rows[0].MetricValue != 10 {
			t.Fatalf("seven-day inbound = %+v more=%t err=%v", rows, more, err)
		}
	})

	t.Run("operator filter and shop filter compose", func(t *testing.T) {
		operatorPlan := larkAnalyticsPlan{Metric: larkMetricMovementQuantity, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, OperatorKeyword: "张三"}
		rows, more, err := queryLarkAnalytics(db, operatorPlan, now)
		if err != nil || more || len(rows) != 1 || rows[0].MetricValue != 3 {
			t.Fatalf("operator total = %+v more=%t err=%v", rows, more, err)
		}

		shopPlan := larkAnalyticsPlan{Metric: larkMetricMovementValue, MovementType: larkMovementOutbound, TimeRange: larkTimeAll, ShopKeyword: "A店"}
		rows, more, err = queryLarkAnalytics(db, shopPlan, now)
		if err != nil || more || len(rows) != 1 || rows[0].MetricValue != 1400 {
			t.Fatalf("shop value = %+v more=%t err=%v", rows, more, err)
		}
	})

	t.Run("current month operator count", func(t *testing.T) {
		plan := larkAnalyticsPlan{Metric: larkMetricMovementCount, GroupBy: larkGroupOperator, MovementType: larkMovementAll, TimeRange: larkTimeCurrentMonth, Sort: "desc", Limit: 5}
		rows, more, err := queryLarkAnalytics(db, plan, now)
		if err != nil || more || len(rows) != 2 || rows[0].Name != "李四" || rows[0].MetricValue != 4 || rows[1].MetricValue != 3 {
			t.Fatalf("operator count = %+v more=%t err=%v", rows, more, err)
		}
	})
}

func Test_Lark_inventory_analytics_uses_current_purchase_price_and_missing_snapshots(t *testing.T) {
	db := newInventoryTestDB(t)
	products := []models.Product{
		{Name: "商品A", Code: "A", ImagePath: "/uploads/a.jpg", DefaultPurchaseCents: 300, Enabled: true},
		{Name: "商品B", Code: "B", ImagePath: "/uploads/b.jpg", DefaultPurchaseCents: 100, Enabled: true},
		{Name: "无快照", Code: "ZERO", ImagePath: "/uploads/zero.jpg", DefaultPurchaseCents: 999, Enabled: true},
	}
	for index := range products {
		if err := db.Create(&products[index]).Error; err != nil {
			t.Fatalf("create product %d: %v", index, err)
		}
	}
	for index, quantity := range []int64{2, 10} {
		if err := db.Create(&models.InventorySnapshot{ProductID: products[index].ID, Quantity: quantity}).Error; err != nil {
			t.Fatalf("create snapshot %d: %v", index, err)
		}
	}
	archivedAt := time.Now()
	archived := models.Product{Name: "归档", Code: "OLD", DefaultPurchaseCents: 99999, Enabled: false, ArchivedAt: &archivedAt}
	if err := db.Create(&archived).Error; err != nil {
		t.Fatalf("create archived product: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: archived.ID, Quantity: 100}).Error; err != nil {
		t.Fatalf("create archived snapshot: %v", err)
	}

	totalPlan := larkAnalyticsPlan{Metric: larkMetricInventoryValue}
	rows, more, err := queryLarkAnalytics(db, totalPlan, time.Now())
	if err != nil || more || len(rows) != 1 || rows[0].MetricValue != 1600 {
		t.Fatalf("inventory value total = %+v more=%t err=%v", rows, more, err)
	}

	rankingPlan := larkAnalyticsPlan{Metric: larkMetricInventoryValue, GroupBy: larkGroupProduct, Sort: "desc", Limit: 2}
	rows, more, err = queryLarkAnalytics(db, rankingPlan, time.Now())
	if err != nil || !more || len(rows) != 2 || rows[0].Code != "B" || rows[0].MetricValue != 1000 || rows[1].Code != "A" || rows[1].MetricValue != 600 {
		t.Fatalf("inventory ranking = %+v more=%t err=%v", rows, more, err)
	}
	card, err := larkAnalyticsCard(rankingPlan, rows, more, []string{"img_b", "img_a"})
	if err != nil || strings.Count(card, `"img_key":"img_`) != 2 || !strings.Contains(card, "当前库存金额 · 商品排行") || !strings.Contains(card, "¥10.00") {
		t.Fatalf("inventory analytics card = %s err=%v", card, err)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
