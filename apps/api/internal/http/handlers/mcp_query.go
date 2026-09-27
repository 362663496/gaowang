package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

const (
	movementDefaultLimit = 50
	movementMaxLimit     = 1000
	inventoryCodeLimit   = 1000
)

type movementsQueryInput struct {
	Type         string `json:"type,omitempty" jsonschema:"流水类型：inbound、sales_outbound 或 adjustment，可空"`
	ProductQuery string `json:"product_query,omitempty" jsonschema:"按商品名称或编码筛选，可空。名称不唯一时返回候选，不要猜测"`
	ShopQuery    string `json:"shop_query,omitempty" jsonschema:"按店铺名称筛选，可空"`
	From         string `json:"from,omitempty" jsonschema:"开始日期 YYYY-MM-DD，上海时区，含当天，可空"`
	To           string `json:"to,omitempty" jsonschema:"结束日期 YYYY-MM-DD，上海时区，含当天，可空"`
	Limit        int    `json:"limit,omitempty" jsonschema:"每页条数，默认 50，最大 1000。超过 1000 按 1000 返回，并用 next_cursor 继续翻页"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"上一页返回的 next_cursor。不传则从排序的第一页开始"`
	Order        string `json:"order,omitempty" jsonschema:"按 created_at 排序，asc 或 desc，默认 desc。相同时间再按 id 排序"`
	Verbose      bool   `json:"verbose,omitempty" jsonschema:"为 true 时返回完整 product 和 shop 对象。默认只返回精简字段"`
}

type summarizeMovementsInput struct {
	Type         string   `json:"type,omitempty" jsonschema:"流水类型：inbound、sales_outbound 或 adjustment，可空表示全部类型"`
	From         string   `json:"from,omitempty" jsonschema:"开始日期 YYYY-MM-DD，上海时区，含当天，可空"`
	To           string   `json:"to,omitempty" jsonschema:"结束日期 YYYY-MM-DD，上海时区，含当天，可空"`
	ShopQuery    string   `json:"shop_query,omitempty" jsonschema:"按店铺名称筛选，可空。不唯一时返回候选"`
	ProductQuery string   `json:"product_query,omitempty" jsonschema:"按商品名称或编码筛选，可空。不唯一时返回候选"`
	GroupBy      []string `json:"group_by,omitempty" jsonschema:"分组字段，可组合 shop、product、date。空数组表示只返回总计"`
}

type movementSummaryRow struct {
	ShopName        *string `gorm:"column:shop_name"`
	ProductCode     *string `gorm:"column:product_code"`
	ProductName     *string `gorm:"column:product_name"`
	MovementDate    *string `gorm:"column:movement_date"`
	Quantity        int64   `gorm:"column:quantity"`
	RecordCount     int64   `gorm:"column:record_count"`
	CostAmountCents int64   `gorm:"column:cost_amount_cents"`
}

func (rt *mcpRuntime) listMovements(_ context.Context, _ *mcp.CallToolRequest, in movementsQueryInput) (*mcp.CallToolResult, map[string]any, error) {
	limit, err := movementPageLimit(in.Limit)
	if err != nil {
		return toolFail(err.Error(), map[string]any{"code": "VALIDATION"})
	}
	order, err := movementPageOrder(in.Order)
	if err != nil {
		return toolFail(err.Error(), map[string]any{"code": "VALIDATION"})
	}
	filtered, errResult, errPayload := rt.movementFilter(in.Type, in.ProductQuery, in.ShopQuery, in.From, in.To, false)
	if errResult != nil {
		return errResult, errPayload, nil
	}
	var total int64
	if err := filtered.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return toolFail("failed to count stock movements", nil)
	}
	listQuery := filtered.Session(&gorm.Session{})
	if in.Cursor != "" {
		createdAt, id, err := decodeMovementCursor(in.Cursor)
		if err != nil {
			return toolFail("cursor 无效", map[string]any{"code": "VALIDATION"})
		}
		listQuery = applyMovementCursor(listQuery, createdAt, id, order)
	}
	if order == "asc" {
		listQuery = listQuery.Order("created_at asc").Order("id asc")
	} else {
		listQuery = listQuery.Order("created_at desc").Order("id desc")
	}
	var movements []models.StockMovement
	if err := listQuery.Preload("Product").Preload("Shop").Limit(limit + 1).Find(&movements).Error; err != nil {
		return toolFail("failed to list stock movements", nil)
	}
	var nextCursor any
	if len(movements) > limit {
		movements = movements[:limit]
		last := movements[len(movements)-1]
		nextCursor = encodeMovementCursor(last.CreatedAt, last.ID)
	}
	items := make([]map[string]any, 0, len(movements))
	for _, movement := range movements {
		priced, err := services.CurrentPriceMovement(movement)
		if err != nil {
			return toolFail("failed to calculate movement amounts", nil)
		}
		items = append(items, movementItem(priced, in.Verbose))
	}
	return toolOK(map[string]any{"items": items, "next_cursor": nextCursor, "total_count": total})
}

func (rt *mcpRuntime) summarizeMovements(_ context.Context, _ *mcp.CallToolRequest, in summarizeMovementsInput) (*mcp.CallToolResult, map[string]any, error) {
	movementType, err := normalizeMovementType(in.Type)
	if err != nil {
		return toolFail(err.Error(), map[string]any{"code": "VALIDATION"})
	}
	groups, err := normalizeGroupBy(in.GroupBy)
	if err != nil {
		return toolFail(err.Error(), map[string]any{"code": "VALIDATION"})
	}
	filtered, errResult, errPayload := rt.movementFilter(movementType, in.ProductQuery, in.ShopQuery, in.From, in.To, true)
	if errResult != nil {
		return errResult, errPayload, nil
	}
	groupShop := containsString(groups, "shop")
	groupProduct := containsString(groups, "product")
	groupDate := containsString(groups, "date")
	query := filtered.Joins("JOIN products ON products.id = stock_movements.product_id")
	if groupShop {
		query = query.Joins("LEFT JOIN shops ON shops.id = stock_movements.shop_id")
	}
	selects := []string{
		"COALESCE(SUM(ABS(stock_movements.quantity_delta)),0) AS quantity",
		"COUNT(*) AS record_count",
		"COALESCE(SUM(CASE stock_movements.type WHEN 'sales_outbound' THEN ABS(stock_movements.quantity_delta) * products.default_purchase_cents WHEN 'adjustment' THEN stock_movements.quantity_delta * products.default_purchase_cents ELSE 0 END),0) AS cost_amount_cents",
	}
	groupExprs := make([]string, 0, 5)
	orders := make([]string, 0, 3)
	if groupShop {
		selects = append(selects, "shops.name AS shop_name")
		groupExprs = append(groupExprs, "shops.id", "shops.name")
		orders = append(orders, "shops.name asc")
	}
	if groupProduct {
		selects = append(selects, "products.code AS product_code", "products.name AS product_name")
		groupExprs = append(groupExprs, "products.id", "products.code", "products.name")
		orders = append(orders, "products.code asc")
	}
	if groupDate {
		dateExpr := movementShanghaiDateExpr(rt.db)
		selects = append(selects, dateExpr+" AS movement_date")
		groupExprs = append(groupExprs, dateExpr)
		orders = append(orders, "movement_date asc")
	}
	query = query.Select(strings.Join(selects, ", "))
	if len(groupExprs) > 0 {
		query = query.Group(strings.Join(groupExprs, ", "))
	}
	for _, order := range orders {
		query = query.Order(order)
	}
	rows := make([]movementSummaryRow, 0)
	if err := query.Scan(&rows).Error; err != nil {
		return toolFail("failed to summarize movements", nil)
	}
	if len(groups) == 0 && len(rows) == 1 && rows[0].RecordCount == 0 {
		rows = nil
	}
	items := make([]map[string]any, 0, len(rows))
	var totalQuantity int64
	var totalCount int64
	var totalCost int64
	for _, row := range rows {
		item := map[string]any{
			"quantity":          row.Quantity,
			"record_count":      row.RecordCount,
			"cost_amount_cents": row.CostAmountCents,
		}
		if groupShop {
			if row.ShopName == nil {
				item["shop_name"] = nil
			} else {
				item["shop_name"] = *row.ShopName
			}
		}
		if groupProduct {
			item["product_code"] = stringValue(row.ProductCode)
			item["product_name"] = stringValue(row.ProductName)
		}
		if groupDate {
			item["date"] = stringValue(row.MovementDate)
		}
		items = append(items, item)
		totalQuantity += row.Quantity
		totalCount += row.RecordCount
		totalCost += row.CostAmountCents
	}
	return toolOK(map[string]any{
		"items": items,
		"totals": map[string]any{
			"quantity":          totalQuantity,
			"record_count":      totalCount,
			"cost_amount_cents": totalCost,
		},
	})
}

func (rt *mcpRuntime) movementFilter(movementType string, productQuery string, shopQuery string, fromRaw string, toRaw string, strictDates bool) (*gorm.DB, *mcp.CallToolResult, map[string]any) {
	query := rt.db.Model(&models.StockMovement{})
	if movementType != "" {
		query = query.Where("stock_movements.type = ?", movementType)
	}
	if strings.TrimSpace(productQuery) != "" {
		product, refs, err := services.ResolveProduct(rt.db, "", "", productQuery)
		if err != nil {
			result, payload, _ := catalogFail(err, refs)
			return nil, result, payload
		}
		query = query.Where("stock_movements.product_id = ?", product.ID)
	}
	if strings.TrimSpace(shopQuery) != "" {
		shop, refs, err := services.ResolveShop(rt.db, "", shopQuery)
		if err != nil {
			result, payload, _ := catalogFail(err, refs)
			return nil, result, payload
		}
		query = query.Where("stock_movements.shop_id = ?", shop.ID)
	}
	if strings.TrimSpace(fromRaw) != "" {
		from, ok := parseShanghaiDay(fromRaw)
		if !ok {
			if strictDates {
				result, payload, _ := toolFail("from 须为上海时区 YYYY-MM-DD", map[string]any{"code": "VALIDATION"})
				return nil, result, payload
			}
		} else {
			query = query.Where("stock_movements.created_at >= ?", from)
		}
	}
	if strings.TrimSpace(toRaw) != "" {
		to, ok := parseShanghaiDay(toRaw)
		if !ok {
			if strictDates {
				result, payload, _ := toolFail("to 须为上海时区 YYYY-MM-DD", map[string]any{"code": "VALIDATION"})
				return nil, result, payload
			}
		} else {
			query = query.Where("stock_movements.created_at < ?", to.AddDate(0, 0, 1))
		}
	}
	return query, nil, nil
}

func movementItem(priced models.StockMovement, verbose bool) map[string]any {
	if verbose {
		return map[string]any{
			"id":                    priced.ID,
			"type":                  priced.Type,
			"product":               priced.Product,
			"shop":                  priced.Shop,
			"quantity_delta":        priced.QuantityDelta,
			"reason":                priced.Reason,
			"created_at":            priced.CreatedAt,
			"purchase_amount_cents": priced.PurchaseAmountCents,
			"cost_amount_cents":     priced.CostAmountCents,
		}
	}
	var shopName any
	if priced.Shop != nil {
		shopName = priced.Shop.Name
	}
	return map[string]any{
		"id":                priced.ID,
		"type":              priced.Type,
		"created_at":        priced.CreatedAt,
		"product_code":      priced.Product.Code,
		"product_name":      priced.Product.Name,
		"shop_name":         shopName,
		"quantity_delta":    priced.QuantityDelta,
		"cost_amount_cents": priced.CostAmountCents,
		"note":              priced.Reason,
	}
}

func movementPageLimit(raw int) (int, error) {
	if raw == 0 {
		return movementDefaultLimit, nil
	}
	if raw < 0 {
		return 0, fmt.Errorf("limit 必须大于 0")
	}
	if raw > movementMaxLimit {
		return movementMaxLimit, nil
	}
	return raw, nil
}

func movementPageOrder(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "desc":
		return "desc", nil
	case "asc":
		return "asc", nil
	default:
		return "", fmt.Errorf("order 只能是 asc 或 desc")
	}
}

func encodeMovementCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeMovementCursor(raw string) (time.Time, uuid.UUID, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	createdRaw, idRaw, ok := strings.Cut(string(decoded), "|")
	if !ok {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return createdAt, id, nil
}

func applyMovementCursor(query *gorm.DB, createdAt time.Time, id uuid.UUID, order string) *gorm.DB {
	if order == "asc" {
		return query.Where("(stock_movements.created_at > ? OR (stock_movements.created_at = ? AND stock_movements.id > ?))", createdAt, createdAt, id)
	}
	return query.Where("(stock_movements.created_at < ? OR (stock_movements.created_at = ? AND stock_movements.id < ?))", createdAt, createdAt, id)
}

func normalizeMovementType(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return "", nil
	case string(models.MovementTypeInbound), string(models.MovementTypeSalesOutbound), string(models.MovementTypeAdjustment):
		return strings.TrimSpace(raw), nil
	default:
		return "", fmt.Errorf("type 只能是 inbound、sales_outbound 或 adjustment")
	}
}

func normalizeGroupBy(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	groups := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		switch key {
		case "shop", "product", "date":
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			groups = append(groups, key)
		default:
			return nil, fmt.Errorf("group_by 只能包含 shop、product、date")
		}
	}
	return groups, nil
}

func movementShanghaiDateExpr(db *gorm.DB) string {
	if db.Dialector.Name() == "postgres" {
		return "TO_CHAR(stock_movements.created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')"
	}
	return "STRFTIME('%Y-%m-%d', stock_movements.created_at, '+8 hours')"
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
