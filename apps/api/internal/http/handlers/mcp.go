package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
	Query    string `json:"q,omitempty" jsonschema:"按商品名称或编码搜索，可空"`
	LowStock bool   `json:"low_stock,omitempty" jsonschema:"为 true 时只返回低于预警阈值的商品"`
}

type movementsQueryInput struct {
	Type         string `json:"type,omitempty" jsonschema:"流水类型：inbound、sales_outbound 或 adjustment，可空"`
	ProductQuery string `json:"product_query,omitempty" jsonschema:"按商品名称或编码筛选，可空"`
	ShopQuery    string `json:"shop_query,omitempty" jsonschema:"按店铺名称筛选，可空"`
	From         string `json:"from,omitempty" jsonschema:"开始日期 YYYY-MM-DD，上海时区，可空"`
	To           string `json:"to,omitempty" jsonschema:"结束日期 YYYY-MM-DD，上海时区，可空"`
}

type inboundInput struct {
	ProductID    string `json:"product_id,omitempty" jsonschema:"商品 UUID，可与名称/编码三选一"`
	ProductCode  string `json:"product_code,omitempty" jsonschema:"商品编码，优先精确匹配"`
	ProductQuery string `json:"product_query,omitempty" jsonschema:"商品名称或编码，用户说绿茶时填绿茶"`
	ShopID       string `json:"shop_id,omitempty" jsonschema:"店铺 UUID，可空"`
	ShopQuery    string `json:"shop_query,omitempty" jsonschema:"店铺名称，可空"`
	Quantity     int64  `json:"quantity" jsonschema:"入库数量，必须大于 0"`
	Note         string `json:"note,omitempty" jsonschema:"备注，可空"`
}

type outboundInput struct {
	ProductID    string `json:"product_id,omitempty" jsonschema:"商品 UUID，可与名称/编码三选一"`
	ProductCode  string `json:"product_code,omitempty" jsonschema:"商品编码，优先精确匹配"`
	ProductQuery string `json:"product_query,omitempty" jsonschema:"商品名称或编码，用户说绿茶时填绿茶"`
	ShopID       string `json:"shop_id,omitempty" jsonschema:"店铺 UUID，可与店铺名称二选一"`
	ShopQuery    string `json:"shop_query,omitempty" jsonschema:"店铺名称，例如总店"`
	Quantity     int64  `json:"quantity" jsonschema:"出库数量，必须大于 0"`
	Note         string `json:"note,omitempty" jsonschema:"备注，可空"`
}

type adjustmentInput struct {
	ProductID     string `json:"product_id,omitempty" jsonschema:"商品 UUID，可与名称/编码三选一"`
	ProductCode   string `json:"product_code,omitempty" jsonschema:"商品编码，优先精确匹配"`
	ProductQuery  string `json:"product_query,omitempty" jsonschema:"商品名称或编码，用户说绿茶时填绿茶"`
	QuantityDelta int64  `json:"quantity_delta" jsonschema:"库存增减，正数增加负数减少，不能为 0"`
	Reason        string `json:"reason" jsonschema:"调整原因，必填"`
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
			Description: "按名称或编码搜索商品。用户问有哪些茶、绿茶的编码是什么时使用。",
		}, rt.listProducts)
	}
	if rt.can(services.PermShopRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_shops",
			Title:       "查店铺",
			Description: "列出店铺或按名称查找。出库前若不知道店铺全称，先用这个工具。",
		}, rt.listShops)
	}
	if rt.can(services.PermInventoryRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_inventory",
			Title:       "查库存",
			Description: "查询当前库存数量。用户说「绿茶还有多少」「哪些缺货」时使用。low_stock=true 只看低于预警的商品。",
		}, rt.getInventory)
	}
	if rt.can(services.PermMovementRead) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_stock_movements",
			Title:       "查流水",
			Description: "查询入库、销售出库或调整流水。可按商品、店铺和日期筛选。",
		}, rt.listMovements)
	}
	if rt.can(services.PermInventoryInbound) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_inbound",
			Title:       "入库",
			Description: "商品入库，调用成功立即记账。用商品编码或名称，不必填 UUID。店铺可选。例如「绿茶入 20」。名称不唯一时不要猜测，把候选告诉用户。",
		}, rt.createInbound)
	}
	if rt.can(services.PermInventorySalesOutbound) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_sales_outbound",
			Title:       "销售出库",
			Description: "销售出库，调用成功立即记账。必须有商品和店铺，用名称或编码即可。例如「给总店出库 2 件绿茶」。名称不唯一时不要猜测。",
		}, rt.createOutbound)
	}
	if rt.can(services.PermInventoryAdjust) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_adjustment",
			Title:       "库存调整",
			Description: "盘点或纠错时调整库存，调用成功立即记账。quantity_delta 为有符号整数，必须填写原因。",
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
	query := rt.db.Model(&models.InventorySnapshot{}).
		Joins("JOIN products ON products.id = inventory_snapshots.product_id").
		Where("products.archived_at IS NULL")
	if in.LowStock {
		query = query.Where("products.low_stock_threshold > 0 AND inventory_snapshots.quantity <= products.low_stock_threshold")
	}
	if keyword := in.Query; keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("LOWER(products.name) LIKE LOWER(?) OR LOWER(products.code) LIKE LOWER(?)", like, like)
	}
	var snapshots []models.InventorySnapshot
	if err := query.Preload("Product").Order("products.name asc").Limit(100).Find(&snapshots).Error; err != nil {
		return toolFail("failed to list inventory", nil)
	}
	items := make([]map[string]any, 0, len(snapshots))
	for _, snapshot := range snapshots {
		value, err := services.CurrentInventoryValue(snapshot)
		if err != nil {
			return toolFail("failed to calculate inventory value", nil)
		}
		items = append(items, map[string]any{
			"product_id":            snapshot.ProductID,
			"product":               snapshot.Product,
			"quantity":              snapshot.Quantity,
			"inventory_value_cents": value,
		})
	}
	return toolOK(map[string]any{"items": items})
}

func (rt *mcpRuntime) listMovements(_ context.Context, _ *mcp.CallToolRequest, in movementsQueryInput) (*mcp.CallToolResult, map[string]any, error) {
	query := rt.db.Model(&models.StockMovement{})
	if in.Type != "" {
		query = query.Where("type = ?", in.Type)
	}
	if in.ProductQuery != "" {
		product, refs, err := services.ResolveProduct(rt.db, "", "", in.ProductQuery)
		if err != nil {
			return catalogFail(err, refs)
		}
		query = query.Where("product_id = ?", product.ID)
	}
	if in.ShopQuery != "" {
		shop, refs, err := services.ResolveShop(rt.db, "", in.ShopQuery)
		if err != nil {
			return catalogFail(err, refs)
		}
		query = query.Where("shop_id = ?", shop.ID)
	}
	if from, ok := parseShanghaiDay(in.From); ok {
		query = query.Where("created_at >= ?", from)
	}
	if to, ok := parseShanghaiDay(in.To); ok {
		query = query.Where("created_at < ?", to.AddDate(0, 0, 1))
	}
	var movements []models.StockMovement
	if err := query.Preload("Product").Preload("Shop").Preload("Operator").
		Order("created_at desc").Limit(50).Find(&movements).Error; err != nil {
		return toolFail("failed to list stock movements", nil)
	}
	items := make([]map[string]any, 0, len(movements))
	for _, movement := range movements {
		priced, err := services.CurrentPriceMovement(movement)
		if err != nil {
			return toolFail("failed to calculate movement amounts", nil)
		}
		items = append(items, map[string]any{
			"id":                    priced.ID,
			"type":                  priced.Type,
			"product":               priced.Product,
			"shop":                  priced.Shop,
			"quantity_delta":        priced.QuantityDelta,
			"reason":                priced.Reason,
			"created_at":            priced.CreatedAt,
			"purchase_amount_cents": priced.PurchaseAmountCents,
			"cost_amount_cents":     priced.CostAmountCents,
		})
	}
	return toolOK(map[string]any{"items": items})
}

func (rt *mcpRuntime) createInbound(_ context.Context, _ *mcp.CallToolRequest, in inboundInput) (*mcp.CallToolResult, map[string]any, error) {
	product, refs, err := services.ResolveProduct(rt.db, in.ProductID, in.ProductCode, in.ProductQuery)
	if err != nil {
		return catalogFail(err, refs)
	}
	var shopID *uuid.UUID
	if in.ShopID != "" || in.ShopQuery != "" {
		shop, shopRefs, shopErr := services.ResolveShop(rt.db, in.ShopID, in.ShopQuery)
		if shopErr != nil {
			return catalogFail(shopErr, shopRefs)
		}
		shopID = &shop.ID
	}
	change, err := services.InventoryService{DB: rt.db}.CreateInbound(services.InboundInput{
		ProductID: product.ID, ShopID: shopID, Quantity: in.Quantity, Note: in.Note, OperatorID: rt.user.ID,
	})
	if res, out, callErr, failed := stockToolError(err); failed {
		return res, out, callErr
	}
	metadata := map[string]string{"quantity": strconv.FormatInt(in.Quantity, 10)}
	if shopID != nil {
		metadata["shop_id"] = shopID.String()
	}
	if change.Movement.Reason != "" {
		metadata["note"] = change.Movement.Reason
	}
	recordAudit(rt.c, rt.db, "inventory.inbound", "product", product.ID.String(), metadata)
	if rt.lark != nil {
		rt.lark.Enqueue(change)
	}
	return toolOK(map[string]any{"ok": true, "product": product, "quantity_after": change.QuantityAfter})
}

func (rt *mcpRuntime) createOutbound(_ context.Context, _ *mcp.CallToolRequest, in outboundInput) (*mcp.CallToolResult, map[string]any, error) {
	product, refs, err := services.ResolveProduct(rt.db, in.ProductID, in.ProductCode, in.ProductQuery)
	if err != nil {
		return catalogFail(err, refs)
	}
	shop, shopRefs, err := services.ResolveShop(rt.db, in.ShopID, in.ShopQuery)
	if err != nil {
		return catalogFail(err, shopRefs)
	}
	change, err := services.InventoryService{DB: rt.db}.CreateSalesOutbound(services.OutboundInput{
		ProductID: product.ID, ShopID: shop.ID, Quantity: in.Quantity, Note: in.Note, OperatorID: rt.user.ID,
	})
	if res, out, callErr, failed := stockToolError(err); failed {
		return res, out, callErr
	}
	metadata := map[string]string{"quantity": strconv.FormatInt(in.Quantity, 10), "shop_id": shop.ID.String()}
	if change.Movement.Reason != "" {
		metadata["note"] = change.Movement.Reason
	}
	recordAudit(rt.c, rt.db, "inventory.sales_outbound", "product", product.ID.String(), metadata)
	if rt.lark != nil {
		rt.lark.Enqueue(change)
	}
	return toolOK(map[string]any{"ok": true, "product": product, "shop": shop, "quantity_after": change.QuantityAfter})
}

func (rt *mcpRuntime) createAdjustment(_ context.Context, _ *mcp.CallToolRequest, in adjustmentInput) (*mcp.CallToolResult, map[string]any, error) {
	product, refs, err := services.ResolveProduct(rt.db, in.ProductID, in.ProductCode, in.ProductQuery)
	if err != nil {
		return catalogFail(err, refs)
	}
	change, err := services.InventoryService{DB: rt.db}.CreateAdjustment(services.AdjustmentInput{
		ProductID: product.ID, QuantityDelta: in.QuantityDelta, Reason: in.Reason, OperatorID: rt.user.ID,
	})
	if res, out, callErr, failed := stockToolError(err); failed {
		return res, out, callErr
	}
	recordAudit(rt.c, rt.db, "inventory.adjustment", "product", product.ID.String(), map[string]string{
		"quantity_delta": strconv.FormatInt(in.QuantityDelta, 10), "reason": in.Reason,
	})
	if rt.lark != nil {
		rt.lark.Enqueue(change)
	}
	return toolOK(map[string]any{"ok": true, "product": product, "quantity_after": change.QuantityAfter})
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
