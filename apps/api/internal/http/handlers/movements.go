package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MovementHandler struct {
	DB *gorm.DB
}

type movementResponse struct {
	ID                  uuid.UUID
	Type                models.MovementType
	ProductID           uuid.UUID
	Product             models.Product
	ShopID              *uuid.UUID
	Shop                *models.Shop
	QuantityDelta       int64
	PurchaseAmountCents int64
	CostAmountCents     int64
	Reason              string
	OperatorID          uuid.UUID
	Operator            userResponse
	Revision            int64
	LastEditedByID      *uuid.UUID
	LastEditedBy        *userResponse
	CreatedAt           time.Time
	UpdatedAt           time.Time
	IsLatest            bool
}

type movementUpdateRequest struct {
	ExpectedRevision *int64  `json:"expected_revision"`
	Quantity         *int64  `json:"quantity"`
	QuantityDelta    *int64  `json:"quantity_delta"`
	ShopID           *string `json:"shop_id"`
	Note             string  `json:"note"`
	ChangeReason     string  `json:"change_reason"`
}

func (h MovementHandler) List(c *gin.Context) {
	var movements []models.StockMovement
	query := h.DB.Model(&models.StockMovement{})
	if movementType := c.Query("type"); movementType != "" {
		query = query.Where("type = ?", movementType)
	}
	if productID := c.Query("product_id"); productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	if shopID := c.Query("shop_id"); shopID != "" {
		query = query.Where("shop_id = ?", shopID)
	}
	if from, ok := movementQueryDate(c, "from"); ok {
		query = query.Where("created_at >= ?", from)
	}
	if to, ok := movementQueryDate(c, "to"); ok {
		query = query.Where("created_at < ?", to.AddDate(0, 0, 1))
	}
	query, meta, err := paginate(c, query)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to count movements")
		return
	}
	if err := query.Preload("Product").Preload("Shop").Preload("Operator").Preload("LastEditedBy").
		Order("created_at desc").Order("id desc").Find(&movements).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list movements")
		return
	}

	// ponytail: one lookup per visible product keeps this dialect-neutral; use a window query if pages become large.
	latestByProduct := make(map[uuid.UUID]uuid.UUID)
	items := make([]movementResponse, 0, len(movements))
	for _, movement := range movements {
		latestID, ok := latestByProduct[movement.ProductID]
		if !ok {
			var latest models.StockMovement
			if err := h.DB.Select("id").Where("product_id = ?", movement.ProductID).
				Order("created_at desc").Order("id desc").Take(&latest).Error; err != nil {
				writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to determine latest movement")
				return
			}
			latestID = latest.ID
			latestByProduct[movement.ProductID] = latestID
		}
		priced, err := services.CurrentPriceMovement(movement)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to calculate movement amounts")
			return
		}
		priced.IsLatest = movement.ID == latestID
		items = append(items, newMovementResponse(priced))
	}
	writePage(c, items, meta)
}

func (h MovementHandler) PreviewUpdate(c *gin.Context) {
	input, ok := movementUpdateInput(c)
	if !ok {
		return
	}
	result, err := (services.InventoryService{DB: h.DB}).PreviewMovementUpdate(input)
	if err != nil {
		writeMovementUpdateError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h MovementHandler) Update(c *gin.Context) {
	input, ok := movementUpdateInput(c)
	if !ok {
		return
	}
	input.EditorID = currentUserID(c)
	input.IPAddress = c.ClientIP()
	movement, result, err := (services.InventoryService{DB: h.DB}).UpdateMovement(input)
	if err != nil {
		writeMovementUpdateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": newMovementResponse(movement), "impact": result.Impact})
}

func movementUpdateInput(c *gin.Context) (services.MovementUpdateInput, bool) {
	movementID, ok := parseUUID(c, c.Param("id"), "id")
	if !ok {
		return services.MovementUpdateInput{}, false
	}
	var req movementUpdateRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION", err.Error())
		return services.MovementUpdateInput{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "VALIDATION", "request must contain one JSON object")
		return services.MovementUpdateInput{}, false
	}
	if req.ExpectedRevision == nil {
		writeError(c, http.StatusBadRequest, "VALIDATION", "expected_revision is required")
		return services.MovementUpdateInput{}, false
	}
	var shopID *uuid.UUID
	if req.ShopID != nil && strings.TrimSpace(*req.ShopID) != "" {
		parsed, ok := parseUUID(c, *req.ShopID, "shop_id")
		if !ok {
			return services.MovementUpdateInput{}, false
		}
		shopID = &parsed
	}
	return services.MovementUpdateInput{
		MovementID: movementID, ExpectedRevision: *req.ExpectedRevision,
		Quantity: req.Quantity, QuantityDelta: req.QuantityDelta,
		ShopID: shopID, Note: req.Note, ChangeReason: req.ChangeReason,
	}, true
}

func movementQueryDate(c *gin.Context, key string) (time.Time, bool) {
	raw := c.Query(key)
	if raw == "" {
		return time.Time{}, false
	}
	value, err := time.ParseInLocation("2006-01-02", raw, time.FixedZone("Asia/Shanghai", 8*60*60))
	if err != nil {
		return time.Time{}, false
	}
	return value.UTC(), true
}

func writeMovementUpdateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrMovementValidation):
		message := strings.TrimPrefix(err.Error(), services.ErrMovementValidation.Error()+": ")
		writeError(c, http.StatusBadRequest, "VALIDATION", message)
	case errors.Is(err, services.ErrMovementNotFound):
		writeError(c, http.StatusNotFound, "MOVEMENT_NOT_FOUND", "流水不存在")
	case errors.Is(err, services.ErrMovementStale):
		writeError(c, http.StatusConflict, "MOVEMENT_STALE", "流水已变化，请刷新后重试")
	case errors.Is(err, services.ErrInsufficientStock):
		writeError(c, http.StatusConflict, "INSUFFICIENT_STOCK", "修正后库存不足")
	case errors.Is(err, services.ErrProductArchived):
		writeError(c, http.StatusConflict, "PRODUCT_ARCHIVED", "归档商品只允许修改备注或店铺")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "更新流水失败")
	}
}

func newMovementResponse(movement models.StockMovement) movementResponse {
	item := movementResponse{
		ID: movement.ID, Type: movement.Type, ProductID: movement.ProductID, Product: movement.Product,
		ShopID: movement.ShopID, Shop: movement.Shop, QuantityDelta: movement.QuantityDelta,
		PurchaseAmountCents: movement.PurchaseAmountCents,
		CostAmountCents:     movement.CostAmountCents, Reason: movement.Reason,
		OperatorID: movement.OperatorID,
		Operator:   userResponse{ID: movement.Operator.ID, Name: movement.Operator.Name, Email: movement.Operator.Email, Role: movement.Operator.Role},
		Revision:   movement.Revision, LastEditedByID: movement.LastEditedByID,
		CreatedAt: movement.CreatedAt, UpdatedAt: movement.UpdatedAt, IsLatest: movement.IsLatest,
	}
	if movement.LastEditedBy != nil {
		item.LastEditedBy = &userResponse{
			ID: movement.LastEditedBy.ID, Name: movement.LastEditedBy.Name,
			Email: movement.LastEditedBy.Email, Role: movement.LastEditedBy.Role,
		}
	}
	return item
}
