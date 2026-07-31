package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"gorm.io/gorm"
)

const (
	larkMetricInventoryQuantity = "inventory_quantity"
	larkMetricInventoryValue    = "inventory_value"
	larkMetricMovementQuantity  = "movement_quantity"
	larkMetricMovementCount     = "movement_count"
	larkMetricMovementValue     = "movement_value"

	larkGroupNone     = "none"
	larkGroupProduct  = "product"
	larkGroupShop     = "shop"
	larkGroupOperator = "operator"

	larkMovementAll        = "all"
	larkMovementInbound    = "inbound"
	larkMovementOutbound   = "sales_outbound"
	larkMovementAdjustment = "adjustment"

	larkTimeCurrent      = "current"
	larkTimeToday        = "today"
	larkTimeYesterday    = "yesterday"
	larkTimeLastNDays    = "last_n_days"
	larkTimeCurrentMonth = "current_month"
	larkTimeAll          = "all"
)

type larkAnalyticsPlan struct {
	Metric          string
	GroupBy         string
	MovementType    string
	TimeRange       string
	Days            int
	Sort            string
	Limit           int
	ProductKeyword  string
	ShopKeyword     string
	OperatorKeyword string
}

type larkAnalyticsRow struct {
	Name                 string `gorm:"column:name"`
	Code                 string `gorm:"column:code"`
	ImagePath            string `gorm:"column:image_path"`
	DefaultPurchaseCents int64  `gorm:"column:default_purchase_cents"`
	CurrentQuantity      int64  `gorm:"column:current_quantity"`
	MetricValue          int64  `gorm:"column:metric_value"`
}

func normalizeLarkAnalyticsPlan(plan larkAnalyticsPlan) (larkAnalyticsPlan, error) {
	plan.Metric = strings.TrimSpace(plan.Metric)
	plan.GroupBy = strings.TrimSpace(plan.GroupBy)
	plan.MovementType = strings.TrimSpace(plan.MovementType)
	plan.TimeRange = strings.TrimSpace(plan.TimeRange)
	plan.Sort = strings.TrimSpace(plan.Sort)
	plan.ProductKeyword = strings.TrimSpace(plan.ProductKeyword)
	plan.ShopKeyword = strings.TrimSpace(plan.ShopKeyword)
	plan.OperatorKeyword = strings.TrimSpace(plan.OperatorKeyword)
	for _, keyword := range []string{plan.ProductKeyword, plan.ShopKeyword, plan.OperatorKeyword} {
		if len([]rune(keyword)) > larkKeywordMaxRunes {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	}
	if plan.GroupBy == "" {
		plan.GroupBy = larkGroupNone
	}
	switch plan.GroupBy {
	case larkGroupNone, larkGroupProduct, larkGroupShop, larkGroupOperator:
	default:
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}

	switch plan.Metric {
	case larkMetricInventoryQuantity, larkMetricInventoryValue:
		if plan.TimeRange == "" {
			plan.TimeRange = larkTimeCurrent
		}
		if plan.TimeRange != larkTimeCurrent || plan.MovementType != "" ||
			(plan.GroupBy != larkGroupNone && plan.GroupBy != larkGroupProduct) ||
			plan.ShopKeyword != "" || plan.OperatorKeyword != "" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	case larkMetricMovementQuantity, larkMetricMovementCount, larkMetricMovementValue:
		if plan.MovementType == "" {
			plan.MovementType = larkMovementAll
		}
		switch plan.MovementType {
		case larkMovementAll, larkMovementInbound, larkMovementOutbound, larkMovementAdjustment:
		default:
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
		if plan.TimeRange == "" {
			plan.TimeRange = larkTimeAll
		}
		switch plan.TimeRange {
		case larkTimeToday, larkTimeYesterday, larkTimeCurrentMonth, larkTimeAll:
		case larkTimeLastNDays:
			if plan.Days < 1 || plan.Days > 365 {
				return larkAnalyticsPlan{}, errDeepSeekIntent
			}
		default:
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	default:
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}
	if plan.TimeRange != larkTimeLastNDays && plan.Days != 0 {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}

	if plan.GroupBy == larkGroupNone {
		if plan.Sort != "" && plan.Sort != "asc" && plan.Sort != "desc" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
		if plan.Limit < 0 || plan.Limit > larkResultLimit {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
		plan.Sort = ""
		plan.Limit = 1
		return plan, nil
	}
	if plan.Sort == "" {
		plan.Sort = "desc"
	}
	if plan.Sort != "asc" && plan.Sort != "desc" {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}
	if plan.Limit == 0 {
		plan.Limit = larkResultLimit
	}
	if plan.Limit < 1 || plan.Limit > larkResultLimit {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}
	return plan, nil
}

func (plan larkAnalyticsPlan) auditMetadata() map[string]string {
	return map[string]string{
		"metric":           plan.Metric,
		"group_by":         plan.GroupBy,
		"movement_type":    plan.MovementType,
		"time_range":       plan.TimeRange,
		"days":             strconv.Itoa(plan.Days),
		"sort":             plan.Sort,
		"limit":            strconv.Itoa(plan.Limit),
		"product_keyword":  plan.ProductKeyword,
		"shop_keyword":     plan.ShopKeyword,
		"operator_keyword": plan.OperatorKeyword,
	}
}

func queryLarkAnalytics(db *gorm.DB, plan larkAnalyticsPlan, now time.Time) ([]larkAnalyticsRow, bool, error) {
	plan, err := normalizeLarkAnalyticsPlan(plan)
	if err != nil {
		return nil, false, err
	}
	if plan.Metric == larkMetricInventoryQuantity || plan.Metric == larkMetricInventoryValue {
		return queryLarkInventoryAnalytics(db, plan)
	}
	return queryLarkMovementAnalytics(db, plan, now)
}

func queryLarkInventoryAnalytics(db *gorm.DB, plan larkAnalyticsPlan) ([]larkAnalyticsRow, bool, error) {
	quantityExpr := "COALESCE(inventory_snapshots.quantity, 0)"
	metricExpr := quantityExpr
	if plan.Metric == larkMetricInventoryValue {
		metricExpr = quantityExpr + " * products.default_purchase_cents"
	}
	query := db.Table("products").
		Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id").
		Where("products.archived_at IS NULL")
	if plan.ProductKeyword != "" {
		like := "%" + strings.ToLower(plan.ProductKeyword) + "%"
		query = query.Where("LOWER(products.name) LIKE ? OR LOWER(products.code) LIKE ?", like, like)
	}
	if plan.GroupBy == larkGroupNone {
		var rows []larkAnalyticsRow
		err := query.Select("COALESCE(SUM(" + metricExpr + "), 0) AS metric_value").Scan(&rows).Error
		return rows, false, err
	}

	var rows []larkAnalyticsRow
	query = query.Select("products.name AS name, products.code AS code, products.image_path AS image_path, products.default_purchase_cents AS default_purchase_cents, " +
		quantityExpr + " AS current_quantity, " + metricExpr + " AS metric_value")
	query = orderLarkAnalytics(query, plan.Sort).
		Order("products.name ASC").
		Order("products.code ASC").
		Order("products.id ASC").
		Limit(plan.Limit + 1)
	if err := query.Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	return trimLarkAnalyticsRows(rows, plan.Limit)
}

func queryLarkMovementAnalytics(db *gorm.DB, plan larkAnalyticsPlan, now time.Time) ([]larkAnalyticsRow, bool, error) {
	query := db.Table("stock_movements").Joins("JOIN products ON products.id = stock_movements.product_id")
	if plan.GroupBy == larkGroupShop || plan.ShopKeyword != "" {
		query = query.Joins("JOIN shops ON shops.id = stock_movements.shop_id")
	}
	if plan.GroupBy == larkGroupOperator || plan.OperatorKeyword != "" {
		query = query.Joins("JOIN users ON users.id = stock_movements.operator_id")
	}
	if plan.GroupBy == larkGroupProduct {
		query = query.Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id")
	}
	if plan.MovementType != larkMovementAll {
		query = query.Where("stock_movements.type = ?", plan.MovementType)
	}
	if start, end, bounded := larkAnalyticsRange(plan, now); bounded {
		query = query.Where("stock_movements.created_at >= ? AND stock_movements.created_at < ?", start, end)
	}
	if plan.ProductKeyword != "" {
		like := "%" + strings.ToLower(plan.ProductKeyword) + "%"
		query = query.Where("LOWER(products.name) LIKE ? OR LOWER(products.code) LIKE ?", like, like)
	}
	if plan.ShopKeyword != "" {
		query = query.Where("LOWER(shops.name) LIKE ?", "%"+strings.ToLower(plan.ShopKeyword)+"%")
	}
	if plan.OperatorKeyword != "" {
		like := "%" + strings.ToLower(plan.OperatorKeyword) + "%"
		query = query.Where("LOWER(users.name) LIKE ? OR LOWER(users.email) LIKE ?", like, like)
	}

	metricExpr := larkMovementMetricExpr(plan)
	if plan.GroupBy == larkGroupNone {
		var rows []larkAnalyticsRow
		err := query.Select(metricExpr + " AS metric_value").Scan(&rows).Error
		return rows, false, err
	}

	var rows []larkAnalyticsRow
	switch plan.GroupBy {
	case larkGroupProduct:
		query = query.Select("products.name AS name, products.code AS code, products.image_path AS image_path, products.default_purchase_cents AS default_purchase_cents, " +
			"COALESCE(inventory_snapshots.quantity, 0) AS current_quantity, " + metricExpr + " AS metric_value").
			Group("products.id, products.name, products.code, products.image_path, products.default_purchase_cents, inventory_snapshots.quantity")
		query = orderLarkAnalytics(query, plan.Sort).Order("products.name ASC").Order("products.code ASC").Order("products.id ASC")
	case larkGroupShop:
		query = query.Select("shops.name AS name, " + metricExpr + " AS metric_value").Group("shops.id, shops.name")
		query = orderLarkAnalytics(query, plan.Sort).Order("shops.name ASC").Order("shops.id ASC")
	case larkGroupOperator:
		query = query.Select("users.name AS name, " + metricExpr + " AS metric_value").Group("users.id, users.name")
		query = orderLarkAnalytics(query, plan.Sort).Order("users.name ASC").Order("users.id ASC")
	}
	if err := query.Limit(plan.Limit + 1).Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	return trimLarkAnalyticsRows(rows, plan.Limit)
}

func larkMovementMetricExpr(plan larkAnalyticsPlan) string {
	if plan.Metric == larkMetricMovementCount {
		return "COUNT(*)"
	}
	quantityExpr := "stock_movements.quantity_delta"
	if plan.MovementType == larkMovementOutbound {
		quantityExpr = "-stock_movements.quantity_delta"
	}
	if plan.Metric == larkMetricMovementValue {
		return "COALESCE(SUM((" + quantityExpr + ") * products.default_purchase_cents), 0)"
	}
	return "COALESCE(SUM(" + quantityExpr + "), 0)"
}

func larkAnalyticsRange(plan larkAnalyticsPlan, now time.Time) (time.Time, time.Time, bool) {
	localNow := now.In(larkLocation)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, larkLocation)
	switch plan.TimeRange {
	case larkTimeToday:
		return today.UTC(), today.AddDate(0, 0, 1).UTC(), true
	case larkTimeYesterday:
		return today.AddDate(0, 0, -1).UTC(), today.UTC(), true
	case larkTimeLastNDays:
		return today.AddDate(0, 0, 1-plan.Days).UTC(), today.AddDate(0, 0, 1).UTC(), true
	case larkTimeCurrentMonth:
		start := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, larkLocation)
		return start.UTC(), start.AddDate(0, 1, 0).UTC(), true
	default:
		return time.Time{}, time.Time{}, false
	}
}

func orderLarkAnalytics(query *gorm.DB, sort string) *gorm.DB {
	if sort == "asc" {
		return query.Order("metric_value ASC")
	}
	return query.Order("metric_value DESC")
}

func trimLarkAnalyticsRows(rows []larkAnalyticsRow, limit int) ([]larkAnalyticsRow, bool, error) {
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	return rows, more, nil
}

func larkAnalyticsCard(plan larkAnalyticsPlan, rows []larkAnalyticsRow, more bool, imageKeys []string) (string, error) {
	plan, err := normalizeLarkAnalyticsPlan(plan)
	if err != nil {
		return "", err
	}
	title := larkAnalyticsTitle(plan)
	if len(rows) == 0 {
		return larkSimpleCard(title, "grey", "没有匹配的统计数据。")
	}
	elements := make([]larkcard.MessageCardElement, 0, len(rows)*2+1)
	for index, row := range rows {
		if index > 0 {
			elements = append(elements, larkcard.NewMessageCardHr().Build())
		}
		elements = append(elements, larkAnalyticsElement(plan, row, index+1, larkImageKeyAt(imageKeys, index)))
	}
	if more {
		elements = append(elements, larkcard.NewMessageCardMarkdown().Content(fmt.Sprintf("仅展示前 %d 条。", plan.Limit)).Build())
	}
	return larkCard(title, "blue", elements)
}

func larkAnalyticsElement(plan larkAnalyticsPlan, row larkAnalyticsRow, rank int, imageKey string) larkcard.MessageCardElement {
	label := larkAnalyticsMetricLabel(plan)
	value := larkAnalyticsValue(plan, row.MetricValue)
	if plan.GroupBy == larkGroupNone {
		return larkDetailElement(fmt.Sprintf("**%s**\n\n📊 **%s**", label, value), nil, "", "")
	}
	if plan.GroupBy == larkGroupProduct {
		fields := []*larkcard.MessageCardField{
			larkField(fmt.Sprintf("**当前库存**\n**%d 件**", row.CurrentQuantity)),
			larkField(fmt.Sprintf("**采购价**\n%s", larkMoney(row.DefaultPurchaseCents))),
		}
		return larkDetailElement(
			fmt.Sprintf("**%d. %s**\n编码：`%s`\n\n📊 **%s：%s**", rank, escapeLarkMarkdown(row.Name), escapeLarkMarkdown(row.Code), label, value),
			fields,
			imageKey,
			row.Name,
		)
	}
	dimension := "店铺"
	if plan.GroupBy == larkGroupOperator {
		dimension = "操作人"
	}
	return larkDetailElement(
		fmt.Sprintf("**%d. %s**\n%s\n\n📊 **%s：%s**", rank, escapeLarkMarkdown(row.Name), dimension, label, value),
		nil,
		"",
		"",
	)
}

func larkAnalyticsTitle(plan larkAnalyticsPlan) string {
	title := larkAnalyticsTimeLabel(plan) + larkAnalyticsMetricLabel(plan)
	switch plan.GroupBy {
	case larkGroupProduct:
		return title + " · 商品排行"
	case larkGroupShop:
		return title + " · 店铺排行"
	case larkGroupOperator:
		return title + " · 操作人排行"
	default:
		return title
	}
}

func larkAnalyticsTimeLabel(plan larkAnalyticsPlan) string {
	switch plan.TimeRange {
	case larkTimeCurrent:
		return "当前"
	case larkTimeToday:
		return "今日"
	case larkTimeYesterday:
		return "昨日"
	case larkTimeLastNDays:
		return fmt.Sprintf("近%d天", plan.Days)
	case larkTimeCurrentMonth:
		return "本月"
	default:
		return "全部历史"
	}
}

func larkAnalyticsMetricLabel(plan larkAnalyticsPlan) string {
	switch plan.Metric {
	case larkMetricInventoryQuantity:
		return "库存数量"
	case larkMetricInventoryValue:
		return "库存金额"
	case larkMetricMovementCount:
		return larkMovementLabel(plan.MovementType) + "流水笔数"
	case larkMetricMovementValue:
		return larkMovementLabel(plan.MovementType) + "金额"
	default:
		return larkMovementLabel(plan.MovementType) + "数量"
	}
}

func larkMovementLabel(movementType string) string {
	switch movementType {
	case larkMovementInbound:
		return "入库"
	case larkMovementOutbound:
		return "销售出库"
	case larkMovementAdjustment:
		return "库存调整"
	default:
		return "库存净变动"
	}
}

func larkAnalyticsValue(plan larkAnalyticsPlan, value int64) string {
	switch plan.Metric {
	case larkMetricInventoryValue, larkMetricMovementValue:
		return larkSignedMoney(value)
	case larkMetricMovementCount:
		return fmt.Sprintf("%d 笔", value)
	default:
		return fmt.Sprintf("%d 件", value)
	}
}

func larkSignedMoney(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s¥%d.%02d", sign, cents/100, cents%100)
}
