package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

type MCPHandler struct {
	DB   *gorm.DB
	Lark *services.LarkNotifier
}

type mcpRuntime struct {
	c     *gin.Context
	db    *gorm.DB
	user  models.User
	perms map[string]struct{}
	lark  *services.LarkNotifier
}

type queryInput struct {
	Query string `json:"q,omitempty" jsonschema:"按商品名称或编码搜索，可空"`
}

type inventoryQueryInput struct {
	Query    string   `json:"q,omitempty" jsonschema:"按商品名称或编码模糊搜索，可空。与 codes 同时传时取并集"`
	LowStock bool     `json:"low_stock,omitempty" jsonschema:"为 true 时只过滤关键词列表中低于预警阈值的商品，不隐藏 codes 指定的商品"`
	Codes    []string `json:"codes,omitempty" jsonschema:"商品编码数组，精确匹配，最多 1000 个。不存在的编码返回 not_found=true，不会省略"`
	Verbose  bool     `json:"verbose,omitempty" jsonschema:"为 true 时返回完整商品对象和 inventory_value_cents。默认返回 product_code、product_name、quantity、low_stock_threshold"`
}

func (h MCPHandler) Serve(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
		return
	}
	runtime := &mcpRuntime{c: c, db: h.DB, user: user, perms: currentPermissionSet(c), lark: h.Lark}
	server := mcp.NewServer(&mcp.Implementation{Name: "gaowang", Title: "高旺库存", Version: "1.0.0"}, nil)
	runtime.addTools(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		DisableLocalhostProtection:   true,
		PropagateRequestCancellation: true,
	})
	handler.ServeHTTP(c.Writer, c.Request)
}

func (rt *mcpRuntime) addTools(server *mcp.Server) {
	if rt.can(services.PermProductRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_products",
			Title:       "查商品",
			Description: "按名称或编码搜索未归档商品，最多 100 条。用户问有哪些茶、绿茶的编码是什么时使用。",
		}, rt.listProducts)
	}
	if rt.can(services.PermShopRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_shops",
			Title:       "查店铺",
			Description: "列出店铺或按名称查找，最多 100 条。出库前若不知道店铺全称，先用这个工具。",
		}, rt.listShops)
	}
	if rt.can(services.PermInventoryRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_inventory",
			Title:       "查库存",
			Description: "查询当前库存。金额单位为分。默认返回 product_code、product_name、quantity、low_stock_threshold；verbose=true 才返回完整商品对象。codes 精确匹配编码，最多 1000 个，不存在的编码返回 not_found=true。与 q 同时传时取并集。low_stock 只过滤关键词列表。不传 codes 时最多 100 条。",
		}, rt.getInventory)
	}
	if rt.can(services.PermMovementRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_stock_movements",
			Title:       "查流水",
			Description: "查询入库、销售出库或调整流水。from/to 为上海时区 YYYY-MM-DD，含首尾两天。金额单位为分，cost_amount_cents 按当前进货价计算。默认按 created_at 降序，limit 默认 50、最大 1000，用 next_cursor 翻页；没有下一页时 next_cursor 为 null，total_count 是筛选后的总条数。默认精简字段；verbose=true 返回完整 product 和 shop。",
		}, rt.listMovements)
	}
	if rt.can(services.PermMovementRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "summarize_movements",
			Title:       "汇总流水",
			Description: "在数据库中一次聚合全部匹配流水，不受 50 条分页限制。type 可空：inbound、sales_outbound、adjustment。from/to 为上海时区 YYYY-MM-DD。group_by 可组合 shop、product、date。quantity 为数量绝对值之和，出库即销量。cost_amount_cents 单位为分，与逐条流水的 cost_amount_cents 一致（按当前进货价；入库为 0）。返回 items 和 totals。",
		}, rt.summarizeMovements)
	}
	if rt.can(services.PermInventoryInbound) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_inbound",
			Title:       "入库",
			Description: "商品入库，调用成功立即记账。金额单位为分。用商品编码或名称，不必填 UUID。店铺可选。名称不唯一时不要猜测，把候选告诉用户。request_id 可选，同一账号 7 天内相同参数不重复记账，第二次返回 duplicate=true；参数不同则报错。默认返回精简字段，verbose=true 返回完整商品对象。",
		}, rt.createInbound)
	}
	if rt.can(services.PermInventorySalesOutbound) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_sales_outbound",
			Title:       "销售出库",
			Description: "销售出库，调用成功立即记账。金额单位为分。必须有商品和店铺，用名称或编码即可。名称不唯一时不要猜测。request_id 可选，同一账号 7 天内相同参数不重复记账，第二次返回 duplicate=true；参数不同则报错。默认返回精简字段，verbose=true 返回完整商品和店铺对象。",
		}, rt.createOutbound)
	}
	if rt.can(services.PermInventorySalesOutbound) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_sales_outbound_batch",
			Title:       "批量销售出库",
			Description: "同一店铺批量销售出库，金额单位为分。items 用 product_code 或 product_id，quantity 必须大于 0，最多 200 项。先校验全部商品和库存，任一项失败则整批不记账，并返回每一项失败原因。全部通过时在同一个事务写入。dry_run=true 只校验并返回预计结果，不记账。request_id 可选，成功记账后同一账号 7 天内相同参数重试返回第一次结果且 duplicate=true，参数不同则报错不记账。",
		}, rt.createOutboundBatch)
	}
	if rt.can(services.PermInventoryAdjust) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_adjustment",
			Title:       "库存调整",
			Description: "盘点或纠错时调整库存，调用成功立即记账。quantity_delta 为有符号整数，不能为 0，必须填写原因。金额单位为分。request_id 可选，同一账号 7 天内相同参数不重复记账，第二次返回 duplicate=true；参数不同则报错。默认返回精简字段，verbose=true 返回完整商品对象。",
		}, rt.createAdjustment)
	}
}

func (rt *mcpRuntime) listProducts(_ context.Context, _ *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, map[string]any, error) {
	query := rt.db.Model(&models.Product{}).Where("archived_at IS NULL")
	if keyword := in.Query; keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(code) LIKE LOWER(?)", like, like)
	}
	var products []models.Product
	if err := query.Order("name asc").Order("code asc").Limit(100).Find(&products).Error; err != nil {
		return toolFail("failed to list products", nil)
	}
	return toolOK(map[string]any{"items": products})
}

func (rt *mcpRuntime) listShops(_ context.Context, _ *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, map[string]any, error) {
	query := rt.db.Model(&models.Shop{})
	if keyword := in.Query; keyword != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+keyword+"%")
	}
	var shops []models.Shop
	if err := query.Order("name asc").Limit(100).Find(&shops).Error; err != nil {
		return toolFail("failed to list shops", nil)
	}
	return toolOK(map[string]any{"items": shops})
}

func (rt *mcpRuntime) getInventory(_ context.Context, _ *mcp.CallToolRequest, in inventoryQueryInput) (*mcp.CallToolResult, map[string]any, error) {
	codes := normalizeInventoryCodes(in.Codes)
	if len(codes) > inventoryCodeLimit {
		return toolFail("codes 最多 1000 个", map[string]any{"code": "VALIDATION"})
	}
	keyword := strings.TrimSpace(in.Query)
	items := make([]map[string]any, 0)
	seen := map[uuid.UUID]struct{}{}
	if len(codes) == 0 || keyword != "" {
		query := rt.db.Model(&models.InventorySnapshot{}).
			Joins("JOIN products ON products.id = inventory_snapshots.product_id").
			Where("products.archived_at IS NULL")
		if in.LowStock {
			query = query.Where("products.low_stock_threshold > 0 AND inventory_snapshots.quantity <= products.low_stock_threshold")
		}
		if keyword != "" {
			like := "%" + keyword + "%"
			query = query.Where("LOWER(products.name) LIKE LOWER(?) OR LOWER(products.code) LIKE LOWER(?)", like, like)
		}
		var snapshots []models.InventorySnapshot
		if err := query.Preload("Product").Order("products.name asc").Limit(100).Find(&snapshots).Error; err != nil {
			return toolFail("failed to list inventory", nil)
		}
		for _, snapshot := range snapshots {
			item, err := inventoryItem(snapshot, in.Verbose)
			if err != nil {
				return toolFail("failed to calculate inventory value", nil)
			}
			items = append(items, item)
			seen[snapshot.ProductID] = struct{}{}
		}
	}
	if len(codes) > 0 {
		extra, err := rt.inventoryByCodes(codes, seen, in.Verbose)
		if err != nil {
			return toolFail("failed to list inventory", nil)
		}
		items = append(items, extra...)
	}
	return toolOK(map[string]any{"items": items})
}

func (rt *mcpRuntime) inventoryByCodes(codes []string, seen map[uuid.UUID]struct{}, verbose bool) ([]map[string]any, error) {
	lower := make([]string, len(codes))
	for i, code := range codes {
		lower[i] = strings.ToLower(code)
	}
	var products []models.Product
	if err := rt.db.Where("archived_at IS NULL AND (code IN ? OR LOWER(code) IN ?)", codes, lower).Find(&products).Error; err != nil {
		return nil, err
	}
	exact := make(map[string]models.Product, len(products))
	folded := make(map[string][]models.Product, len(products))
	ids := make([]uuid.UUID, 0, len(products))
	for _, product := range products {
		exact[product.Code] = product
		key := strings.ToLower(product.Code)
		folded[key] = append(folded[key], product)
		ids = append(ids, product.ID)
	}
	quantities := map[uuid.UUID]int64{}
	if len(ids) > 0 {
		var snapshots []models.InventorySnapshot
		if err := rt.db.Where("product_id IN ?", ids).Find(&snapshots).Error; err != nil {
			return nil, err
		}
		for _, snapshot := range snapshots {
			quantities[snapshot.ProductID] = snapshot.Quantity
		}
	}
	items := make([]map[string]any, 0, len(codes))
	missing := map[string]struct{}{}
	for _, code := range codes {
		product, ok := matchInventoryCode(code, exact, folded)
		if !ok {
			if _, emitted := missing[code]; emitted {
				continue
			}
			missing[code] = struct{}{}
			items = append(items, map[string]any{"product_code": code, "not_found": true})
			continue
		}
		if _, emitted := seen[product.ID]; emitted {
			continue
		}
		seen[product.ID] = struct{}{}
		item, err := inventoryItem(models.InventorySnapshot{
			ProductID: product.ID, Product: product, Quantity: quantities[product.ID],
		}, verbose)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func normalizeInventoryCodes(raw []string) []string {
	codes := make([]string, 0, len(raw))
	for _, code := range raw {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		codes = append(codes, code)
	}
	return codes
}

func matchInventoryCode(code string, exact map[string]models.Product, folded map[string][]models.Product) (models.Product, bool) {
	if product, ok := exact[code]; ok {
		return product, true
	}
	matches := folded[strings.ToLower(code)]
	if len(matches) == 1 {
		return matches[0], true
	}
	return models.Product{}, false
}

func inventoryItem(snapshot models.InventorySnapshot, verbose bool) (map[string]any, error) {
	if verbose {
		value, err := services.CurrentInventoryValue(snapshot)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"product_id":            snapshot.ProductID,
			"product":               snapshot.Product,
			"quantity":              snapshot.Quantity,
			"inventory_value_cents": value,
		}, nil
	}
	return map[string]any{
		"product_code":        snapshot.Product.Code,
		"product_name":        snapshot.Product.Name,
		"quantity":            snapshot.Quantity,
		"low_stock_threshold": snapshot.Product.LowStockThreshold,
	}, nil
}

func (rt *mcpRuntime) can(permission string) bool {
	if rt.user.Role == models.RoleAdmin {
		return true
	}
	return services.HasPermission(rt.perms, permission)
}

func stockToolError(err error) (*mcp.CallToolResult, map[string]any, error, bool) {
	if err == nil {
		return nil, nil, nil, false
	}
	if errors.Is(err, services.ErrInsufficientStock) {
		res, out, callErr := toolFail(err.Error(), map[string]any{"code": "INSUFFICIENT_STOCK"})
		return res, out, callErr, true
	}
	if errors.Is(err, services.ErrProductArchived) {
		res, out, callErr := toolFail("商品已归档，不能进行库存操作", map[string]any{"code": "PRODUCT_ARCHIVED"})
		return res, out, callErr, true
	}
	res, out, callErr := toolFail(err.Error(), map[string]any{"code": "STOCK_OPERATION_FAILED"})
	return res, out, callErr, true
}

func currentPermissionSet(c *gin.Context) map[string]struct{} {
	value, ok := c.Get("current_permissions")
	if !ok {
		return map[string]struct{}{}
	}
	set, ok := value.(map[string]struct{})
	if !ok {
		return map[string]struct{}{}
	}
	return set
}

func catalogFail(err error, refs []services.CatalogRef) (*mcp.CallToolResult, map[string]any, error) {
	if refs == nil {
		refs = []services.CatalogRef{}
	}
	return toolFail(err.Error(), map[string]any{"candidates": refs})
}

func toolOK(output map[string]any) (*mcp.CallToolResult, map[string]any, error) {
	return nil, output, nil
}

func toolFail(message string, extra map[string]any) (*mcp.CallToolResult, map[string]any, error) {
	payload := map[string]any{"error": message}
	for key, value := range extra {
		payload[key] = value
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"error":"internal"}`)
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, payload, nil
}

func parseShanghaiDay(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	value, err := time.ParseInLocation("2006-01-02", raw, time.FixedZone("Asia/Shanghai", 8*60*60))
	if err != nil {
		return time.Time{}, false
	}
	return value.UTC(), true
}
