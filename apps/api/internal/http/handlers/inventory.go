package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InventoryHandler struct {
	DB   *gorm.DB
	Cfg  config.Config
	Lark *services.LarkNotifier
}

type inboundRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	ShopID    string `json:"shop_id"`
	Quantity  int64  `json:"quantity" binding:"required,gt=0"`
}

type outboundRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	ShopID    string `json:"shop_id" binding:"required"`
	Quantity  int64  `json:"quantity" binding:"required,gt=0"`
}

type adjustmentRequest struct {
	ProductID     string `json:"product_id" binding:"required"`
	QuantityDelta int64  `json:"quantity_delta" binding:"required"`
	Reason        string `json:"reason" binding:"required,min=1,max=500"`
}

type inventoryResponse struct {
	ProductID           uuid.UUID
	Product             models.Product
	Quantity            int64
	InventoryValueCents int64
}

func (h InventoryHandler) ListCurrent(c *gin.Context) {
	var snapshots []models.InventorySnapshot
	base := h.activeInventoryQuery(c.Query("low_stock") == "true", c.Query("q"))
	query, meta, err := paginate(c, base)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to count inventory")
		return
	}
	if err := query.
		Preload("Product").
		Find(&snapshots).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list inventory")
		return
	}
	items := make([]inventoryResponse, 0, len(snapshots))
	for _, snapshot := range snapshots {
		value, err := services.CurrentInventoryValue(snapshot)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to calculate inventory value")
			return
		}
		items = append(items, inventoryResponse{
			ProductID: snapshot.ProductID, Product: snapshot.Product,
			Quantity: snapshot.Quantity, InventoryValueCents: value,
		})
	}
	writePage(c, items, meta)
}

func (h InventoryHandler) ExportCurrent(c *gin.Context) {
	var items []models.InventorySnapshot
	if err := h.activeInventoryQuery(c.Query("low_stock") == "true", c.Query("q")).
		Preload("Product").
		Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to export inventory")
		return
	}
	data, err := services.BuildInventoryWorkbook(items, h.Cfg.UploadDir)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to build inventory workbook")
		return
	}
	filename := fmt.Sprintf("inventory-%s.xlsx", time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (h InventoryHandler) activeInventoryQuery(lowStock bool, keyword string) *gorm.DB {
	base := h.DB.Model(&models.InventorySnapshot{}).
		Joins("JOIN products ON products.id = inventory_snapshots.product_id").
		Where("products.archived_at IS NULL")
	if lowStock {
		base = base.Where("products.low_stock_threshold > 0 AND inventory_snapshots.quantity <= products.low_stock_threshold")
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		base = base.Where("products.name ILIKE ? OR products.code ILIKE ?", like, like)
	}
	return base.
		Order("products.name ASC").
		Order("products.code ASC").
		Order("inventory_snapshots.product_id ASC")
}

func (h InventoryHandler) CreateInbound(c *gin.Context) {
	var req inboundRequest
	if !bindJSON(c, &req) {
		return
	}
	productID, ok := parseUUID(c, req.ProductID, "product_id")
	if !ok {
		return
	}
	var shopID *uuid.UUID
	if req.ShopID != "" {
		parsedShopID, ok := parseUUID(c, req.ShopID, "shop_id")
		if !ok {
			return
		}
		shopID = &parsedShopID
	}
	change, err := services.InventoryService{DB: h.DB}.CreateInbound(services.InboundInput{
		ProductID: productID, ShopID: shopID, Quantity: req.Quantity, OperatorID: currentUserID(c),
	})
	if writeStockResult(c, err) {
		metadata := map[string]string{"quantity": strconv.FormatInt(req.Quantity, 10)}
		if shopID != nil {
			metadata["shop_id"] = shopID.String()
		}
		recordAudit(c, h.DB, "inventory.inbound", "product", productID.String(), metadata)
		h.Lark.Enqueue(change)
	}
}

func (h InventoryHandler) CreateSalesOutbound(c *gin.Context) {
	var req outboundRequest
	if !bindJSON(c, &req) {
		return
	}
	productID, ok := parseUUID(c, req.ProductID, "product_id")
	if !ok {
		return
	}
	shopID, ok := parseUUID(c, req.ShopID, "shop_id")
	if !ok {
		return
	}
	change, err := services.InventoryService{DB: h.DB}.CreateSalesOutbound(services.OutboundInput{
		ProductID: productID, ShopID: shopID, Quantity: req.Quantity, OperatorID: currentUserID(c),
	})
	if writeStockResult(c, err) {
		recordAudit(c, h.DB, "inventory.sales_outbound", "product", productID.String(), map[string]string{"quantity": strconv.FormatInt(req.Quantity, 10), "shop_id": shopID.String()})
		h.Lark.Enqueue(change)
	}
}

func (h InventoryHandler) CreateAdjustment(c *gin.Context) {
	var req adjustmentRequest
	if !bindJSON(c, &req) {
		return
	}
	productID, ok := parseUUID(c, req.ProductID, "product_id")
	if !ok {
		return
	}
	change, err := services.InventoryService{DB: h.DB}.CreateAdjustment(services.AdjustmentInput{
		ProductID: productID, QuantityDelta: req.QuantityDelta, Reason: req.Reason, OperatorID: currentUserID(c),
	})
	if writeStockResult(c, err) {
		recordAudit(c, h.DB, "inventory.adjustment", "product", productID.String(), map[string]string{"quantity_delta": strconv.FormatInt(req.QuantityDelta, 10), "reason": req.Reason})
		h.Lark.Enqueue(change)
	}
}

func parseUUID(c *gin.Context, raw string, field string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION", "invalid "+field)
		return uuid.Nil, false
	}
	return id, true
}

func writeStockResult(c *gin.Context, err error) bool {
	if err == nil {
		c.JSON(http.StatusCreated, gin.H{"ok": true})
		return true
	}
	if errors.Is(err, services.ErrInsufficientStock) {
		writeError(c, http.StatusConflict, "INSUFFICIENT_STOCK", err.Error())
		return false
	}
	if errors.Is(err, services.ErrProductArchived) {
		writeError(c, http.StatusConflict, "PRODUCT_ARCHIVED", "商品已归档，不能进行库存操作")
		return false
	}
	writeError(c, http.StatusBadRequest, "STOCK_OPERATION_FAILED", err.Error())
	return false
}
