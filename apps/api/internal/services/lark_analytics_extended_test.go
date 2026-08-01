package services

import (
	"strings"
	"testing"
	"time"

	"gaowang/apps/api/internal/models"
)

func Test_Lark_analytics_supports_two_dimensions_details_trend_share_and_comparison(t *testing.T) {
	db := newInventoryTestDB(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, larkLocation)
	today := time.Date(2026, 8, 1, 0, 0, 0, 0, larkLocation).UTC()
	deletedAt := now
	users := []models.User{
		{Name: "张三", Email: "zhang-extended@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true},
		{Name: "李四", Email: "li-extended@example.com", PasswordHash: "hash", Role: models.RoleStaff, Enabled: false, DeletedAt: &deletedAt},
	}
	shops := []models.Shop{{Name: "晴朗店", Note: "主店", Enabled: true}, {Name: "多云店", Note: "分店", Enabled: true}}
	products := []models.Product{
		{Name: "绿茶", Code: "TEA", ImagePath: "/uploads/tea.jpg", Note: "清香", DefaultPurchaseCents: 200, Enabled: true},
		{Name: "咖啡", Code: "COFFEE", ImagePath: "/uploads/coffee.jpg", DefaultPurchaseCents: 100, Enabled: true},
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
		if err := db.Create(&models.InventorySnapshot{ProductID: products[index].ID, Quantity: int64(20 + index)}).Error; err != nil {
			t.Fatalf("create snapshot %d: %v", index, err)
		}
	}
	movements := []models.StockMovement{
		{Type: models.MovementTypeSalesOutbound, ProductID: products[0].ID, ShopID: &shops[0].ID, OperatorID: users[0].ID, QuantityDelta: -3, CreatedAt: today.Add(time.Hour)},
		{Type: models.MovementTypeSalesOutbound, ProductID: products[0].ID, ShopID: &shops[1].ID, OperatorID: users[1].ID, QuantityDelta: -2, Reason: "客户自提", CreatedAt: today.Add(2 * time.Hour)},
		{Type: models.MovementTypeSalesOutbound, ProductID: products[1].ID, ShopID: &shops[0].ID, OperatorID: users[1].ID, QuantityDelta: -5, CreatedAt: today.Add(3 * time.Hour)},
		{Type: models.MovementTypeSalesOutbound, ProductID: products[0].ID, ShopID: &shops[0].ID, OperatorID: users[0].ID, QuantityDelta: -4, CreatedAt: today.AddDate(0, 0, -1).Add(time.Hour)},
		{Type: models.MovementTypeInbound, ProductID: products[1].ID, ShopID: &shops[1].ID, OperatorID: users[1].ID, QuantityDelta: 9, CreatedAt: today.AddDate(0, -1, 0)},
	}
	for index := range movements {
		if err := db.Create(&movements[index]).Error; err != nil {
			t.Fatalf("create movement %d: %v", index, err)
		}
	}

	matrixPlan := larkAnalyticsPlan{
		Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationRanking,
		GroupBy: larkGroupProduct, GroupBy2: larkGroupShop, MovementType: larkMovementOutbound,
		TimeRange: larkTimeToday, Sort: "desc", Limit: 10, Presentation: larkPresentationMatrix,
	}
	matrix, more, err := queryLarkAnalytics(db, matrixPlan, now)
	if err != nil || more || len(matrix) != 3 || matrix[0].Name != "咖啡" || matrix[0].Name2 != "晴朗店" || matrix[0].MetricValue != 5 {
		t.Fatalf("matrix = %+v more=%t err=%v", matrix, more, err)
	}
	matrixCard, err := larkAnalyticsCard(matrixPlan, matrix, more, []string{"coffee", "tea", "tea"})
	if err != nil || !strings.Contains(matrixCard, "商品 × 店铺") || !strings.Contains(matrixCard, "咖啡 → 晴朗店") || !strings.Contains(matrixCard, "5 件") {
		t.Fatalf("matrix card = %s err=%v", matrixCard, err)
	}

	sharePlan := larkAnalyticsPlan{
		Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationShare,
		GroupBy: larkGroupShop, MovementType: larkMovementOutbound, TimeRange: larkTimeToday,
		Sort: "desc", Limit: 10, Presentation: larkPresentationShare,
	}
	shares, more, err := queryLarkAnalytics(db, sharePlan, now)
	if err != nil || more || len(shares) != 2 || shares[0].Name != "晴朗店" || shares[0].MetricValue != 8 ||
		shares[0].ShareBasisPoints != 8000 || shares[1].ShareBasisPoints != 2000 {
		t.Fatalf("shares = %+v more=%t err=%v", shares, more, err)
	}
	shareCard, err := larkAnalyticsCard(sharePlan, shares, more, nil)
	if err != nil || !strings.Contains(shareCard, "占比") || !strings.Contains(shareCard, "80.00%") {
		t.Fatalf("share card = %s err=%v", shareCard, err)
	}

	trendPlan := larkAnalyticsPlan{
		Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationTrend,
		MovementType: larkMovementOutbound, TimeRange: larkTimeDateRange, DateFrom: "2026-07-31", DateTo: "2026-08-01",
		TimeBucket: larkBucketDay, Limit: 10, Presentation: larkPresentationTrend,
	}
	trend, more, err := queryLarkAnalytics(db, trendPlan, now)
	if err != nil || more || len(trend) != 2 || trend[0].Name != "2026-07-31" || trend[0].MetricValue != 4 || trend[1].MetricValue != 10 {
		t.Fatalf("trend = %+v more=%t err=%v", trend, more, err)
	}
	trendCard, err := larkAnalyticsCard(trendPlan, trend, more, nil)
	if err != nil || !strings.Contains(trendCard, "趋势") || !strings.Contains(trendCard, "2026-07-31") || !strings.Contains(trendCard, "▰") {
		t.Fatalf("trend card = %s err=%v", trendCard, err)
	}

	comparisonPlan := larkAnalyticsPlan{
		Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationComparison,
		MovementType: larkMovementOutbound, TimeRange: larkTimeToday, CompareTo: larkComparePreviousPeriod,
		Limit: 10, Presentation: larkPresentationComparison,
	}
	comparison, more, err := queryLarkAnalytics(db, comparisonPlan, now)
	if err != nil || more || len(comparison) != 1 || comparison[0].MetricValue != 10 || comparison[0].ReferenceValue != 4 {
		t.Fatalf("comparison = %+v more=%t err=%v", comparison, more, err)
	}
	comparisonCard, err := larkAnalyticsCard(comparisonPlan, comparison, more, nil)
	if err != nil || !strings.Contains(comparisonCard, "对比") || !strings.Contains(comparisonCard, "150.00%") || !strings.Contains(comparisonCard, "参考期") {
		t.Fatalf("comparison card = %s err=%v", comparisonCard, err)
	}

	detailPlan := larkAnalyticsPlan{
		Domain: larkDomainMovement, Operation: larkOperationDetails, MovementType: larkMovementOutbound,
		TimeRange: larkTimeToday, ShopKeyword: "多云", Limit: 10, Presentation: larkPresentationDetail,
	}
	details, more, err := queryLarkAnalytics(db, detailPlan, now)
	if err != nil || more || len(details) != 1 || details[0].Note != "客户自提" || details[0].OperatorName != "李四" {
		t.Fatalf("movement details = %+v more=%t err=%v", details, more, err)
	}
	detailCard, err := larkAnalyticsCard(detailPlan, details, more, []string{"tea"})
	if err != nil || !strings.Contains(detailCard, "流水明细") || !strings.Contains(detailCard, "客户自提") || !strings.Contains(detailCard, "李四") {
		t.Fatalf("detail card = %s err=%v", detailCard, err)
	}

	operatorPlan := larkAnalyticsPlan{Domain: larkDomainOperator, Operation: larkOperationDetails, TimeRange: larkTimeAll, OperatorKeyword: "李四", Limit: 10}
	operators, _, err := queryLarkAnalytics(db, operatorPlan, now)
	if err != nil || len(operators) != 1 || operators[0].Name != "李四" || operators[0].CountValue != 3 {
		t.Fatalf("deleted operator history = %+v err=%v", operators, err)
	}

	productPlan := larkAnalyticsPlan{Domain: larkDomainProduct, Operation: larkOperationDetails, ProductKeyword: "TEA", Limit: 10}
	productRows, _, err := queryLarkAnalytics(db, productPlan, now)
	if err != nil || len(productRows) != 1 || productRows[0].Note != "清香" || productRows[0].CurrentQuantity != 20 {
		t.Fatalf("product details = %+v err=%v", productRows, err)
	}
	productCard, err := larkAnalyticsCard(productPlan, productRows, false, []string{"tea"})
	if err != nil || !strings.Contains(productCard, "商品明细") || !strings.Contains(productCard, "清香") || !strings.Contains(productCard, "20 件") {
		t.Fatalf("product detail card = %s err=%v", productCard, err)
	}

	shopPlan := larkAnalyticsPlan{Domain: larkDomainShop, Operation: larkOperationDetails, ShopKeyword: "晴朗", Limit: 10}
	shopRows, _, err := queryLarkAnalytics(db, shopPlan, now)
	if err != nil || len(shopRows) != 1 || shopRows[0].Note != "主店" {
		t.Fatalf("shop details = %+v err=%v", shopRows, err)
	}
	shopCard, err := larkAnalyticsCard(shopPlan, shopRows, false, nil)
	if err != nil || !strings.Contains(shopCard, "店铺明细") || !strings.Contains(shopCard, "主店") {
		t.Fatalf("shop detail card = %s err=%v", shopCard, err)
	}

	totalPlan := larkAnalyticsPlan{Domain: larkDomainMovement, Metric: larkMetricMovementCount, Operation: larkOperationTotal, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Presentation: larkPresentationSummary}
	total, _, err := queryLarkAnalytics(db, totalPlan, now)
	if err != nil || len(total) != 1 || total[0].MetricValue != 3 {
		t.Fatalf("summary = %+v err=%v", total, err)
	}
	summaryCard, err := larkAnalyticsCard(totalPlan, total, false, nil)
	if err != nil || !strings.Contains(summaryCard, "3 笔") || !strings.Contains(summaryCard, "时间范围") {
		t.Fatalf("summary card = %s err=%v", summaryCard, err)
	}
}

func Test_Lark_analytics_extended_time_ranges_use_shanghai_boundaries(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, larkLocation)
	tests := []struct {
		rangeName string
		from      string
		to        string
		dateFrom  string
		dateTo    string
	}{
		{rangeName: larkTimeCurrentWeek, from: "2026-08-03", to: "2026-08-10"},
		{rangeName: larkTimePreviousWeek, from: "2026-07-27", to: "2026-08-03"},
		{rangeName: larkTimePreviousMonth, from: "2026-07-01", to: "2026-08-01"},
		{rangeName: larkTimeCurrentYear, from: "2026-01-01", to: "2027-01-01"},
		{rangeName: larkTimeDateRange, from: "2026-06-01", to: "2026-07-01", dateFrom: "2026-06-01", dateTo: "2026-06-30"},
	}
	for _, test := range tests {
		plan := larkAnalyticsPlan{TimeRange: test.rangeName, DateFrom: test.dateFrom, DateTo: test.dateTo}
		start, end, bounded := larkAnalyticsRange(plan, now)
		if !bounded || start.In(larkLocation).Format("2006-01-02") != test.from || end.In(larkLocation).Format("2006-01-02") != test.to {
			t.Fatalf("%s range = %s..%s bounded=%t", test.rangeName, start.In(larkLocation), end.In(larkLocation), bounded)
		}
	}
}

func Test_Lark_analytics_rejects_ignored_current_range_fields(t *testing.T) {
	for name, plan := range map[string]larkAnalyticsPlan{
		"inventory metric with days": {
			Domain: larkDomainInventory, Metric: larkMetricInventoryQuantity, Operation: larkOperationTotal,
			TimeRange: larkTimeCurrent, Days: 7,
		},
		"product details with dates": {
			Domain: larkDomainProduct, Operation: larkOperationDetails, TimeRange: larkTimeCurrent,
			DateFrom: "2026-07-01", DateTo: "2026-07-31",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeLarkAnalyticsPlan(plan); err == nil {
				t.Fatalf("plan unexpectedly accepted: %+v", plan)
			}
		})
	}
}

func Test_Lark_analytics_negative_trend_uses_magnitude_for_bars(t *testing.T) {
	plan := larkAnalyticsPlan{
		Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationTrend,
		MovementType: larkMovementAdjustment, TimeRange: larkTimeCurrentMonth,
		TimeBucket: larkBucketDay, Limit: 10, Presentation: larkPresentationTrend,
	}
	card, err := larkAnalyticsCard(plan, []larkAnalyticsRow{{Name: "2026-08-01", MetricValue: -8}, {Name: "2026-08-02", MetricValue: -4}}, false, nil)
	if err != nil || !strings.Contains(card, "▰▰▰▰▰▰▰▰ ") || !strings.Contains(card, "▰▰▰▰ ▱▱▱▱") {
		t.Fatalf("negative trend card = %s err=%v", card, err)
	}
}
