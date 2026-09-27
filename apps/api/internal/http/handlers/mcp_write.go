package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

type inboundInput struct {
	ProductID    string `json:"product_id,omitempty" jsonschema:"商品 UUID，可与名称/编码三选一"`
	ProductCode  string `json:"product_code,omitempty" jsonschema:"商品编码，优先精确匹配"`
	ProductQuery string `json:"product_query,omitempty" jsonschema:"商品名称或编码，用户说绿茶时填绿茶"`
	ShopID       string `json:"shop_id,omitempty" jsonschema:"店铺 UUID，可空"`
	ShopQuery    string `json:"shop_query,omitempty" jsonschema:"店铺名称，可空"`
	Quantity     int64  `json:"quantity" jsonschema:"入库数量，必须大于 0"`
	Note         string `json:"note,omitempty" jsonschema:"备注，可空，最多 500 字"`
	RequestID    string `json:"request_id,omitempty" jsonschema:"调用方生成的幂等键，例如 UUID。同一账号 7 天内相同参数不重复记账，第二次返回 duplicate=true；参数不同则报错且不记账。可空"`
	Verbose      bool   `json:"verbose,omitempty" jsonschema:"为 true 时返回完整商品对象。默认只返回编码、名称和记账后数量"`
}

type outboundInput struct {
	ProductID    string `json:"product_id,omitempty" jsonschema:"商品 UUID，可与名称/编码三选一"`
	ProductCode  string `json:"product_code,omitempty" jsonschema:"商品编码，优先精确匹配"`
	ProductQuery string `json:"product_query,omitempty" jsonschema:"商品名称或编码，用户说绿茶时填绿茶"`
	ShopID       string `json:"shop_id,omitempty" jsonschema:"店铺 UUID，可与店铺名称二选一"`
	ShopQuery    string `json:"shop_query,omitempty" jsonschema:"店铺名称，例如总店"`
	Quantity     int64  `json:"quantity" jsonschema:"出库数量，必须大于 0"`
	Note         string `json:"note,omitempty" jsonschema:"备注，可空，最多 500 字"`
	RequestID    string `json:"request_id,omitempty" jsonschema:"调用方生成的幂等键，例如 UUID。同一账号 7 天内相同参数不重复记账，第二次返回 duplicate=true；参数不同则报错且不记账。可空"`
	Verbose      bool   `json:"verbose,omitempty" jsonschema:"为 true 时返回完整商品和店铺对象。默认返回精简字段，金额单位为分"`
}

type adjustmentInput struct {
	ProductID     string `json:"product_id,omitempty" jsonschema:"商品 UUID，可与名称/编码三选一"`
	ProductCode   string `json:"product_code,omitempty" jsonschema:"商品编码，优先精确匹配"`
	ProductQuery  string `json:"product_query,omitempty" jsonschema:"商品名称或编码，用户说绿茶时填绿茶"`
	QuantityDelta int64  `json:"quantity_delta" jsonschema:"库存增减，正数增加负数减少，不能为 0"`
	Reason        string `json:"reason" jsonschema:"调整原因，必填"`
	RequestID     string `json:"request_id,omitempty" jsonschema:"调用方生成的幂等键，例如 UUID。同一账号 7 天内相同参数不重复记账，第二次返回 duplicate=true；参数不同则报错且不记账。可空"`
	Verbose       bool   `json:"verbose,omitempty" jsonschema:"为 true 时返回完整商品对象。默认只返回编码、名称、调整量和记账后数量"`
}

type outboundBatchItemInput struct {
	ProductID   string `json:"product_id,omitempty" jsonschema:"商品 UUID，可与 product_code 二选一"`
	ProductCode string `json:"product_code,omitempty" jsonschema:"商品编码，精确匹配"`
	Quantity    int64  `json:"quantity" jsonschema:"出库数量，必须大于 0"`
}

type outboundBatchInput struct {
	ShopID    string                   `json:"shop_id,omitempty" jsonschema:"店铺 UUID，整批同一店铺，可与 shop_query 二选一"`
	ShopQuery string                   `json:"shop_query,omitempty" jsonschema:"店铺名称，整批同一店铺"`
	Note      string                   `json:"note,omitempty" jsonschema:"整批备注，可空，最多 500 字"`
	RequestID string                   `json:"request_id,omitempty" jsonschema:"调用方生成的幂等键，例如 UUID。成功记账后 7 天内相同参数重试返回第一次结果且 duplicate=true；参数不同则报错且不记账。dry_run 不占用幂等键"`
	DryRun    bool                     `json:"dry_run,omitempty" jsonschema:"为 true 时只校验并返回预计结果，不记账"`
	Verbose   bool                     `json:"verbose,omitempty" jsonschema:"为 true 时额外返回完整商品和店铺对象。默认返回精简字段，金额单位为分"`
	Items     []outboundBatchItemInput `json:"items" jsonschema:"出库明细，最多 200 项。任一项失败则整批不记账"`
}

type mcpAuditEvent struct {
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]string
}

type mcpCatalogError struct {
	err  error
	refs []services.CatalogRef
}

func (e *mcpCatalogError) Error() string {
	if e == nil || e.err == nil {
		return "catalog lookup failed"
	}
	return e.err.Error()
}

func (e *mcpCatalogError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

type inboundFingerprint struct {
	ProductID    string `json:"product_id"`
	ProductCode  string `json:"product_code"`
	ProductQuery string `json:"product_query"`
	ShopID       string `json:"shop_id"`
	ShopQuery    string `json:"shop_query"`
	Quantity     int64  `json:"quantity"`
	Note         string `json:"note"`
}

type outboundFingerprint struct {
	ProductID    string `json:"product_id"`
	ProductCode  string `json:"product_code"`
	ProductQuery string `json:"product_query"`
	ShopID       string `json:"shop_id"`
	ShopQuery    string `json:"shop_query"`
	Quantity     int64  `json:"quantity"`
	Note         string `json:"note"`
}

type adjustmentFingerprint struct {
	ProductID     string `json:"product_id"`
	ProductCode   string `json:"product_code"`
	ProductQuery  string `json:"product_query"`
	QuantityDelta int64  `json:"quantity_delta"`
	Reason        string `json:"reason"`
}

type batchItemFingerprint struct {
	ProductID   string `json:"product_id"`
	ProductCode string `json:"product_code"`
	Quantity    int64  `json:"quantity"`
}

type batchFingerprint struct {
	ShopID    string                 `json:"shop_id"`
	ShopQuery string                 `json:"shop_query"`
	Note      string                 `json:"note"`
	Items     []batchItemFingerprint `json:"items"`
}

func (rt *mcpRuntime) createInbound(_ context.Context, _ *mcp.CallToolRequest, in inboundInput) (*mcp.CallToolResult, map[string]any, error) {
	requestID, fp, fail := mcpRequestKey("create_inbound", in.RequestID, inboundFingerprint{
		ProductID: strings.TrimSpace(in.ProductID), ProductCode: strings.TrimSpace(in.ProductCode), ProductQuery: strings.TrimSpace(in.ProductQuery),
		ShopID: strings.TrimSpace(in.ShopID), ShopQuery: strings.TrimSpace(in.ShopQuery), Quantity: in.Quantity, Note: strings.TrimSpace(in.Note),
	})
	if fail != nil {
		return fail, nil, nil
	}
	var audits []mcpAuditEvent
	var changes []services.InventoryChange
	payload, duplicate, err := services.CommitMCPRequest(rt.db, rt.user.ID, requestID, "create_inbound", fp, func(tx *gorm.DB) (map[string]any, error) {
		product, refs, err := services.ResolveProduct(tx, in.ProductID, in.ProductCode, in.ProductQuery)
		if err != nil {
			return nil, &mcpCatalogError{err: err, refs: refs}
		}
		var shopID *uuid.UUID
		var shop *models.Shop
		if strings.TrimSpace(in.ShopID) != "" || strings.TrimSpace(in.ShopQuery) != "" {
			resolved, shopRefs, shopErr := services.ResolveShop(tx, in.ShopID, in.ShopQuery)
			if shopErr != nil {
				return nil, &mcpCatalogError{err: shopErr, refs: shopRefs}
			}
			shop = &resolved
			shopID = &resolved.ID
		}
		change, err := services.InventoryService{DB: tx}.CreateInbound(services.InboundInput{
			ProductID: product.ID, ShopID: shopID, Quantity: in.Quantity, Note: in.Note, OperatorID: rt.user.ID,
		})
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
		metadata := map[string]string{"quantity": strconv.FormatInt(in.Quantity, 10)}
		if shopID != nil {
			metadata["shop_id"] = shopID.String()
		}
		if change.Movement.Reason != "" {
			metadata["note"] = change.Movement.Reason
		}
		if requestID != "" {
			metadata["request_id"] = requestID
		}
		audits = append(audits, mcpAuditEvent{Action: "inventory.inbound", ResourceType: "product", ResourceID: product.ID.String(), Metadata: metadata})
		return inboundPayload(change, shop, in.Verbose), nil
	})
	return rt.finishMCPWrite(payload, duplicate, err, audits, changes)
}

func (rt *mcpRuntime) createOutbound(_ context.Context, _ *mcp.CallToolRequest, in outboundInput) (*mcp.CallToolResult, map[string]any, error) {
	requestID, fp, fail := mcpRequestKey("create_sales_outbound", in.RequestID, outboundFingerprint{
		ProductID: strings.TrimSpace(in.ProductID), ProductCode: strings.TrimSpace(in.ProductCode), ProductQuery: strings.TrimSpace(in.ProductQuery),
		ShopID: strings.TrimSpace(in.ShopID), ShopQuery: strings.TrimSpace(in.ShopQuery), Quantity: in.Quantity, Note: strings.TrimSpace(in.Note),
	})
	if fail != nil {
		return fail, nil, nil
	}
	var audits []mcpAuditEvent
	var changes []services.InventoryChange
	payload, duplicate, err := services.CommitMCPRequest(rt.db, rt.user.ID, requestID, "create_sales_outbound", fp, func(tx *gorm.DB) (map[string]any, error) {
		product, refs, err := services.ResolveProduct(tx, in.ProductID, in.ProductCode, in.ProductQuery)
		if err != nil {
			return nil, &mcpCatalogError{err: err, refs: refs}
		}
		shop, shopRefs, err := services.ResolveShop(tx, in.ShopID, in.ShopQuery)
		if err != nil {
			return nil, &mcpCatalogError{err: err, refs: shopRefs}
		}
		change, err := services.InventoryService{DB: tx}.CreateSalesOutbound(services.OutboundInput{
			ProductID: product.ID, ShopID: shop.ID, Quantity: in.Quantity, Note: in.Note, OperatorID: rt.user.ID,
		})
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
		metadata := map[string]string{"quantity": strconv.FormatInt(in.Quantity, 10), "shop_id": shop.ID.String()}
		if change.Movement.Reason != "" {
			metadata["note"] = change.Movement.Reason
		}
		if requestID != "" {
			metadata["request_id"] = requestID
		}
		audits = append(audits, mcpAuditEvent{Action: "inventory.sales_outbound", ResourceType: "product", ResourceID: product.ID.String(), Metadata: metadata})
		return outboundPayload(change, shop, in.Verbose), nil
	})
	return rt.finishMCPWrite(payload, duplicate, err, audits, changes)
}

func (rt *mcpRuntime) createAdjustment(_ context.Context, _ *mcp.CallToolRequest, in adjustmentInput) (*mcp.CallToolResult, map[string]any, error) {
	requestID, fp, fail := mcpRequestKey("create_adjustment", in.RequestID, adjustmentFingerprint{
		ProductID: strings.TrimSpace(in.ProductID), ProductCode: strings.TrimSpace(in.ProductCode), ProductQuery: strings.TrimSpace(in.ProductQuery),
		QuantityDelta: in.QuantityDelta, Reason: strings.TrimSpace(in.Reason),
	})
	if fail != nil {
		return fail, nil, nil
	}
	var audits []mcpAuditEvent
	var changes []services.InventoryChange
	payload, duplicate, err := services.CommitMCPRequest(rt.db, rt.user.ID, requestID, "create_adjustment", fp, func(tx *gorm.DB) (map[string]any, error) {
		product, refs, err := services.ResolveProduct(tx, in.ProductID, in.ProductCode, in.ProductQuery)
		if err != nil {
			return nil, &mcpCatalogError{err: err, refs: refs}
		}
		change, err := services.InventoryService{DB: tx}.CreateAdjustment(services.AdjustmentInput{
			ProductID: product.ID, QuantityDelta: in.QuantityDelta, Reason: in.Reason, OperatorID: rt.user.ID,
		})
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
		metadata := map[string]string{
			"quantity_delta": strconv.FormatInt(in.QuantityDelta, 10),
			"reason":         in.Reason,
		}
		if requestID != "" {
			metadata["request_id"] = requestID
		}
		audits = append(audits, mcpAuditEvent{Action: "inventory.adjustment", ResourceType: "product", ResourceID: product.ID.String(), Metadata: metadata})
		return adjustmentPayload(change, in.QuantityDelta, in.Verbose), nil
	})
	return rt.finishMCPWrite(payload, duplicate, err, audits, changes)
}

func (rt *mcpRuntime) createOutboundBatch(_ context.Context, _ *mcp.CallToolRequest, in outboundBatchInput) (*mcp.CallToolResult, map[string]any, error) {
	if len(in.Items) == 0 {
		return toolFail("items 不能为空", map[string]any{"code": "VALIDATION"})
	}
	if len(in.Items) > 200 {
		return toolFail("items 最多 200 项", map[string]any{"code": "VALIDATION"})
	}
	requestID, fp, fail := mcpRequestKey("create_sales_outbound_batch", in.RequestID, batchRequestFingerprint(in))
	if fail != nil {
		return fail, nil, nil
	}
	if in.DryRun {
		payload, found, err := services.LookupMCPRequest(rt.db, rt.user.ID, requestID, "create_sales_outbound_batch", fp)
		if err != nil {
			return rt.finishMCPWrite(nil, false, err, nil, nil)
		}
		if found {
			payload["duplicate"] = true
			return toolOK(payload)
		}
		payload, err = rt.writeOutboundBatch(rt.db, in, requestID, nil, nil)
		return rt.finishMCPWrite(payload, false, err, nil, nil)
	}
	var audits []mcpAuditEvent
	var changes []services.InventoryChange
	payload, duplicate, err := services.CommitMCPRequest(rt.db, rt.user.ID, requestID, "create_sales_outbound_batch", fp, func(tx *gorm.DB) (map[string]any, error) {
		return rt.writeOutboundBatch(tx, in, requestID, &audits, &changes)
	})
	return rt.finishMCPWrite(payload, duplicate, err, audits, changes)
}

func (rt *mcpRuntime) writeOutboundBatch(db *gorm.DB, in outboundBatchInput, requestID string, audits *[]mcpAuditEvent, changes *[]services.InventoryChange) (map[string]any, error) {
	shop, refs, err := services.ResolveShop(db, in.ShopID, in.ShopQuery)
	if err != nil {
		return nil, &mcpCatalogError{err: err, refs: refs}
	}
	items, failures := resolveBatchItems(db, in.Items)
	if len(failures) > 0 {
		return nil, &services.OutboundBatchRejectedError{Items: failures}
	}
	result, err := services.InventoryService{DB: db}.CreateSalesOutboundBatch(services.OutboundBatchInput{
		ShopID: shop.ID, Note: in.Note, OperatorID: rt.user.ID, Items: items,
	}, in.DryRun)
	if err != nil {
		return nil, err
	}
	if !in.DryRun && audits != nil && changes != nil {
		for _, line := range result.Lines {
			metadata := map[string]string{
				"quantity": strconv.FormatInt(line.Quantity, 10),
				"shop_id":  shop.ID.String(),
				"batch":    "true",
			}
			if line.Change.Movement.Reason != "" {
				metadata["note"] = line.Change.Movement.Reason
			}
			if requestID != "" {
				metadata["request_id"] = requestID
			}
			*audits = append(*audits, mcpAuditEvent{
				Action: "inventory.sales_outbound", ResourceType: "product", ResourceID: line.Product.ID.String(), Metadata: metadata,
			})
			*changes = append(*changes, line.Change)
		}
	}
	return batchSuccessPayload(shop, result, in.Verbose, in.DryRun), nil
}

func resolveBatchItems(db *gorm.DB, inputs []outboundBatchItemInput) ([]services.OutboundBatchItem, []services.OutboundBatchItemError) {
	items := make([]services.OutboundBatchItem, 0, len(inputs))
	var failures []services.OutboundBatchItemError
	seen := map[string]struct{}{}
	for _, input := range inputs {
		code := strings.TrimSpace(input.ProductCode)
		if input.Quantity <= 0 {
			failures = append(failures, services.OutboundBatchItemError{
				ProductID: strings.TrimSpace(input.ProductID), ProductCode: code, Message: "数量必须大于 0", Requested: input.Quantity,
			})
			continue
		}
		product, refs, err := services.ResolveProduct(db, input.ProductID, code, "")
		if err != nil {
			message := "商品不存在"
			if errors.Is(err, services.ErrCatalogAmbiguous) {
				message = "商品不明确"
			}
			failures = append(failures, services.OutboundBatchItemError{
				ProductID: strings.TrimSpace(input.ProductID), ProductCode: code, Message: message, Requested: input.Quantity, Candidates: refs,
			})
			continue
		}
		if _, ok := seen[product.ID.String()]; ok {
			failures = append(failures, services.OutboundBatchItemError{
				ProductID: product.ID.String(), ProductCode: product.Code, Message: "商品重复", Requested: input.Quantity,
			})
			continue
		}
		seen[product.ID.String()] = struct{}{}
		items = append(items, services.OutboundBatchItem{ProductID: product.ID, ProductCode: product.Code, Quantity: input.Quantity})
	}
	return items, failures
}

func (rt *mcpRuntime) finishMCPWrite(payload map[string]any, duplicate bool, err error, audits []mcpAuditEvent, changes []services.InventoryChange) (*mcp.CallToolResult, map[string]any, error) {
	if err != nil {
		var catalogErr *mcpCatalogError
		if errors.As(err, &catalogErr) {
			return catalogFail(catalogErr.err, catalogErr.refs)
		}
		if errors.Is(err, services.ErrMCPRequestConflict) {
			return toolFail("request_id 已用于不同参数，未记账", map[string]any{"code": "IDEMPOTENCY_CONFLICT"})
		}
		if errors.Is(err, services.ErrMCPRequestIDInvalid) {
			return toolFail("request_id 须为 1 到 128 个字符", map[string]any{"code": "VALIDATION"})
		}
		var rejected *services.OutboundBatchRejectedError
		if errors.As(err, &rejected) {
			return toolOK(batchRejectedPayload(rejected))
		}
		if res, out, callErr, failed := stockToolError(err); failed {
			return res, out, callErr
		}
		return toolFail(err.Error(), nil)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if duplicate {
		payload["duplicate"] = true
		return toolOK(payload)
	}
	for _, audit := range audits {
		recordAudit(rt.c, rt.db, audit.Action, audit.ResourceType, audit.ResourceID, audit.Metadata)
	}
	if rt.lark != nil {
		for _, change := range changes {
			rt.lark.Enqueue(change)
		}
	}
	return toolOK(payload)
}

func mcpRequestKey(tool string, requestID string, payload any) (string, string, *mcp.CallToolResult) {
	id, err := services.NormalizeMCPRequestID(requestID)
	if err != nil {
		result, _, _ := toolFail("request_id 须为 1 到 128 个字符", map[string]any{"code": "VALIDATION"})
		return "", "", result
	}
	fingerprint, err := services.MCPRequestFingerprint(tool, payload)
	if err != nil {
		result, _, _ := toolFail("failed to fingerprint request", nil)
		return "", "", result
	}
	return id, fingerprint, nil
}

func inboundPayload(change services.InventoryChange, shop *models.Shop, verbose bool) map[string]any {
	if verbose {
		return map[string]any{"ok": true, "product": change.Product, "quantity_after": change.QuantityAfter}
	}
	payload := map[string]any{
		"ok": true, "product_code": change.Product.Code, "product_name": change.Product.Name, "quantity_after": change.QuantityAfter,
	}
	if shop != nil {
		payload["shop_name"] = shop.Name
	}
	return payload
}

func outboundPayload(change services.InventoryChange, shop models.Shop, verbose bool) map[string]any {
	if verbose {
		return map[string]any{"ok": true, "product": change.Product, "shop": shop, "quantity_after": change.QuantityAfter}
	}
	return map[string]any{
		"ok": true, "product_code": change.Product.Code, "product_name": change.Product.Name, "shop_name": shop.Name,
		"quantity_after": change.QuantityAfter, "cost_amount_cents": change.Movement.CostAmountCents,
	}
}

func adjustmentPayload(change services.InventoryChange, delta int64, verbose bool) map[string]any {
	if verbose {
		return map[string]any{"ok": true, "product": change.Product, "quantity_after": change.QuantityAfter}
	}
	return map[string]any{
		"ok": true, "product_code": change.Product.Code, "product_name": change.Product.Name,
		"quantity_delta": delta, "quantity_after": change.QuantityAfter,
	}
}

func batchSuccessPayload(shop models.Shop, result services.OutboundBatchResult, verbose bool, dryRun bool) map[string]any {
	items := make([]map[string]any, 0, len(result.Lines))
	for _, line := range result.Lines {
		item := map[string]any{
			"product_code": line.ProductCode, "quantity": line.Quantity, "quantity_after": line.QuantityAfter, "cost_amount_cents": line.CostAmountCents,
		}
		if verbose {
			item["product"] = line.Product
		}
		items = append(items, item)
	}
	payload := map[string]any{
		"ok": true, "shop_name": shop.Name, "items": items, "total_quantity": result.TotalQuantity, "total_cost_cents": result.TotalCostCents,
	}
	if dryRun {
		payload["dry_run"] = true
	}
	if verbose {
		payload["shop"] = shop
	}
	return payload
}

func batchRejectedPayload(rejected *services.OutboundBatchRejectedError) map[string]any {
	items := make([]map[string]any, 0, len(rejected.Items))
	for _, item := range rejected.Items {
		entry := map[string]any{"product_code": item.ProductCode, "error": item.Message, "requested": item.Requested}
		if item.ProductID != "" && item.ProductID != "00000000-0000-0000-0000-000000000000" {
			entry["product_id"] = item.ProductID
		}
		if item.Available != nil {
			entry["available"] = *item.Available
		}
		if len(item.Candidates) > 0 {
			entry["candidates"] = item.Candidates
		}
		items = append(items, entry)
	}
	return map[string]any{"ok": false, "items": items}
}

func batchRequestFingerprint(in outboundBatchInput) batchFingerprint {
	items := make([]batchItemFingerprint, 0, len(in.Items))
	for _, item := range in.Items {
		items = append(items, batchItemFingerprint{
			ProductID: strings.TrimSpace(item.ProductID), ProductCode: strings.TrimSpace(item.ProductCode), Quantity: item.Quantity,
		})
	}
	return batchFingerprint{
		ShopID: strings.TrimSpace(in.ShopID), ShopQuery: strings.TrimSpace(in.ShopQuery), Note: strings.TrimSpace(in.Note), Items: items,
	}
}
