package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrProductArchived    = errors.New("product is archived")
	ErrMovementNotFound   = errors.New("movement not found")
	ErrMovementStale      = errors.New("movement is stale")
	ErrMovementValidation = errors.New("invalid movement update")
	ErrMovementState      = errors.New("invalid movement state")
)

type InventoryService struct {
	DB *gorm.DB
}

type InboundInput struct {
	ProductID  uuid.UUID
	ShopID     *uuid.UUID
	Quantity   int64
	Note       string
	OperatorID uuid.UUID
}

type OutboundInput struct {
	ProductID  uuid.UUID
	ShopID     uuid.UUID
	Quantity   int64
	Note       string
	OperatorID uuid.UUID
}

type AdjustmentInput struct {
	ProductID     uuid.UUID
	QuantityDelta int64
	Reason        string
	OperatorID    uuid.UUID
}

type InventoryChange struct {
	Product        models.Product
	Movement       models.StockMovement
	QuantityBefore int64
	QuantityAfter  int64
}

type MovementUpdateInput struct {
	MovementID       uuid.UUID
	ExpectedRevision int64
	Quantity         *int64
	QuantityDelta    *int64
	ShopID           *uuid.UUID
	Note             string
	ChangeReason     string
	EditorID         uuid.UUID
	IPAddress        string
}

type MovementRevisionValues struct {
	ID                  uuid.UUID           `json:"id"`
	Type                models.MovementType `json:"type"`
	ProductID           uuid.UUID           `json:"product_id"`
	OperatorID          uuid.UUID           `json:"operator_id"`
	CreatedAt           time.Time           `json:"created_at"`
	QuantityDelta       int64               `json:"quantity_delta"`
	ShopID              *uuid.UUID          `json:"shop_id"`
	Note                string              `json:"note"`
	PurchaseAmountCents int64               `json:"purchase_amount_cents"`
	CostAmountCents     int64               `json:"cost_amount_cents"`
}

type MovementImpact struct {
	CurrentQuantity            int64 `json:"current_quantity"`
	ResultQuantity             int64 `json:"result_quantity"`
	QuantityChange             int64 `json:"quantity_change"`
	CurrentInventoryValueCents int64 `json:"current_inventory_value_cents"`
	ResultInventoryValueCents  int64 `json:"result_inventory_value_cents"`
	InventoryValueDeltaCents   int64 `json:"inventory_value_delta_cents"`
	PurchaseAmountDeltaCents   int64 `json:"purchase_amount_delta_cents"`
	CostDeltaCents             int64 `json:"cost_delta_cents"`
}

type MovementEditResult struct {
	Before           MovementRevisionValues `json:"before"`
	After            MovementRevisionValues `json:"after"`
	Impact           MovementImpact         `json:"impact"`
	ExpectedRevision int64                  `json:"expected_revision"`
}

func CurrentInventoryValue(snapshot models.InventorySnapshot) (int64, error) {
	return checkedMul(snapshot.Quantity, snapshot.Product.DefaultPurchaseCents)
}

func CurrentPriceMovement(movement models.StockMovement) (models.StockMovement, error) {
	return priceMovement(movement, movement.Product)
}

func priceMovement(movement models.StockMovement, product models.Product) (models.StockMovement, error) {
	movement.PurchaseUnitCents = nil
	movement.CostUnitCents = product.DefaultPurchaseCents
	movement.PurchaseAmountCents = 0
	movement.CostAmountCents = 0

	switch movement.Type {
	case models.MovementTypeInbound:
		if movement.QuantityDelta <= 0 {
			return models.StockMovement{}, ErrMovementState
		}
		amount, err := checkedMul(movement.QuantityDelta, product.DefaultPurchaseCents)
		if err != nil {
			return models.StockMovement{}, err
		}
		purchase := product.DefaultPurchaseCents
		movement.PurchaseUnitCents = &purchase
		movement.PurchaseAmountCents = amount
	case models.MovementTypeSalesOutbound:
		if movement.QuantityDelta >= 0 {
			return models.StockMovement{}, ErrMovementState
		}
		quantity, err := checkedSub(0, movement.QuantityDelta)
		if err != nil {
			return models.StockMovement{}, err
		}
		cost, err := checkedMul(quantity, product.DefaultPurchaseCents)
		if err != nil {
			return models.StockMovement{}, err
		}
		movement.CostAmountCents = cost
	case models.MovementTypeAdjustment:
		if movement.QuantityDelta == 0 {
			return models.StockMovement{}, ErrMovementState
		}
		amount, err := checkedMul(movement.QuantityDelta, product.DefaultPurchaseCents)
		if err != nil {
			return models.StockMovement{}, err
		}
		movement.CostAmountCents = amount
	default:
		return models.StockMovement{}, ErrMovementState
	}
	return movement, nil
}

func repriceSnapshot(snapshot *models.InventorySnapshot, purchaseCents int64) error {
	if snapshot.Quantity < 0 {
		return ErrMovementState
	}
	value, err := checkedMul(snapshot.Quantity, purchaseCents)
	if err != nil {
		return err
	}
	snapshot.MovingAverageCostCents = purchaseCents
	if snapshot.Quantity == 0 {
		snapshot.MovingAverageCostCents = 0
	}
	snapshot.InventoryValueCents = value
	return nil
}

func validateOutbound(currentQty int64, outboundQty int64) error {
	if outboundQty <= 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}
	if currentQty < outboundQty {
		return ErrInsufficientStock
	}
	return nil
}

func validateAdjustment(delta int64, reason string) error {
	if delta == 0 {
		return fmt.Errorf("adjustment quantity cannot be zero")
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("adjustment reason is required")
	}
	return nil
}

func normalizeOptionalNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 500 {
		return "", fmt.Errorf("备注不能超过 500 字")
	}
	return note, nil
}

func applyInbound(snapshot *models.InventorySnapshot, quantity int64, purchaseCents int64) (models.StockMovement, error) {
	if quantity <= 0 {
		return models.StockMovement{}, fmt.Errorf("quantity must be greater than zero")
	}
	purchaseAmount, err := checkedMul(quantity, purchaseCents)
	if err != nil {
		return models.StockMovement{}, err
	}
	quantityAfter, err := checkedAdd(snapshot.Quantity, quantity)
	if err != nil {
		return models.StockMovement{}, err
	}
	snapshot.Quantity = quantityAfter
	if err := repriceSnapshot(snapshot, purchaseCents); err != nil {
		return models.StockMovement{}, err
	}
	unit := purchaseCents
	return models.StockMovement{
		Type:                models.MovementTypeInbound,
		QuantityDelta:       quantity,
		PurchaseUnitCents:   &unit,
		CostUnitCents:       purchaseCents,
		PurchaseAmountCents: purchaseAmount,
	}, nil
}

func applySalesOutbound(snapshot *models.InventorySnapshot, quantity int64, purchaseCents int64) (models.StockMovement, error) {
	if err := validateOutbound(snapshot.Quantity, quantity); err != nil {
		return models.StockMovement{}, err
	}
	costAmount, err := checkedMul(quantity, purchaseCents)
	if err != nil {
		return models.StockMovement{}, err
	}
	snapshot.Quantity -= quantity
	if err := repriceSnapshot(snapshot, purchaseCents); err != nil {
		return models.StockMovement{}, err
	}
	return models.StockMovement{
		Type:            models.MovementTypeSalesOutbound,
		QuantityDelta:   -quantity,
		CostUnitCents:   purchaseCents,
		CostAmountCents: costAmount,
	}, nil
}

func applyAdjustment(snapshot *models.InventorySnapshot, quantityDelta int64, reason string, purchaseCents int64) (models.StockMovement, error) {
	if err := validateAdjustment(quantityDelta, reason); err != nil {
		return models.StockMovement{}, err
	}
	quantityAfter, err := checkedAdd(snapshot.Quantity, quantityDelta)
	if err != nil {
		return models.StockMovement{}, err
	}
	if quantityAfter < 0 {
		return models.StockMovement{}, ErrInsufficientStock
	}
	costAmount, err := checkedMul(quantityDelta, purchaseCents)
	if err != nil {
		return models.StockMovement{}, err
	}
	snapshot.Quantity = quantityAfter
	if err := repriceSnapshot(snapshot, purchaseCents); err != nil {
		return models.StockMovement{}, err
	}
	return models.StockMovement{
		Type:            models.MovementTypeAdjustment,
		QuantityDelta:   quantityDelta,
		CostUnitCents:   purchaseCents,
		CostAmountCents: costAmount,
		Reason:          reason,
	}, nil
}

func (s InventoryService) CreateInbound(input InboundInput) (InventoryChange, error) {
	note, err := normalizeOptionalNote(input.Note)
	if err != nil {
		return InventoryChange{}, err
	}
	var change InventoryChange
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		product, err := lockActiveProduct(tx, input.ProductID)
		if err != nil {
			return err
		}
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		quantityBefore := snapshot.Quantity
		movement, err := applyInbound(&snapshot, input.Quantity, product.DefaultPurchaseCents)
		if err != nil {
			return err
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		movement.ProductID = input.ProductID
		movement.ShopID = input.ShopID
		movement.Reason = note
		movement.OperatorID = input.OperatorID
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}
		change = InventoryChange{Product: product, Movement: movement, QuantityBefore: quantityBefore, QuantityAfter: snapshot.Quantity}
		return nil
	})
	if err != nil {
		return InventoryChange{}, err
	}
	return change, nil
}

func (s InventoryService) CreateSalesOutbound(input OutboundInput) (InventoryChange, error) {
	note, err := normalizeOptionalNote(input.Note)
	if err != nil {
		return InventoryChange{}, err
	}
	var change InventoryChange
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		product, err := lockActiveProduct(tx, input.ProductID)
		if err != nil {
			return err
		}
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		quantityBefore := snapshot.Quantity
		movement, err := applySalesOutbound(&snapshot, input.Quantity, product.DefaultPurchaseCents)
		if err != nil {
			return err
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		movement.ProductID = input.ProductID
		movement.ShopID = &input.ShopID
		movement.Reason = note
		movement.OperatorID = input.OperatorID
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}
		change = InventoryChange{Product: product, Movement: movement, QuantityBefore: quantityBefore, QuantityAfter: snapshot.Quantity}
		return nil
	})
	if err != nil {
		return InventoryChange{}, err
	}
	return change, nil
}

func (s InventoryService) CreateAdjustment(input AdjustmentInput) (InventoryChange, error) {
	var change InventoryChange
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		product, err := lockActiveProduct(tx, input.ProductID)
		if err != nil {
			return err
		}
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		quantityBefore := snapshot.Quantity
		movement, err := applyAdjustment(&snapshot, input.QuantityDelta, input.Reason, product.DefaultPurchaseCents)
		if err != nil {
			return err
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		movement.ProductID = input.ProductID
		movement.OperatorID = input.OperatorID
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}
		change = InventoryChange{Product: product, Movement: movement, QuantityBefore: quantityBefore, QuantityAfter: snapshot.Quantity}
		return nil
	})
	if err != nil {
		return InventoryChange{}, err
	}
	return change, nil
}

func (s InventoryService) PreviewMovementUpdate(input MovementUpdateInput) (MovementEditResult, error) {
	if err := validateMovementUpdateInput(input, false); err != nil {
		return MovementEditResult{}, err
	}
	movement, err := loadMovementForEdit(s.DB, input.MovementID)
	if err != nil {
		return MovementEditResult{}, err
	}
	latest, err := latestMovement(s.DB, movement.ProductID)
	if err != nil {
		return MovementEditResult{}, fmt.Errorf("load latest movement: %w", err)
	}
	if latest.ID != movement.ID || movement.Revision != input.ExpectedRevision {
		return MovementEditResult{}, ErrMovementStale
	}
	var snapshot models.InventorySnapshot
	if err := s.DB.First(&snapshot, "product_id = ?", movement.ProductID).Error; err != nil {
		return MovementEditResult{}, fmt.Errorf("load inventory snapshot: %w", err)
	}
	_, result, _, err := calculateMovementUpdate(snapshot, movement, movement.Product, input)
	return result, err
}

func (s InventoryService) UpdateMovement(input MovementUpdateInput) (models.StockMovement, MovementEditResult, error) {
	if err := validateMovementUpdateInput(input, true); err != nil {
		return models.StockMovement{}, MovementEditResult{}, err
	}
	var stub models.StockMovement
	if err := s.DB.Select("id", "product_id").Take(&stub, "id = ?", input.MovementID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.StockMovement{}, MovementEditResult{}, ErrMovementNotFound
		}
		return models.StockMovement{}, MovementEditResult{}, fmt.Errorf("load movement: %w", err)
	}

	var editResult MovementEditResult
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		product, err := lockProduct(tx, stub.ProductID)
		if err != nil {
			return fmt.Errorf("lock product: %w", err)
		}
		snapshot, err := lockExistingSnapshot(tx, stub.ProductID)
		if err != nil {
			return err
		}
		var movement models.StockMovement
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&movement, "id = ?", input.MovementID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMovementNotFound
			}
			return fmt.Errorf("lock movement: %w", err)
		}
		latest, err := latestMovement(tx.Clauses(clause.Locking{Strength: "UPDATE"}), stub.ProductID)
		if err != nil {
			return fmt.Errorf("load latest movement: %w", err)
		}
		if latest.ID != movement.ID || movement.Revision != input.ExpectedRevision {
			return ErrMovementStale
		}
		next, result, numbersChanged, err := calculateMovementUpdate(snapshot, movement, product, input)
		if err != nil {
			return err
		}
		if numbersChanged {
			snapshot.Quantity = result.Impact.ResultQuantity
			snapshot.InventoryValueCents = result.Impact.ResultInventoryValueCents
			snapshot.MovingAverageCostCents = product.DefaultPurchaseCents
			if snapshot.Quantity == 0 {
				snapshot.MovingAverageCostCents = 0
			}
			if err := tx.Save(&snapshot).Error; err != nil {
				return fmt.Errorf("save inventory snapshot: %w", err)
			}
		}

		now := time.Now().UTC()
		nextRevision := movement.Revision + 1
		updates := map[string]any{
			"shop_id":               next.ShopID,
			"quantity_delta":        next.QuantityDelta,
			"purchase_unit_cents":   next.PurchaseUnitCents,
			"cost_unit_cents":       next.CostUnitCents,
			"purchase_amount_cents": next.PurchaseAmountCents,
			"cost_amount_cents":     next.CostAmountCents,
			"reason":                next.Reason,
			"revision":              nextRevision,
			"last_edited_by_id":     input.EditorID,
			"updated_at":            now,
		}
		update := tx.Model(&models.StockMovement{}).
			Where("id = ? AND revision = ?", movement.ID, movement.Revision).
			Updates(updates)
		if update.Error != nil {
			return fmt.Errorf("update movement: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return ErrMovementStale
		}
		metadata, err := movementAuditMetadata(result, input, movement.Revision, nextRevision, now)
		if err != nil {
			return err
		}
		editorID := input.EditorID
		audit := models.AuditLog{
			ActorID: &editorID, Action: "movement.updated", ResourceType: "stock_movement",
			ResourceID: movement.ID.String(), Metadata: metadata, IPAddress: input.IPAddress, CreatedAt: now,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("create movement audit: %w", err)
		}
		editResult = result
		return nil
	})
	if err != nil {
		return models.StockMovement{}, MovementEditResult{}, err
	}

	movement, err := loadMovementWithAssociations(s.DB, input.MovementID)
	if err != nil {
		return models.StockMovement{}, MovementEditResult{}, err
	}
	movement, err = CurrentPriceMovement(movement)
	if err != nil {
		return models.StockMovement{}, MovementEditResult{}, err
	}
	movement.IsLatest = true
	return movement, editResult, nil
}

func calculateMovementUpdate(current models.InventorySnapshot, movement models.StockMovement, product models.Product, input MovementUpdateInput) (models.StockMovement, MovementEditResult, bool, error) {
	if err := repriceSnapshot(&current, product.DefaultPurchaseCents); err != nil {
		return models.StockMovement{}, MovementEditResult{}, false, err
	}
	beforeMovement, err := priceMovement(movement, product)
	if err != nil {
		return models.StockMovement{}, MovementEditResult{}, false, err
	}
	next := beforeMovement
	next.ShopID = input.ShopID
	next.Reason = input.Note
	numbersChanged := false
	resultSnapshot := current

	switch movement.Type {
	case models.MovementTypeInbound:
		if input.Quantity == nil || input.QuantityDelta != nil {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("入库需要数量")
		}
		if *input.Quantity <= 0 {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("入库数量必须大于 0")
		}
		numbersChanged = movement.QuantityDelta != *input.Quantity
		if numbersChanged {
			if product.ArchivedAt != nil {
				return models.StockMovement{}, MovementEditResult{}, false, ErrProductArchived
			}
			before, err := reverseLatestSnapshot(current, movement, product.DefaultPurchaseCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, err
			}
			calculated, err := applyInbound(&before, *input.Quantity, product.DefaultPurchaseCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, movementCalculationError(err)
			}
			setMovementNumbers(&next, calculated)
			resultSnapshot = before
		}
	case models.MovementTypeSalesOutbound:
		if input.Quantity == nil || input.QuantityDelta != nil || input.ShopID == nil {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("销售出库需要数量和店铺")
		}
		if *input.Quantity <= 0 {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("出库数量必须大于 0")
		}
		numbersChanged = -movement.QuantityDelta != *input.Quantity
		if numbersChanged {
			if product.ArchivedAt != nil {
				return models.StockMovement{}, MovementEditResult{}, false, ErrProductArchived
			}
			before, err := reverseLatestSnapshot(current, movement, product.DefaultPurchaseCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, err
			}
			calculated, err := applySalesOutbound(&before, *input.Quantity, product.DefaultPurchaseCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, movementCalculationError(err)
			}
			setMovementNumbers(&next, calculated)
			resultSnapshot = before
		}
	case models.MovementTypeAdjustment:
		if input.QuantityDelta == nil || input.Quantity != nil || input.ShopID != nil {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("库存调整只接受调整数量和备注")
		}
		if err := validateAdjustment(*input.QuantityDelta, input.Note); err != nil {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation(err.Error())
		}
		numbersChanged = movement.QuantityDelta != *input.QuantityDelta
		if numbersChanged {
			if product.ArchivedAt != nil {
				return models.StockMovement{}, MovementEditResult{}, false, ErrProductArchived
			}
			before, err := reverseLatestSnapshot(current, movement, product.DefaultPurchaseCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, err
			}
			calculated, err := applyAdjustment(&before, *input.QuantityDelta, input.Note, product.DefaultPurchaseCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, movementCalculationError(err)
			}
			setMovementNumbers(&next, calculated)
			resultSnapshot = before
		}
	default:
		return models.StockMovement{}, MovementEditResult{}, false, movementValidation("不支持的流水类型")
	}

	result, err := buildMovementEditResult(current, resultSnapshot, beforeMovement, next)
	if err != nil {
		return models.StockMovement{}, MovementEditResult{}, false, err
	}
	return next, result, numbersChanged, nil
}

func validateMovementUpdateInput(input MovementUpdateInput, requireEditor bool) error {
	if input.MovementID == uuid.Nil || input.ExpectedRevision < 1 {
		return movementValidation("流水 ID 或版本无效")
	}
	if utf8.RuneCountInString(input.Note) > 500 {
		return movementValidation("备注不能超过 500 字")
	}
	reason := strings.TrimSpace(input.ChangeReason)
	if reason == "" || utf8.RuneCountInString(input.ChangeReason) > 500 {
		return movementValidation("修改原因必填且不能超过 500 字")
	}
	if requireEditor && input.EditorID == uuid.Nil {
		return movementValidation("编辑人无效")
	}
	return nil
}

func reverseLatestSnapshot(current models.InventorySnapshot, movement models.StockMovement, purchaseCents int64) (models.InventorySnapshot, error) {
	// ponytail: latest-only edits can reverse one saved effect; replay history if arbitrary past edits are introduced.
	before := current
	switch movement.Type {
	case models.MovementTypeInbound, models.MovementTypeSalesOutbound, models.MovementTypeAdjustment:
	default:
		return models.InventorySnapshot{}, ErrMovementState
	}
	quantity, err := checkedSub(current.Quantity, movement.QuantityDelta)
	if err != nil || quantity < 0 {
		return models.InventorySnapshot{}, ErrMovementState
	}
	before.Quantity = quantity
	if err := repriceSnapshot(&before, purchaseCents); err != nil {
		return models.InventorySnapshot{}, ErrMovementState
	}
	return before, nil
}

func buildMovementEditResult(current models.InventorySnapshot, result models.InventorySnapshot, before models.StockMovement, after models.StockMovement) (MovementEditResult, error) {
	quantityChange, err := checkedSub(result.Quantity, current.Quantity)
	if err != nil {
		return MovementEditResult{}, ErrMovementState
	}
	valueDelta, err := checkedSub(result.InventoryValueCents, current.InventoryValueCents)
	if err != nil {
		return MovementEditResult{}, ErrMovementState
	}
	purchaseDelta, err := checkedSub(after.PurchaseAmountCents, before.PurchaseAmountCents)
	if err != nil {
		return MovementEditResult{}, ErrMovementState
	}
	costDelta, err := checkedSub(after.CostAmountCents, before.CostAmountCents)
	if err != nil {
		return MovementEditResult{}, ErrMovementState
	}
	return MovementEditResult{
		Before: movementRevisionValues(before),
		After:  movementRevisionValues(after),
		Impact: MovementImpact{
			CurrentQuantity: current.Quantity, ResultQuantity: result.Quantity, QuantityChange: quantityChange,
			CurrentInventoryValueCents: current.InventoryValueCents, ResultInventoryValueCents: result.InventoryValueCents,
			InventoryValueDeltaCents: valueDelta, PurchaseAmountDeltaCents: purchaseDelta,
			CostDeltaCents: costDelta,
		},
		ExpectedRevision: before.Revision,
	}, nil
}

func movementRevisionValues(movement models.StockMovement) MovementRevisionValues {
	return MovementRevisionValues{
		ID: movement.ID, Type: movement.Type, ProductID: movement.ProductID,
		OperatorID: movement.OperatorID, CreatedAt: movement.CreatedAt,
		QuantityDelta: movement.QuantityDelta, ShopID: movement.ShopID, Note: movement.Reason,
		PurchaseAmountCents: movement.PurchaseAmountCents,
		CostAmountCents:     movement.CostAmountCents,
	}
}

func setMovementNumbers(target *models.StockMovement, source models.StockMovement) {
	target.QuantityDelta = source.QuantityDelta
	target.PurchaseUnitCents = source.PurchaseUnitCents
	target.CostUnitCents = source.CostUnitCents
	target.PurchaseAmountCents = source.PurchaseAmountCents
	target.CostAmountCents = source.CostAmountCents
}

func movementAuditMetadata(result MovementEditResult, input MovementUpdateInput, beforeRevision int64, afterRevision int64, editedAt time.Time) (datatypes.JSON, error) {
	before, err := json.Marshal(result.Before)
	if err != nil {
		return nil, fmt.Errorf("marshal movement audit before: %w", err)
	}
	after, err := json.Marshal(result.After)
	if err != nil {
		return nil, fmt.Errorf("marshal movement audit after: %w", err)
	}
	impact, err := json.Marshal(result.Impact)
	if err != nil {
		return nil, fmt.Errorf("marshal movement audit impact: %w", err)
	}
	metadata, err := json.Marshal(map[string]string{
		"before": string(before), "after": string(after), "impact": string(impact),
		"change_reason": input.ChangeReason, "editor_id": input.EditorID.String(),
		"edited_at":       editedAt.Format(time.RFC3339Nano),
		"before_revision": fmt.Sprint(beforeRevision), "after_revision": fmt.Sprint(afterRevision),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal movement audit: %w", err)
	}
	return datatypes.JSON(metadata), nil
}

func loadMovementForEdit(db *gorm.DB, movementID uuid.UUID) (models.StockMovement, error) {
	var movement models.StockMovement
	err := db.Preload("Product").Take(&movement, "id = ?", movementID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.StockMovement{}, ErrMovementNotFound
	}
	if err != nil {
		return models.StockMovement{}, fmt.Errorf("load movement: %w", err)
	}
	return movement, nil
}

func loadMovementWithAssociations(db *gorm.DB, movementID uuid.UUID) (models.StockMovement, error) {
	var movement models.StockMovement
	err := db.Preload("Product").Preload("Shop").Preload("Operator").Preload("LastEditedBy").Take(&movement, "id = ?", movementID).Error
	if err != nil {
		return models.StockMovement{}, fmt.Errorf("reload movement: %w", err)
	}
	return movement, nil
}

func latestMovement(db *gorm.DB, productID uuid.UUID) (models.StockMovement, error) {
	var movement models.StockMovement
	err := db.Where("product_id = ?", productID).Order("created_at DESC").Order("id DESC").Take(&movement).Error
	return movement, err
}

func movementCalculationError(err error) error {
	if errors.Is(err, ErrInsufficientStock) {
		return err
	}
	if errors.Is(err, ErrMovementValidation) {
		return err
	}
	return movementValidation(err.Error())
}

func movementValidation(message string) error {
	return fmt.Errorf("%w: %s", ErrMovementValidation, message)
}

func checkedAdd(left int64, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, movementValidation("数字超出允许范围")
	}
	return left + right, nil
}

func checkedSub(left int64, right int64) (int64, error) {
	if (right > 0 && left < math.MinInt64+right) || (right < 0 && left > math.MaxInt64+right) {
		return 0, movementValidation("数字超出允许范围")
	}
	return left - right, nil
}

func checkedMul(left int64, right int64) (int64, error) {
	if left == 0 || right == 0 {
		return 0, nil
	}
	if (left == math.MinInt64 && right == -1) || (right == math.MinInt64 && left == -1) {
		return 0, movementValidation("数字超出允许范围")
	}
	result := left * right
	if result/right != left {
		return 0, movementValidation("数字超出允许范围")
	}
	return result, nil
}

func lockProduct(tx *gorm.DB, productID uuid.UUID) (models.Product, error) {
	var product models.Product
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, "id = ?", productID).Error
	return product, err
}

func lockActiveProduct(tx *gorm.DB, productID uuid.UUID) (models.Product, error) {
	product, err := lockProduct(tx, productID)
	if err != nil {
		return models.Product{}, err
	}
	if product.ArchivedAt != nil {
		return models.Product{}, ErrProductArchived
	}
	return product, nil
}

func lockSnapshot(tx *gorm.DB, productID uuid.UUID) (models.InventorySnapshot, error) {
	var snapshot models.InventorySnapshot
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("product_id = ?", productID).First(&snapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		snapshot = models.InventorySnapshot{ProductID: productID}
		return snapshot, tx.Create(&snapshot).Error
	}
	return snapshot, err
}

func lockExistingSnapshot(tx *gorm.DB, productID uuid.UUID) (models.InventorySnapshot, error) {
	var snapshot models.InventorySnapshot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&snapshot, "product_id = ?", productID).Error; err != nil {
		return models.InventorySnapshot{}, fmt.Errorf("lock inventory snapshot: %w", err)
	}
	return snapshot, nil
}
