package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InventoryHandler struct {
	DB *gorm.DB
}

type inboundRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	Quantity  int64  `json:"quantity" binding:"required,gt=0"`
	UnitCents *int64 `json:"unit_cents" binding:"required,gte=0"`
}

type outboundRequest struct {
	ProductID     string `json:"product_id" binding:"required"`
	ShopID        string `json:"shop_id" binding:"required"`
	Quantity      int64  `json:"quantity" binding:"required,gt=0"`
	SaleUnitCents *int64 `json:"sale_unit_cents" binding:"required,gte=0"`
}

type adjustmentRequest struct {
	ProductID     string `json:"product_id" binding:"required"`
	QuantityDelta int64  `json:"quantity_delta" binding:"required"`
	Reason        string `json:"reason" binding:"required,min=1,max=500"`
}

func (h InventoryHandler) ListCurrent(c *gin.Context) {
	var items []models.InventorySnapshot
	if err := h.DB.Preload("Product").Order("updated_at desc").Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list inventory")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
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
	err := services.InventoryService{DB: h.DB}.CreateInbound(services.InboundInput{
		ProductID: productID, Quantity: req.Quantity, UnitCents: *req.UnitCents, OperatorID: currentUserID(c),
	})
	if writeStockResult(c, err) {
		recordAudit(c, h.DB, "inventory.inbound", "product", productID.String(), map[string]string{"quantity": strconv.FormatInt(req.Quantity, 10)})
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
	err := services.InventoryService{DB: h.DB}.CreateSalesOutbound(services.OutboundInput{
		ProductID: productID, ShopID: shopID, Quantity: req.Quantity, SaleUnitCents: *req.SaleUnitCents, OperatorID: currentUserID(c),
	})
	if writeStockResult(c, err) {
		recordAudit(c, h.DB, "inventory.sales_outbound", "product", productID.String(), map[string]string{"quantity": strconv.FormatInt(req.Quantity, 10), "shop_id": shopID.String()})
	}
}

func (r *outboundRequest) UnmarshalJSON(data []byte) error {
	var payload struct {
		ProductID          string `json:"product_id"`
		ShopID             string `json:"shop_id"`
		Quantity           int64  `json:"quantity"`
		SaleUnitCents      *int64 `json:"sale_unit_cents"`
		CamelSaleUnitCents *int64 `json:"saleUnitCents"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	r.ProductID = payload.ProductID
	r.ShopID = payload.ShopID
	r.Quantity = payload.Quantity
	r.SaleUnitCents = payload.SaleUnitCents
	if r.SaleUnitCents == nil {
		r.SaleUnitCents = payload.CamelSaleUnitCents
	}
	return nil
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
	err := services.InventoryService{DB: h.DB}.CreateAdjustment(services.AdjustmentInput{
		ProductID: productID, QuantityDelta: req.QuantityDelta, Reason: req.Reason, OperatorID: currentUserID(c),
	})
	if writeStockResult(c, err) {
		recordAudit(c, h.DB, "inventory.adjustment", "product", productID.String(), map[string]string{"quantity_delta": strconv.FormatInt(req.QuantityDelta, 10), "reason": req.Reason})
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
	writeError(c, http.StatusBadRequest, "STOCK_OPERATION_FAILED", err.Error())
	return false
}
