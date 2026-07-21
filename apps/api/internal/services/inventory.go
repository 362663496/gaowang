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
	UnitCents  int64
	OperatorID uuid.UUID
}

type OutboundInput struct {
	ProductID     uuid.UUID
	ShopID        uuid.UUID
	Quantity      int64
	SaleUnitCents int64
	OperatorID    uuid.UUID
}

type AdjustmentInput struct {
	ProductID     uuid.UUID
	QuantityDelta int64
	Reason        string
	OperatorID    uuid.UUID
}

type MovementUpdateInput struct {
	MovementID       uuid.UUID
	ExpectedRevision int64
	Quantity         *int64
	QuantityDelta    *int64
	UnitCents        *int64
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
	PurchaseUnitCents   *int64              `json:"purchase_unit_cents"`
	SaleUnitCents       *int64              `json:"sale_unit_cents"`
	ShopID              *uuid.UUID          `json:"shop_id"`
	Note                string              `json:"note"`
	CostUnitCents       int64               `json:"cost_unit_cents"`
	PurchaseAmountCents int64               `json:"purchase_amount_cents"`
	RevenueCents        int64               `json:"revenue_cents"`
	CostAmountCents     int64               `json:"cost_amount_cents"`
	GrossProfitCents    int64               `json:"gross_profit_cents"`
}

type MovementImpact struct {
	CurrentQuantity               int64 `json:"current_quantity"`
	ResultQuantity                int64 `json:"result_quantity"`
	QuantityChange                int64 `json:"quantity_change"`
	CurrentMovingAverageCostCents int64 `json:"current_moving_average_cost_cents"`
	ResultMovingAverageCostCents  int64 `json:"result_moving_average_cost_cents"`
	CurrentInventoryValueCents    int64 `json:"current_inventory_value_cents"`
	ResultInventoryValueCents     int64 `json:"result_inventory_value_cents"`
	InventoryValueDeltaCents      int64 `json:"inventory_value_delta_cents"`
	PurchaseAmountDeltaCents      int64 `json:"purchase_amount_delta_cents"`
	RevenueDeltaCents             int64 `json:"revenue_delta_cents"`
	CostDeltaCents                int64 `json:"cost_delta_cents"`
	GrossProfitDeltaCents         int64 `json:"gross_profit_delta_cents"`
}

type MovementEditResult struct {
	Before           MovementRevisionValues `json:"before"`
	After            MovementRevisionValues `json:"after"`
	Impact           MovementImpact         `json:"impact"`
	ExpectedRevision int64                  `json:"expected_revision"`
}

func calculateInboundAverage(currentQty int64, currentValue int64, inboundQty int64, inboundUnit int64) (int64, int64, int64) {
	newQty := currentQty + inboundQty
	newValue := currentValue + inboundQty*inboundUnit
	if newQty == 0 {
		return 0, 0, 0
	}
	return newQty, newValue / newQty, newValue
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

func applyInbound(snapshot *models.InventorySnapshot, quantity int64, unitCents int64) (models.StockMovement, error) {
	if quantity <= 0 {
		return models.StockMovement{}, fmt.Errorf("quantity must be greater than zero")
	}
	if unitCents < 0 {
		return models.StockMovement{}, fmt.Errorf("unit price cannot be negative")
	}
	purchaseAmount, err := checkedMul(quantity, unitCents)
	if err != nil {
		return models.StockMovement{}, err
	}
	if _, err := checkedAdd(snapshot.Quantity, quantity); err != nil {
		return models.StockMovement{}, err
	}
	if _, err := checkedAdd(snapshot.InventoryValueCents, purchaseAmount); err != nil {
		return models.StockMovement{}, err
	}
	quantityAfter, costAfter, valueAfter := calculateInboundAverage(
		snapshot.Quantity,
		snapshot.InventoryValueCents,
		quantity,
		unitCents,
	)
	snapshot.Quantity = quantityAfter
	snapshot.MovingAverageCostCents = costAfter
	snapshot.InventoryValueCents = valueAfter
	unit := unitCents
	return models.StockMovement{
		Type:                models.MovementTypeInbound,
		QuantityDelta:       quantity,
		PurchaseUnitCents:   &unit,
		CostUnitCents:       unitCents,
		PurchaseAmountCents: purchaseAmount,
	}, nil
}

func applySalesOutbound(snapshot *models.InventorySnapshot, quantity int64, saleUnitCents int64) (models.StockMovement, error) {
	if err := validateOutbound(snapshot.Quantity, quantity); err != nil {
		return models.StockMovement{}, err
	}
	if saleUnitCents < 0 {
		return models.StockMovement{}, fmt.Errorf("sale price cannot be negative")
	}
	costUnit := snapshot.MovingAverageCostCents
	costAmount, err := checkedMul(quantity, costUnit)
	if err != nil {
		return models.StockMovement{}, err
	}
	if quantity == snapshot.Quantity {
		costAmount = snapshot.InventoryValueCents
	}
	revenue, err := checkedMul(quantity, saleUnitCents)
	if err != nil {
		return models.StockMovement{}, err
	}
	grossProfit, err := checkedSub(revenue, costAmount)
	if err != nil {
		return models.StockMovement{}, err
	}
	snapshot.Quantity -= quantity
	snapshot.InventoryValueCents -= costAmount
	if snapshot.Quantity == 0 {
		snapshot.MovingAverageCostCents = 0
		snapshot.InventoryValueCents = 0
	}
	sale := saleUnitCents
	return models.StockMovement{
		Type:             models.MovementTypeSalesOutbound,
		QuantityDelta:    -quantity,
		SaleUnitCents:    &sale,
		CostUnitCents:    costUnit,
		RevenueCents:     revenue,
		CostAmountCents:  costAmount,
		GrossProfitCents: grossProfit,
	}, nil
}

func applyAdjustment(snapshot *models.InventorySnapshot, quantityDelta int64, reason string) (models.StockMovement, error) {
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
	valueAfter, err := checkedMul(quantityAfter, snapshot.MovingAverageCostCents)
	if err != nil {
		return models.StockMovement{}, err
	}
	costAmount, err := checkedMul(quantityDelta, snapshot.MovingAverageCostCents)
	if err != nil {
		return models.StockMovement{}, err
	}
	costUnit := snapshot.MovingAverageCostCents
	snapshot.Quantity = quantityAfter
	snapshot.InventoryValueCents = valueAfter
	return models.StockMovement{
		Type:            models.MovementTypeAdjustment,
		QuantityDelta:   quantityDelta,
		CostUnitCents:   costUnit,
		CostAmountCents: costAmount,
		Reason:          reason,
	}, nil
}

func (s InventoryService) CreateInbound(input InboundInput) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockActiveProduct(tx, input.ProductID); err != nil {
			return err
		}
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		movement, err := applyInbound(&snapshot, input.Quantity, input.UnitCents)
		if err != nil {
			return err
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		movement.ProductID = input.ProductID
		movement.ShopID = input.ShopID
		movement.OperatorID = input.OperatorID
		return tx.Create(&movement).Error
	})
}

func (s InventoryService) CreateSalesOutbound(input OutboundInput) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockActiveProduct(tx, input.ProductID); err != nil {
			return err
		}
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		movement, err := applySalesOutbound(&snapshot, input.Quantity, input.SaleUnitCents)
		if err != nil {
			return err
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		movement.ProductID = input.ProductID
		movement.ShopID = &input.ShopID
		movement.OperatorID = input.OperatorID
		return tx.Create(&movement).Error
	})
}

func (s InventoryService) CreateAdjustment(input AdjustmentInput) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockActiveProduct(tx, input.ProductID); err != nil {
			return err
		}
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		movement, err := applyAdjustment(&snapshot, input.QuantityDelta, input.Reason)
		if err != nil {
			return err
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		movement.ProductID = input.ProductID
		movement.OperatorID = input.OperatorID
		return tx.Create(&movement).Error
	})
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
			snapshot.MovingAverageCostCents = result.Impact.ResultMovingAverageCostCents
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
			"sale_unit_cents":       next.SaleUnitCents,
			"cost_unit_cents":       next.CostUnitCents,
			"purchase_amount_cents": next.PurchaseAmountCents,
			"revenue_cents":         next.RevenueCents,
			"cost_amount_cents":     next.CostAmountCents,
			"gross_profit_cents":    next.GrossProfitCents,
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
	movement.IsLatest = true
	return movement, editResult, nil
}

func calculateMovementUpdate(current models.InventorySnapshot, movement models.StockMovement, product models.Product, input MovementUpdateInput) (models.StockMovement, MovementEditResult, bool, error) {
	next := movement
	next.ShopID = input.ShopID
	next.Reason = input.Note
	numbersChanged := false
	resultSnapshot := current

	switch movement.Type {
	case models.MovementTypeInbound:
		if input.Quantity == nil || input.UnitCents == nil || input.QuantityDelta != nil {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("入库需要数量和进货单价")
		}
		if *input.Quantity <= 0 || *input.UnitCents < 0 {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("入库数量必须大于 0，单价不能为负数")
		}
		oldUnit := int64(0)
		if movement.PurchaseUnitCents != nil {
			oldUnit = *movement.PurchaseUnitCents
		}
		numbersChanged = movement.QuantityDelta != *input.Quantity || oldUnit != *input.UnitCents
		if numbersChanged {
			if product.ArchivedAt != nil {
				return models.StockMovement{}, MovementEditResult{}, false, ErrProductArchived
			}
			before, err := reverseLatestSnapshot(current, movement)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, err
			}
			calculated, err := applyInbound(&before, *input.Quantity, *input.UnitCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, movementCalculationError(err)
			}
			setMovementNumbers(&next, calculated)
			resultSnapshot = before
		}
	case models.MovementTypeSalesOutbound:
		if input.Quantity == nil || input.UnitCents == nil || input.QuantityDelta != nil || input.ShopID == nil {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("销售出库需要数量、销售单价和店铺")
		}
		if *input.Quantity <= 0 || *input.UnitCents < 0 {
			return models.StockMovement{}, MovementEditResult{}, false, movementValidation("出库数量必须大于 0，单价不能为负数")
		}
		oldUnit := int64(0)
		if movement.SaleUnitCents != nil {
			oldUnit = *movement.SaleUnitCents
		}
		numbersChanged = -movement.QuantityDelta != *input.Quantity || oldUnit != *input.UnitCents
		if numbersChanged {
			if product.ArchivedAt != nil {
				return models.StockMovement{}, MovementEditResult{}, false, ErrProductArchived
			}
			before, err := reverseLatestSnapshot(current, movement)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, err
			}
			calculated, err := applySalesOutbound(&before, *input.Quantity, *input.UnitCents)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, movementCalculationError(err)
			}
			setMovementNumbers(&next, calculated)
			resultSnapshot = before
		}
	case models.MovementTypeAdjustment:
		if input.QuantityDelta == nil || input.Quantity != nil || input.UnitCents != nil || input.ShopID != nil {
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
			before, err := reverseLatestSnapshot(current, movement)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, err
			}
			calculated, err := applyAdjustment(&before, *input.QuantityDelta, input.Note)
			if err != nil {
				return models.StockMovement{}, MovementEditResult{}, false, movementCalculationError(err)
			}
			setMovementNumbers(&next, calculated)
			resultSnapshot = before
		}
	default:
		return models.StockMovement{}, MovementEditResult{}, false, movementValidation("不支持的流水类型")
	}

	result, err := buildMovementEditResult(current, resultSnapshot, movement, next)
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

func reverseLatestSnapshot(current models.InventorySnapshot, movement models.StockMovement) (models.InventorySnapshot, error) {
	// ponytail: latest-only edits can reverse one saved effect; replay history if arbitrary past edits are introduced.
	before := current
	var err error
	switch movement.Type {
	case models.MovementTypeInbound:
		before.Quantity, err = checkedSub(current.Quantity, movement.QuantityDelta)
		if err == nil {
			before.InventoryValueCents, err = checkedSub(current.InventoryValueCents, movement.PurchaseAmountCents)
		}
		before.MovingAverageCostCents = 0
	case models.MovementTypeSalesOutbound:
		before.Quantity, err = checkedSub(current.Quantity, movement.QuantityDelta)
		if err == nil {
			before.InventoryValueCents, err = checkedAdd(current.InventoryValueCents, movement.CostAmountCents)
		}
		before.MovingAverageCostCents = movement.CostUnitCents
	case models.MovementTypeAdjustment:
		before.Quantity, err = checkedSub(current.Quantity, movement.QuantityDelta)
		before.MovingAverageCostCents = movement.CostUnitCents
		if err == nil {
			before.InventoryValueCents, err = checkedMul(before.Quantity, movement.CostUnitCents)
		}
	default:
		return models.InventorySnapshot{}, ErrMovementState
	}
	if err != nil || before.Quantity < 0 || before.InventoryValueCents < 0 {
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
	revenueDelta, err := checkedSub(after.RevenueCents, before.RevenueCents)
	if err != nil {
		return MovementEditResult{}, ErrMovementState
	}
	costDelta, err := checkedSub(after.CostAmountCents, before.CostAmountCents)
	if err != nil {
		return MovementEditResult{}, ErrMovementState
	}
	grossDelta, err := checkedSub(after.GrossProfitCents, before.GrossProfitCents)
	if err != nil {
		return MovementEditResult{}, ErrMovementState
	}
	return MovementEditResult{
		Before: movementRevisionValues(before),
		After:  movementRevisionValues(after),
		Impact: MovementImpact{
			CurrentQuantity: current.Quantity, ResultQuantity: result.Quantity, QuantityChange: quantityChange,
			CurrentMovingAverageCostCents: current.MovingAverageCostCents,
			ResultMovingAverageCostCents:  result.MovingAverageCostCents,
			CurrentInventoryValueCents:    current.InventoryValueCents, ResultInventoryValueCents: result.InventoryValueCents,
			InventoryValueDeltaCents: valueDelta, PurchaseAmountDeltaCents: purchaseDelta,
			RevenueDeltaCents: revenueDelta, CostDeltaCents: costDelta, GrossProfitDeltaCents: grossDelta,
		},
		ExpectedRevision: before.Revision,
	}, nil
}

func movementRevisionValues(movement models.StockMovement) MovementRevisionValues {
	return MovementRevisionValues{
		ID: movement.ID, Type: movement.Type, ProductID: movement.ProductID,
		OperatorID: movement.OperatorID, CreatedAt: movement.CreatedAt,
		QuantityDelta: movement.QuantityDelta, PurchaseUnitCents: movement.PurchaseUnitCents,
		SaleUnitCents: movement.SaleUnitCents, ShopID: movement.ShopID, Note: movement.Reason,
		CostUnitCents: movement.CostUnitCents, PurchaseAmountCents: movement.PurchaseAmountCents,
		RevenueCents: movement.RevenueCents, CostAmountCents: movement.CostAmountCents,
		GrossProfitCents: movement.GrossProfitCents,
	}
}

func setMovementNumbers(target *models.StockMovement, source models.StockMovement) {
	target.QuantityDelta = source.QuantityDelta
	target.PurchaseUnitCents = source.PurchaseUnitCents
	target.SaleUnitCents = source.SaleUnitCents
	target.CostUnitCents = source.CostUnitCents
	target.PurchaseAmountCents = source.PurchaseAmountCents
	target.RevenueCents = source.RevenueCents
	target.CostAmountCents = source.CostAmountCents
	target.GrossProfitCents = source.GrossProfitCents
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

func lockActiveProduct(tx *gorm.DB, productID uuid.UUID) error {
	product, err := lockProduct(tx, productID)
	if err != nil {
		return err
	}
	if product.ArchivedAt != nil {
		return ErrProductArchived
	}
	return nil
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
