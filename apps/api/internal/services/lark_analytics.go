package services

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"gorm.io/gorm"
)

const (
	larkMetricInventoryQuantity = "inventory_quantity"
	larkMetricInventoryValue    = "inventory_value"
	larkMetricMovementQuantity  = "movement_quantity"
	larkMetricMovementCount     = "movement_count"
	larkMetricMovementValue     = "movement_value"

	larkDomainProduct   = "product"
	larkDomainInventory = "inventory"
	larkDomainShop      = "shop"
	larkDomainMovement  = "movement"
	larkDomainOperator  = "operator"

	larkOperationTotal      = "total"
	larkOperationDetails    = "details"
	larkOperationRanking    = "ranking"
	larkOperationTrend      = "trend"
	larkOperationShare      = "share"
	larkOperationComparison = "comparison"

	larkGroupNone     = "none"
	larkGroupProduct  = "product"
	larkGroupShop     = "shop"
	larkGroupOperator = "operator"

	larkMovementAll        = "all"
	larkMovementInbound    = "inbound"
	larkMovementOutbound   = "sales_outbound"
	larkMovementAdjustment = "adjustment"

	larkTimeCurrent       = "current"
	larkTimeToday         = "today"
	larkTimeYesterday     = "yesterday"
	larkTimeCurrentWeek   = "current_week"
	larkTimePreviousWeek  = "previous_week"
	larkTimeLastNDays     = "last_n_days"
	larkTimeCurrentMonth  = "current_month"
	larkTimePreviousMonth = "previous_month"
	larkTimeCurrentYear   = "current_year"
	larkTimeDateRange     = "date_range"
	larkTimeAll           = "all"

	larkBucketDay   = "day"
	larkBucketMonth = "month"

	larkComparePreviousPeriod = "previous_period"
	larkComparePreviousYear   = "previous_year"

	larkPresentationAuto       = "auto"
	larkPresentationSummary    = "summary"
	larkPresentationDetail     = "detail"
	larkPresentationRanking    = "ranking"
	larkPresentationMatrix     = "matrix"
	larkPresentationTrend      = "trend"
	larkPresentationShare      = "share"
	larkPresentationComparison = "comparison"

	larkAnalyticsDefaultLimit = 5
)

type larkAnalyticsPlan struct {
	Domain          string
	Metric          string
	Operation       string
	GroupBy         string
	GroupBy2        string
	MovementType    string
	TimeRange       string
	Days            int
	DateFrom        string
	DateTo          string
	TimeBucket      string
	CompareTo       string
	Sort            string
	Limit           int
	Presentation    string
	ProductKeyword  string
	ShopKeyword     string
	OperatorKeyword string
}

type larkAnalyticsRow struct {
	Name                 string              `gorm:"column:name"`
	Name2                string              `gorm:"column:name2"`
	Code                 string              `gorm:"column:code"`
	ImagePath            string              `gorm:"column:image_path"`
	Note                 string              `gorm:"column:note"`
	ShopName             string              `gorm:"column:shop_name"`
	OperatorName         string              `gorm:"column:operator_name"`
	MovementType         models.MovementType `gorm:"column:movement_type"`
	Enabled              bool                `gorm:"column:enabled"`
	DefaultPurchaseCents int64               `gorm:"column:default_purchase_cents"`
	CurrentQuantity      int64               `gorm:"column:current_quantity"`
	QuantityDelta        int64               `gorm:"column:quantity_delta"`
	MetricValue          int64               `gorm:"column:metric_value"`
	CountValue           int64               `gorm:"column:count_value"`
	ReferenceValue       int64               `gorm:"-"`
	ShareBasisPoints     int64               `gorm:"-"`
	CreatedAt            time.Time           `gorm:"column:created_at"`
}

func (plan larkAnalyticsPlan) effectiveDomain() string {
	if plan.Domain != "" {
		return plan.Domain
	}
	if plan.Metric == larkMetricInventoryQuantity || plan.Metric == larkMetricInventoryValue {
		return larkDomainInventory
	}
	if plan.Metric != "" {
		return larkDomainMovement
	}
	return ""
}

func (plan larkAnalyticsPlan) effectiveOperation() string {
	if plan.Operation != "" {
		return plan.Operation
	}
	if plan.Metric == "" && plan.Domain != "" {
		return larkOperationDetails
	}
	if plan.GroupBy != "" && plan.GroupBy != larkGroupNone {
		return larkOperationRanking
	}
	return larkOperationTotal
}

func (plan larkAnalyticsPlan) effectivePresentation() string {
	if plan.Presentation != "" && plan.Presentation != larkPresentationAuto {
		return plan.Presentation
	}
	return plan.expectedPresentation()
}

func (plan larkAnalyticsPlan) expectedPresentation() string {
	switch plan.effectiveOperation() {
	case larkOperationDetails:
		return larkPresentationDetail
	case larkOperationRanking:
		if plan.GroupBy2 != "" {
			return larkPresentationMatrix
		}
		return larkPresentationRanking
	case larkOperationTrend:
		return larkPresentationTrend
	case larkOperationShare:
		return larkPresentationShare
	case larkOperationComparison:
		return larkPresentationComparison
	default:
		return larkPresentationSummary
	}
}

func normalizeLarkAnalyticsPlan(plan larkAnalyticsPlan) (larkAnalyticsPlan, error) {
	plan.Domain = strings.TrimSpace(plan.Domain)
	plan.Metric = strings.TrimSpace(plan.Metric)
	plan.Operation = strings.TrimSpace(plan.Operation)
	plan.GroupBy = strings.TrimSpace(plan.GroupBy)
	plan.GroupBy2 = strings.TrimSpace(plan.GroupBy2)
	plan.MovementType = strings.TrimSpace(plan.MovementType)
	plan.TimeRange = strings.TrimSpace(plan.TimeRange)
	plan.DateFrom = strings.TrimSpace(plan.DateFrom)
	plan.DateTo = strings.TrimSpace(plan.DateTo)
	plan.TimeBucket = strings.TrimSpace(plan.TimeBucket)
	plan.CompareTo = strings.TrimSpace(plan.CompareTo)
	plan.Sort = strings.TrimSpace(plan.Sort)
	plan.Presentation = strings.TrimSpace(plan.Presentation)
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
	if plan.GroupBy == larkGroupNone && plan.GroupBy2 != "" {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}
	for _, group := range []string{plan.GroupBy, plan.GroupBy2} {
		if group == "" {
			continue
		}
		switch group {
		case larkGroupNone, larkGroupProduct, larkGroupShop, larkGroupOperator:
		default:
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	}
	if plan.GroupBy2 != "" && plan.GroupBy == plan.GroupBy2 {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}

	operation := plan.effectiveOperation()
	switch operation {
	case larkOperationTotal:
		if plan.GroupBy != larkGroupNone || plan.GroupBy2 != "" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	case larkOperationDetails:
		if plan.GroupBy != larkGroupNone || plan.GroupBy2 != "" || plan.Metric != "" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	case larkOperationRanking:
		if plan.GroupBy == larkGroupNone {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	case larkOperationShare:
		if plan.GroupBy == larkGroupNone || plan.GroupBy2 != "" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	case larkOperationTrend, larkOperationComparison:
		if plan.GroupBy2 != "" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	default:
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}

	domain := plan.effectiveDomain()
	if operation == larkOperationDetails {
		switch domain {
		case larkDomainProduct, larkDomainInventory, larkDomainShop, larkDomainMovement, larkDomainOperator:
		default:
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
		if err := normalizeLarkDetailPlan(&plan, domain); err != nil {
			return larkAnalyticsPlan{}, err
		}
	} else {
		switch plan.Metric {
		case larkMetricInventoryQuantity, larkMetricInventoryValue:
			if domain != larkDomainInventory || operation == larkOperationTrend || operation == larkOperationComparison ||
				(plan.GroupBy != larkGroupNone && plan.GroupBy != larkGroupProduct) || plan.GroupBy2 != "" ||
				plan.MovementType != "" || plan.ShopKeyword != "" || plan.OperatorKeyword != "" ||
				plan.Days != 0 || plan.DateFrom != "" || plan.DateTo != "" {
				return larkAnalyticsPlan{}, errDeepSeekIntent
			}
			if plan.TimeRange == "" {
				plan.TimeRange = larkTimeCurrent
			}
			if plan.TimeRange != larkTimeCurrent {
				return larkAnalyticsPlan{}, errDeepSeekIntent
			}
		case larkMetricMovementQuantity, larkMetricMovementCount, larkMetricMovementValue:
			if domain != larkDomainMovement {
				return larkAnalyticsPlan{}, errDeepSeekIntent
			}
			if plan.MovementType == "" {
				plan.MovementType = larkMovementAll
			}
			if !validLarkMovementType(plan.MovementType) {
				return larkAnalyticsPlan{}, errDeepSeekIntent
			}
			if plan.TimeRange == "" {
				if operation == larkOperationTrend || operation == larkOperationComparison {
					plan.TimeRange = larkTimeCurrentMonth
				} else {
					plan.TimeRange = larkTimeAll
				}
			}
			if err := validateLarkTimeRange(plan); err != nil {
				return larkAnalyticsPlan{}, err
			}
		default:
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	}

	if operation == larkOperationTrend {
		if plan.TimeBucket == "" {
			plan.TimeBucket = larkBucketDay
		}
		if plan.TimeBucket != larkBucketDay && plan.TimeBucket != larkBucketMonth {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	} else if plan.TimeBucket != "" {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}
	if operation == larkOperationComparison {
		if plan.CompareTo != larkComparePreviousPeriod && plan.CompareTo != larkComparePreviousYear {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
		if plan.TimeRange == larkTimeAll || plan.TimeRange == larkTimeCurrent {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	} else if plan.CompareTo != "" {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}

	if operation == larkOperationTotal {
		if plan.Sort != "" && plan.Sort != "asc" && plan.Sort != "desc" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
		plan.Sort = ""
		plan.Limit = 1
	} else {
		if plan.Sort == "" {
			plan.Sort = "desc"
		}
		if plan.Sort != "asc" && plan.Sort != "desc" {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
		if plan.Limit == 0 {
			if operation == larkOperationDetails || operation == larkOperationTrend || operation == larkOperationShare || operation == larkOperationComparison {
				plan.Limit = larkResultLimit
			} else {
				plan.Limit = larkAnalyticsDefaultLimit
			}
		}
		if plan.Limit < 1 || plan.Limit > larkResultLimit {
			return larkAnalyticsPlan{}, errDeepSeekIntent
		}
	}

	if plan.Presentation != "" && plan.Presentation != larkPresentationAuto && plan.Presentation != plan.expectedPresentation() {
		return larkAnalyticsPlan{}, errDeepSeekIntent
	}
	return plan, nil
}

func normalizeLarkDetailPlan(plan *larkAnalyticsPlan, domain string) error {
	switch domain {
	case larkDomainProduct, larkDomainInventory:
		if plan.ShopKeyword != "" || plan.OperatorKeyword != "" || plan.MovementType != "" ||
			plan.Days != 0 || plan.DateFrom != "" || plan.DateTo != "" {
			return errDeepSeekIntent
		}
		if plan.TimeRange == "" {
			plan.TimeRange = larkTimeCurrent
		}
		if plan.TimeRange != larkTimeCurrent {
			return errDeepSeekIntent
		}
	case larkDomainShop:
		if plan.ProductKeyword != "" || plan.OperatorKeyword != "" || plan.MovementType != "" ||
			plan.Days != 0 || plan.DateFrom != "" || plan.DateTo != "" {
			return errDeepSeekIntent
		}
		if plan.TimeRange == "" {
			plan.TimeRange = larkTimeCurrent
		}
		if plan.TimeRange != larkTimeCurrent {
			return errDeepSeekIntent
		}
	case larkDomainMovement, larkDomainOperator:
		if plan.MovementType == "" {
			plan.MovementType = larkMovementAll
		}
		if !validLarkMovementType(plan.MovementType) {
			return errDeepSeekIntent
		}
		if plan.TimeRange == "" {
			plan.TimeRange = larkTimeAll
		}
		return validateLarkTimeRange(*plan)
	}
	return nil
}

func validLarkMovementType(value string) bool {
	switch value {
	case larkMovementAll, larkMovementInbound, larkMovementOutbound, larkMovementAdjustment:
		return true
	default:
		return false
	}
}

func validateLarkTimeRange(plan larkAnalyticsPlan) error {
	switch plan.TimeRange {
	case larkTimeToday, larkTimeYesterday, larkTimeCurrentWeek, larkTimePreviousWeek,
		larkTimeCurrentMonth, larkTimePreviousMonth, larkTimeCurrentYear, larkTimeAll:
		if plan.Days != 0 || plan.DateFrom != "" || plan.DateTo != "" {
			return errDeepSeekIntent
		}
	case larkTimeLastNDays:
		if plan.Days < 1 || plan.Days > 365 || plan.DateFrom != "" || plan.DateTo != "" {
			return errDeepSeekIntent
		}
	case larkTimeDateRange:
		if plan.Days != 0 {
			return errDeepSeekIntent
		}
		start, err := time.ParseInLocation("2006-01-02", plan.DateFrom, larkLocation)
		if err != nil {
			return errDeepSeekIntent
		}
		end, err := time.ParseInLocation("2006-01-02", plan.DateTo, larkLocation)
		if err != nil || start.After(end) {
			return errDeepSeekIntent
		}
	default:
		return errDeepSeekIntent
	}
	return nil
}

func (plan larkAnalyticsPlan) auditMetadata() map[string]string {
	return map[string]string{
		"domain": plan.effectiveDomain(), "metric": plan.Metric, "operation": plan.effectiveOperation(),
		"group_by":      strings.Trim(strings.Join([]string{plan.GroupBy, plan.GroupBy2}, ","), ","),
		"movement_type": plan.MovementType, "time_range": plan.TimeRange, "days": strconv.Itoa(plan.Days),
		"date_from": plan.DateFrom, "date_to": plan.DateTo, "time_bucket": plan.TimeBucket, "compare_to": plan.CompareTo,
		"sort": plan.Sort, "limit": strconv.Itoa(plan.Limit), "presentation": plan.effectivePresentation(),
		"product_keyword": plan.ProductKeyword, "shop_keyword": plan.ShopKeyword, "operator_keyword": plan.OperatorKeyword,
	}
}

func queryLarkAnalytics(db *gorm.DB, plan larkAnalyticsPlan, now time.Time) ([]larkAnalyticsRow, bool, error) {
	plan, err := normalizeLarkAnalyticsPlan(plan)
	if err != nil {
		return nil, false, err
	}
	if plan.effectiveOperation() == larkOperationDetails {
		return queryLarkDetails(db, plan, now)
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
	base := func() *gorm.DB {
		query := db.Table("products").
			Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id").
			Where("products.archived_at IS NULL")
		if plan.ProductKeyword != "" {
			like := "%" + strings.ToLower(plan.ProductKeyword) + "%"
			query = query.Where("LOWER(products.name) LIKE ? OR LOWER(products.code) LIKE ?", like, like)
		}
		return query
	}
	if plan.GroupBy == larkGroupNone {
		var rows []larkAnalyticsRow
		err := base().Select("COALESCE(SUM(" + metricExpr + "), 0) AS metric_value, COUNT(products.id) AS count_value").Scan(&rows).Error
		return rows, false, err
	}

	var total int64
	if plan.effectiveOperation() == larkOperationShare {
		if err := base().Select("COALESCE(SUM(" + metricExpr + "), 0)").Scan(&total).Error; err != nil {
			return nil, false, err
		}
	}
	var rows []larkAnalyticsRow
	query := base().Select("products.name AS name, products.code AS code, products.image_path AS image_path, products.note AS note, products.enabled AS enabled, products.default_purchase_cents AS default_purchase_cents, " +
		quantityExpr + " AS current_quantity, " + metricExpr + " AS metric_value, 1 AS count_value")
	query = orderLarkAnalytics(query, plan.Sort).
		Order("products.name ASC").Order("products.code ASC").Order("products.id ASC").Limit(plan.Limit + 1)
	if err := query.Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	rows, more, _ := trimLarkAnalyticsRows(rows, plan.Limit)
	if plan.effectiveOperation() == larkOperationShare {
		for index := range rows {
			rows[index].ShareBasisPoints = larkBasisPoints(rows[index].MetricValue, total)
		}
	}
	return rows, more, nil
}

func queryLarkDetails(db *gorm.DB, plan larkAnalyticsPlan, now time.Time) ([]larkAnalyticsRow, bool, error) {
	switch plan.effectiveDomain() {
	case larkDomainProduct, larkDomainInventory:
		query := db.Table("products").
			Select("products.name AS name, products.code AS code, products.image_path AS image_path, products.note AS note, products.enabled AS enabled, products.default_purchase_cents AS default_purchase_cents, COALESCE(inventory_snapshots.quantity, 0) AS current_quantity").
			Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id").
			Where("products.archived_at IS NULL")
		if plan.ProductKeyword != "" {
			like := "%" + strings.ToLower(plan.ProductKeyword) + "%"
			query = query.Where("LOWER(products.name) LIKE ? OR LOWER(products.code) LIKE ?", like, like)
		}
		var rows []larkAnalyticsRow
		err := query.Order("products.name ASC").Order("products.code ASC").Order("products.id ASC").Limit(plan.Limit + 1).Scan(&rows).Error
		if err != nil {
			return nil, false, err
		}
		return trimLarkAnalyticsRows(rows, plan.Limit)
	case larkDomainShop:
		query := db.Table("shops").Select("shops.name AS name, shops.note AS note, shops.enabled AS enabled")
		if plan.ShopKeyword != "" {
			query = query.Where("LOWER(shops.name) LIKE ?", "%"+strings.ToLower(plan.ShopKeyword)+"%")
		}
		var rows []larkAnalyticsRow
		err := query.Order("shops.name ASC").Order("shops.id ASC").Limit(plan.Limit + 1).Scan(&rows).Error
		if err != nil {
			return nil, false, err
		}
		return trimLarkAnalyticsRows(rows, plan.Limit)
	case larkDomainOperator:
		query := larkMovementQuery(db, plan, now, nil).
			Select("users.name AS name, COUNT(*) AS count_value").
			Group("users.id, users.name").Order("users.name ASC").Order("users.id ASC")
		var rows []larkAnalyticsRow
		if err := query.Limit(plan.Limit + 1).Scan(&rows).Error; err != nil {
			return nil, false, err
		}
		return trimLarkAnalyticsRows(rows, plan.Limit)
	default:
		query := larkMovementQuery(db, plan, now, nil).
			Select("products.name AS name, products.code AS code, products.image_path AS image_path, shops.name AS shop_name, users.name AS operator_name, stock_movements.type AS movement_type, stock_movements.quantity_delta AS quantity_delta, stock_movements.reason AS note, stock_movements.created_at AS created_at").
			Order("stock_movements.created_at DESC").Order("stock_movements.id DESC")
		var rows []larkAnalyticsRow
		if err := query.Limit(plan.Limit + 1).Scan(&rows).Error; err != nil {
			return nil, false, err
		}
		return trimLarkAnalyticsRows(rows, plan.Limit)
	}
}

type larkTimeWindow struct {
	Start time.Time
	End   time.Time
}

func larkMovementQuery(db *gorm.DB, plan larkAnalyticsPlan, now time.Time, override *larkTimeWindow) *gorm.DB {
	query := db.Table("stock_movements").
		Joins("JOIN products ON products.id = stock_movements.product_id").
		Joins("LEFT JOIN shops ON shops.id = stock_movements.shop_id").
		Joins("JOIN users ON users.id = stock_movements.operator_id").
		Joins("LEFT JOIN inventory_snapshots ON inventory_snapshots.product_id = products.id")
	if plan.MovementType != larkMovementAll {
		query = query.Where("stock_movements.type = ?", plan.MovementType)
	}
	if override != nil {
		query = query.Where("stock_movements.created_at >= ? AND stock_movements.created_at < ?", override.Start, override.End)
	} else if start, end, bounded := larkAnalyticsRange(plan, now); bounded {
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
		query = query.Where("LOWER(users.name) LIKE ?", "%"+strings.ToLower(plan.OperatorKeyword)+"%")
	}
	if plan.GroupBy == larkGroupShop || plan.GroupBy2 == larkGroupShop {
		query = query.Where("stock_movements.shop_id IS NOT NULL")
	}
	return query
}

func queryLarkMovementAnalytics(db *gorm.DB, plan larkAnalyticsPlan, now time.Time) ([]larkAnalyticsRow, bool, error) {
	switch plan.effectiveOperation() {
	case larkOperationComparison:
		return queryLarkMovementComparison(db, plan, now)
	case larkOperationTrend:
		return queryLarkMovementTrend(db, plan, now)
	}
	if plan.GroupBy == larkGroupNone {
		return queryLarkMovementTotal(db, plan, now, nil)
	}
	rows, more, err := queryLarkMovementGroups(db, plan, now, nil, plan.Limit)
	if err != nil {
		return nil, false, err
	}
	if plan.effectiveOperation() == larkOperationShare {
		totalRows, _, totalErr := queryLarkMovementTotal(db, plan, now, nil)
		if totalErr != nil {
			return nil, false, totalErr
		}
		total := int64(0)
		if len(totalRows) > 0 {
			total = totalRows[0].MetricValue
		}
		for index := range rows {
			rows[index].ShareBasisPoints = larkBasisPoints(rows[index].MetricValue, total)
		}
	}
	return rows, more, nil
}

func queryLarkMovementTotal(db *gorm.DB, plan larkAnalyticsPlan, now time.Time, window *larkTimeWindow) ([]larkAnalyticsRow, bool, error) {
	var rows []larkAnalyticsRow
	err := larkMovementQuery(db, plan, now, window).
		Select(larkMovementMetricExpr(plan) + " AS metric_value, COUNT(*) AS count_value").Scan(&rows).Error
	return rows, false, err
}

type larkGroupSQL struct {
	Select string
	Group  string
	Order  []string
}

func larkAnalyticsGroupSQL(group string, alias string, primary bool) larkGroupSQL {
	switch group {
	case larkGroupProduct:
		selectSQL := "products.name AS " + alias
		groupSQL := "products.id, products.name"
		orders := []string{"products.name ASC", "products.id ASC"}
		if primary {
			selectSQL += ", products.code AS code, products.image_path AS image_path, products.default_purchase_cents AS default_purchase_cents, COALESCE(inventory_snapshots.quantity, 0) AS current_quantity"
			groupSQL += ", products.code, products.image_path, products.default_purchase_cents, inventory_snapshots.quantity"
			orders = []string{"products.name ASC", "products.code ASC", "products.id ASC"}
		}
		return larkGroupSQL{Select: selectSQL, Group: groupSQL, Order: orders}
	case larkGroupShop:
		return larkGroupSQL{Select: "shops.name AS " + alias, Group: "shops.id, shops.name", Order: []string{"shops.name ASC", "shops.id ASC"}}
	default:
		return larkGroupSQL{Select: "users.name AS " + alias, Group: "users.id, users.name", Order: []string{"users.name ASC", "users.id ASC"}}
	}
}

func queryLarkMovementGroups(db *gorm.DB, plan larkAnalyticsPlan, now time.Time, window *larkTimeWindow, limit int) ([]larkAnalyticsRow, bool, error) {
	primary := larkAnalyticsGroupSQL(plan.GroupBy, "name", true)
	selects := []string{primary.Select, larkMovementMetricExpr(plan) + " AS metric_value", "COUNT(*) AS count_value"}
	groups := []string{primary.Group}
	orders := append([]string(nil), primary.Order...)
	if plan.GroupBy2 != "" {
		secondary := larkAnalyticsGroupSQL(plan.GroupBy2, "name2", false)
		selects = append(selects, secondary.Select)
		groups = append(groups, secondary.Group)
		orders = append(orders, secondary.Order...)
	}
	query := larkMovementQuery(db, plan, now, window).Select(strings.Join(selects, ", ")).Group(strings.Join(groups, ", "))
	query = orderLarkAnalytics(query, plan.Sort)
	for _, order := range orders {
		query = query.Order(order)
	}
	if limit > 0 {
		query = query.Limit(limit + 1)
	}
	var rows []larkAnalyticsRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	if limit == 0 {
		return rows, false, nil
	}
	return trimLarkAnalyticsRows(rows, limit)
}

func queryLarkMovementTrend(db *gorm.DB, plan larkAnalyticsPlan, now time.Time) ([]larkAnalyticsRow, bool, error) {
	bucket := larkTimeBucketExpr(db, plan.TimeBucket)
	selects := []string{bucket + " AS name", larkMovementMetricExpr(plan) + " AS metric_value", "COUNT(*) AS count_value"}
	groups := []string{bucket}
	orders := []string{"name DESC"}
	if plan.GroupBy != larkGroupNone {
		dimension := larkAnalyticsGroupSQL(plan.GroupBy, "name2", false)
		selects = append(selects, dimension.Select)
		groups = append(groups, dimension.Group)
		orders = append(orders, dimension.Order...)
	}
	query := larkMovementQuery(db, plan, now, nil).Select(strings.Join(selects, ", ")).Group(strings.Join(groups, ", "))
	for _, order := range orders {
		query = query.Order(order)
	}
	var rows []larkAnalyticsRow
	if err := query.Limit(plan.Limit + 1).Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	rows, more, _ := trimLarkAnalyticsRows(rows, plan.Limit)
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	return rows, more, nil
}

func queryLarkMovementComparison(db *gorm.DB, plan larkAnalyticsPlan, now time.Time) ([]larkAnalyticsRow, bool, error) {
	start, end, bounded := larkAnalyticsRange(plan, now)
	if !bounded {
		return nil, false, errDeepSeekIntent
	}
	current := &larkTimeWindow{Start: start, End: end}
	reference := &larkTimeWindow{End: start}
	if plan.CompareTo == larkComparePreviousYear {
		reference.Start = start.AddDate(-1, 0, 0)
		reference.End = end.AddDate(-1, 0, 0)
	} else {
		switch plan.TimeRange {
		case larkTimeCurrentMonth, larkTimePreviousMonth:
			reference.Start = start.AddDate(0, -1, 0)
		case larkTimeCurrentYear:
			reference.Start = start.AddDate(-1, 0, 0)
		default:
			reference.Start = start.Add(-(end.Sub(start)))
		}
	}

	var currentRows, referenceRows []larkAnalyticsRow
	var err error
	if plan.GroupBy == larkGroupNone {
		currentRows, _, err = queryLarkMovementTotal(db, plan, now, current)
		if err == nil {
			referenceRows, _, err = queryLarkMovementTotal(db, plan, now, reference)
		}
	} else {
		currentRows, _, err = queryLarkMovementGroups(db, plan, now, current, 0)
		if err == nil {
			referenceRows, _, err = queryLarkMovementGroups(db, plan, now, reference, 0)
		}
	}
	if err != nil {
		return nil, false, err
	}
	merged := make(map[string]larkAnalyticsRow, len(currentRows)+len(referenceRows))
	for _, row := range currentRows {
		merged[larkAnalyticsRowKey(row)] = row
	}
	for _, row := range referenceRows {
		key := larkAnalyticsRowKey(row)
		currentRow := merged[key]
		if currentRow.Name == "" {
			currentRow.Name, currentRow.Name2, currentRow.Code, currentRow.ImagePath = row.Name, row.Name2, row.Code, row.ImagePath
		}
		currentRow.ReferenceValue = row.MetricValue
		merged[key] = currentRow
	}
	rows := make([]larkAnalyticsRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].MetricValue != rows[j].MetricValue {
			if plan.Sort == "asc" {
				return rows[i].MetricValue < rows[j].MetricValue
			}
			return rows[i].MetricValue > rows[j].MetricValue
		}
		return larkAnalyticsRowKey(rows[i]) < larkAnalyticsRowKey(rows[j])
	})
	return trimLarkAnalyticsRows(rows, plan.Limit)
}

func larkAnalyticsRowKey(row larkAnalyticsRow) string {
	return row.Name + "\x00" + row.Name2 + "\x00" + row.Code
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
	month := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, larkLocation)
	week := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
	switch plan.TimeRange {
	case larkTimeToday:
		return today.UTC(), today.AddDate(0, 0, 1).UTC(), true
	case larkTimeYesterday:
		return today.AddDate(0, 0, -1).UTC(), today.UTC(), true
	case larkTimeCurrentWeek:
		return week.UTC(), week.AddDate(0, 0, 7).UTC(), true
	case larkTimePreviousWeek:
		return week.AddDate(0, 0, -7).UTC(), week.UTC(), true
	case larkTimeLastNDays:
		return today.AddDate(0, 0, 1-plan.Days).UTC(), today.AddDate(0, 0, 1).UTC(), true
	case larkTimeCurrentMonth:
		return month.UTC(), month.AddDate(0, 1, 0).UTC(), true
	case larkTimePreviousMonth:
		return month.AddDate(0, -1, 0).UTC(), month.UTC(), true
	case larkTimeCurrentYear:
		start := time.Date(localNow.Year(), 1, 1, 0, 0, 0, 0, larkLocation)
		return start.UTC(), start.AddDate(1, 0, 0).UTC(), true
	case larkTimeDateRange:
		start, _ := time.ParseInLocation("2006-01-02", plan.DateFrom, larkLocation)
		end, _ := time.ParseInLocation("2006-01-02", plan.DateTo, larkLocation)
		return start.UTC(), end.AddDate(0, 0, 1).UTC(), true
	default:
		return time.Time{}, time.Time{}, false
	}
}

func larkTimeBucketExpr(db *gorm.DB, bucket string) string {
	if db.Dialector.Name() == "postgres" {
		if bucket == larkBucketMonth {
			return "TO_CHAR(stock_movements.created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM')"
		}
		return "TO_CHAR(stock_movements.created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')"
	}
	if bucket == larkBucketMonth {
		return "STRFTIME('%Y-%m', stock_movements.created_at, '+8 hours')"
	}
	return "STRFTIME('%Y-%m-%d', stock_movements.created_at, '+8 hours')"
}

func orderLarkAnalytics(query *gorm.DB, sortOrder string) *gorm.DB {
	if sortOrder == "asc" {
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

func larkBasisPoints(value int64, total int64) int64 {
	if total == 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(value), big.NewInt(10000))
	result := numerator.Quo(numerator, big.NewInt(total))
	if !result.IsInt64() {
		return 0
	}
	return result.Int64()
}

func larkAnalyticsCard(plan larkAnalyticsPlan, rows []larkAnalyticsRow, more bool, imageKeys []string) (string, error) {
	plan, err := normalizeLarkAnalyticsPlan(plan)
	if err != nil {
		return "", err
	}
	title := larkAnalyticsTitle(plan)
	if len(rows) == 0 {
		return larkSimpleCard(title, "grey", "没有匹配的数据。请检查时间范围或筛选条件。")
	}
	elements := []larkcard.MessageCardElement{
		larkcard.NewMessageCardMarkdown().Content(larkAnalyticsContext(plan)).Build(),
	}
	presentation := plan.effectivePresentation()
	switch presentation {
	case larkPresentationMatrix:
		elements = append(elements, larkAnalyticsMatrixElements(plan, rows, imageKeys)...)
	case larkPresentationTrend:
		elements = append(elements, larkAnalyticsTrendElements(plan, rows)...)
	case larkPresentationShare:
		elements = append(elements, larkAnalyticsShareElements(plan, rows, imageKeys)...)
	case larkPresentationComparison:
		elements = append(elements, larkAnalyticsComparisonElements(plan, rows, imageKeys)...)
	case larkPresentationDetail:
		for index, row := range rows {
			if index > 0 {
				elements = append(elements, larkcard.NewMessageCardHr().Build())
			}
			elements = append(elements, larkAnalyticsDetailElement(plan, row, larkImageKeyAt(imageKeys, index)))
		}
	default:
		for index, row := range rows {
			if index > 0 {
				elements = append(elements, larkcard.NewMessageCardHr().Build())
			}
			elements = append(elements, larkAnalyticsElement(plan, row, index+1, larkImageKeyAt(imageKeys, index)))
		}
	}
	if more {
		elements = append(elements, larkcard.NewMessageCardMarkdown().Content(fmt.Sprintf("结果已截断，仅展示前 %d 条；统计总量和占比分母仍基于全部匹配数据。", plan.Limit)).Build())
	}
	template := "blue"
	if presentation == larkPresentationComparison {
		template = "orange"
	} else if presentation == larkPresentationShare || presentation == larkPresentationTrend {
		template = "green"
	}
	return larkCard(title, template, elements)
}

func larkAnalyticsContext(plan larkAnalyticsPlan) string {
	filters := make([]string, 0, 3)
	for label, value := range map[string]string{"商品": plan.ProductKeyword, "店铺": plan.ShopKeyword, "操作人": plan.OperatorKeyword} {
		if value != "" {
			filters = append(filters, label+"="+escapeLarkMarkdown(value))
		}
	}
	sort.Strings(filters)
	filterText := "无"
	if len(filters) > 0 {
		filterText = strings.Join(filters, "，")
	}
	context := fmt.Sprintf("**时间范围：** %s　**筛选：** %s", larkAnalyticsTimeLabel(plan), filterText)
	if plan.Metric == larkMetricMovementValue || plan.Metric == larkMetricInventoryValue {
		context += "\n金额口径：按当前商品采购价估算。"
	}
	return context
}

func larkAnalyticsElement(plan larkAnalyticsPlan, row larkAnalyticsRow, rank int, imageKey string) larkcard.MessageCardElement {
	label := larkAnalyticsMetricLabel(plan)
	value := larkAnalyticsValue(plan, row.MetricValue)
	if plan.GroupBy == larkGroupNone {
		fields := []*larkcard.MessageCardField{larkField(fmt.Sprintf("**匹配流水/记录**\n%d", row.CountValue))}
		return larkDetailElement(fmt.Sprintf("**%s**\n\n📊 **%s**", label, value), fields, "", "")
	}
	if plan.GroupBy == larkGroupProduct {
		fields := []*larkcard.MessageCardField{
			larkField(fmt.Sprintf("**当前库存**\n**%d 件**", row.CurrentQuantity)),
			larkField(fmt.Sprintf("**采购价**\n%s", larkMoney(row.DefaultPurchaseCents))),
			larkField(fmt.Sprintf("**流水笔数**\n%d", row.CountValue)),
		}
		return larkDetailElement(
			fmt.Sprintf("**%d. %s**\n编码：`%s`\n\n📊 **%s：%s**", rank, escapeLarkMarkdown(row.Name), escapeLarkMarkdown(row.Code), label, value),
			fields, imageKey, row.Name,
		)
	}
	dimension := "店铺"
	if plan.GroupBy == larkGroupOperator {
		dimension = "操作人"
	}
	return larkDetailElement(
		fmt.Sprintf("**%d. %s**\n%s\n\n📊 **%s：%s**\n流水笔数：%d", rank, escapeLarkMarkdown(row.Name), dimension, label, value, row.CountValue),
		nil, "", "",
	)
}

func larkAnalyticsDetailElement(plan larkAnalyticsPlan, row larkAnalyticsRow, imageKey string) larkcard.MessageCardElement {
	switch plan.effectiveDomain() {
	case larkDomainProduct, larkDomainInventory:
		status := "停用"
		if row.Enabled {
			status = "启用"
		}
		fields := []*larkcard.MessageCardField{
			larkField(fmt.Sprintf("**当前库存**\n%d 件", row.CurrentQuantity)),
			larkField("**采购价**\n" + larkMoney(row.DefaultPurchaseCents)),
			larkField("**状态**\n" + status),
		}
		if row.Note != "" {
			fields = append(fields, larkField("**备注**\n"+escapeLarkMarkdown(row.Note)))
		}
		return larkDetailElement(fmt.Sprintf("**%s**\n编码：`%s`", escapeLarkMarkdown(row.Name), escapeLarkMarkdown(row.Code)), fields, imageKey, row.Name)
	case larkDomainShop:
		status := "停用"
		if row.Enabled {
			status = "启用"
		}
		content := fmt.Sprintf("**%s**\n状态：%s", escapeLarkMarkdown(row.Name), status)
		if row.Note != "" {
			content += "\n备注：" + escapeLarkMarkdown(row.Note)
		}
		return larkDetailElement(content, nil, "", "")
	case larkDomainOperator:
		return larkDetailElement(fmt.Sprintf("**%s**\n历史流水：%d 笔", escapeLarkMarkdown(row.Name), row.CountValue), nil, "", "")
	default:
		shop := "-"
		if row.ShopName != "" {
			shop = escapeLarkMarkdown(row.ShopName)
		}
		fields := []*larkcard.MessageCardField{
			larkField("**类型**\n" + larkMovementLabel(string(row.MovementType))),
			larkField(fmt.Sprintf("**数量变化**\n%s", larkSignedQuantity(row.QuantityDelta))),
			larkField("**店铺**\n" + shop),
			larkField("**操作人**\n" + escapeLarkMarkdown(row.OperatorName)),
			larkField("**时间**\n" + row.CreatedAt.In(larkLocation).Format("2006-01-02 15:04:05")),
		}
		if row.Note != "" {
			fields = append(fields, larkField("**备注**\n"+escapeLarkMarkdown(row.Note)))
		}
		return larkDetailElement(fmt.Sprintf("**%s**\n编码：`%s`", escapeLarkMarkdown(row.Name), escapeLarkMarkdown(row.Code)), fields, imageKey, row.Name)
	}
}

func larkAnalyticsMatrixElements(plan larkAnalyticsPlan, rows []larkAnalyticsRow, imageKeys []string) []larkcard.MessageCardElement {
	elements := make([]larkcard.MessageCardElement, 0, len(rows)*2)
	for index, row := range rows {
		if index > 0 {
			elements = append(elements, larkcard.NewMessageCardHr().Build())
		}
		content := fmt.Sprintf("**%d. %s → %s**\n%s：**%s**　·　%d 笔", index+1,
			escapeLarkMarkdown(row.Name), escapeLarkMarkdown(row.Name2), larkAnalyticsMetricLabel(plan), larkAnalyticsValue(plan, row.MetricValue), row.CountValue)
		elements = append(elements, larkDetailElement(content, nil, larkImageKeyAt(imageKeys, index), row.Name))
	}
	return elements
}

func larkAnalyticsTrendElements(plan larkAnalyticsPlan, rows []larkAnalyticsRow) []larkcard.MessageCardElement {
	maxValue := int64(0)
	for _, row := range rows {
		value := row.MetricValue
		if value < 0 {
			value = -value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	elements := make([]larkcard.MessageCardElement, 0, len(rows))
	for _, row := range rows {
		magnitude := row.MetricValue
		if magnitude < 0 {
			magnitude = -magnitude
		}
		barLength := 1
		if maxValue > 0 {
			barLength = int(larkBasisPoints(magnitude, maxValue) * 8 / 10000)
			if barLength < 1 {
				barLength = 1
			}
		}
		label := row.Name
		if row.Name2 != "" {
			label += " · " + row.Name2
		}
		content := fmt.Sprintf("**%s**　%s\n%s %s", escapeLarkMarkdown(label), larkAnalyticsValue(plan, row.MetricValue), strings.Repeat("▰", barLength), strings.Repeat("▱", 8-barLength))
		elements = append(elements, larkcard.NewMessageCardMarkdown().Content(content).Build())
	}
	return elements
}

func larkAnalyticsShareElements(plan larkAnalyticsPlan, rows []larkAnalyticsRow, imageKeys []string) []larkcard.MessageCardElement {
	elements := make([]larkcard.MessageCardElement, 0, len(rows)*2)
	for index, row := range rows {
		if index > 0 {
			elements = append(elements, larkcard.NewMessageCardHr().Build())
		}
		barLength := int(row.ShareBasisPoints * 10 / 10000)
		if barLength < 0 {
			barLength = -barLength
		}
		if barLength > 10 {
			barLength = 10
		}
		content := fmt.Sprintf("**%d. %s**\n%s：**%s**　占比：**%s**\n%s%s", index+1, escapeLarkMarkdown(row.Name),
			larkAnalyticsMetricLabel(plan), larkAnalyticsValue(plan, row.MetricValue), larkBasisPointText(row.ShareBasisPoints), strings.Repeat("■", barLength), strings.Repeat("□", 10-barLength))
		elements = append(elements, larkDetailElement(content, nil, larkImageKeyAt(imageKeys, index), row.Name))
	}
	return elements
}

func larkAnalyticsComparisonElements(plan larkAnalyticsPlan, rows []larkAnalyticsRow, imageKeys []string) []larkcard.MessageCardElement {
	elements := make([]larkcard.MessageCardElement, 0, len(rows)*2)
	for index, row := range rows {
		if index > 0 {
			elements = append(elements, larkcard.NewMessageCardHr().Build())
		}
		name := larkAnalyticsMetricLabel(plan)
		if row.Name != "" {
			name = row.Name
		}
		delta := row.MetricValue - row.ReferenceValue
		change := "无可比基数"
		if row.ReferenceValue != 0 {
			change = larkBasisPointText(larkBasisPoints(delta, row.ReferenceValue))
		}
		fields := []*larkcard.MessageCardField{
			larkField("**当前**\n" + larkAnalyticsValue(plan, row.MetricValue)),
			larkField("**参考期**\n" + larkAnalyticsValue(plan, row.ReferenceValue)),
			larkField("**差值**\n" + larkAnalyticsValue(plan, delta)),
			larkField("**变化**\n" + change),
		}
		elements = append(elements, larkDetailElement("**"+escapeLarkMarkdown(name)+"**", fields, larkImageKeyAt(imageKeys, index), row.Name))
	}
	return elements
}

func larkBasisPointText(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d%%", sign, value/100, value%100)
}

func larkAnalyticsTitle(plan larkAnalyticsPlan) string {
	if plan.effectiveOperation() == larkOperationDetails {
		title := map[string]string{
			larkDomainProduct: "商品明细", larkDomainInventory: "当前库存明细", larkDomainShop: "店铺明细",
			larkDomainMovement: "流水明细", larkDomainOperator: "历史操作人",
		}[plan.effectiveDomain()]
		if plan.effectiveDomain() == larkDomainMovement || plan.effectiveDomain() == larkDomainOperator {
			return larkAnalyticsTimeLabel(plan) + title
		}
		return title
	}
	title := larkAnalyticsTimeLabel(plan) + larkAnalyticsMetricLabel(plan)
	switch plan.effectiveOperation() {
	case larkOperationTrend:
		return title + " · 趋势"
	case larkOperationShare:
		return title + " · 占比"
	case larkOperationComparison:
		return title + " · 对比"
	}
	if plan.GroupBy2 != "" {
		return title + " · " + larkAnalyticsGroupLabel(plan.GroupBy) + " × " + larkAnalyticsGroupLabel(plan.GroupBy2)
	}
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

func larkAnalyticsGroupLabel(group string) string {
	switch group {
	case larkGroupProduct:
		return "商品"
	case larkGroupShop:
		return "店铺"
	default:
		return "操作人"
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
	case larkTimeCurrentWeek:
		return "本周"
	case larkTimePreviousWeek:
		return "上周"
	case larkTimeLastNDays:
		return fmt.Sprintf("近%d天", plan.Days)
	case larkTimeCurrentMonth:
		return "本月"
	case larkTimePreviousMonth:
		return "上月"
	case larkTimeCurrentYear:
		return "本年"
	case larkTimeDateRange:
		return plan.DateFrom + " 至 " + plan.DateTo + " "
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
		return larkMovementLabel(plan.MovementType) + "估算成本"
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
