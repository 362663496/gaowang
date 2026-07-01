package services

import (
	"errors"
	"fmt"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInsufficientStock = errors.New("insufficient stock")

type InventoryService struct {
	DB *gorm.DB
}

type InboundInput struct {
	ProductID  uuid.UUID
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

func calculateInboundAverage(currentQty int64, _ int64, currentValue int64, inboundQty int64, inboundUnit int64) (int64, int64, int64) {
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
	if reason == "" {
		return fmt.Errorf("adjustment reason is required")
	}
	return nil
}

func (s InventoryService) CreateInbound(input InboundInput) error {
	if input.Quantity <= 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		qty, cost, value := calculateInboundAverage(snapshot.Quantity, snapshot.MovingAverageCostCents, snapshot.InventoryValueCents, input.Quantity, input.UnitCents)
		snapshot.Quantity = qty
		snapshot.MovingAverageCostCents = cost
		snapshot.InventoryValueCents = value
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		unit := input.UnitCents
		movement := models.StockMovement{
			Type: models.MovementTypeInbound, ProductID: input.ProductID, QuantityDelta: input.Quantity,
			PurchaseUnitCents: &unit, CostUnitCents: input.UnitCents, PurchaseAmountCents: input.Quantity * input.UnitCents,
			OperatorID: input.OperatorID,
		}
		return tx.Create(&movement).Error
	})
}

func (s InventoryService) CreateSalesOutbound(input OutboundInput) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		if err := validateOutbound(snapshot.Quantity, input.Quantity); err != nil {
			return err
		}
		costUnit := snapshot.MovingAverageCostCents
		costAmount := input.Quantity * costUnit
		revenue := input.Quantity * input.SaleUnitCents
		snapshot.Quantity -= input.Quantity
		snapshot.InventoryValueCents -= costAmount
		if snapshot.Quantity == 0 {
			snapshot.MovingAverageCostCents = 0
			snapshot.InventoryValueCents = 0
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		sale := input.SaleUnitCents
		movement := models.StockMovement{
			Type: models.MovementTypeSalesOutbound, ProductID: input.ProductID, ShopID: &input.ShopID,
			QuantityDelta: -input.Quantity, SaleUnitCents: &sale, CostUnitCents: costUnit,
			RevenueCents: revenue, CostAmountCents: costAmount, GrossProfitCents: revenue - costAmount,
			OperatorID: input.OperatorID,
		}
		return tx.Create(&movement).Error
	})
}

func (s InventoryService) CreateAdjustment(input AdjustmentInput) error {
	if err := validateAdjustment(input.QuantityDelta, input.Reason); err != nil {
		return err
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		if snapshot.Quantity+input.QuantityDelta < 0 {
			return ErrInsufficientStock
		}
		snapshot.Quantity += input.QuantityDelta
		snapshot.InventoryValueCents = snapshot.Quantity * snapshot.MovingAverageCostCents
		if err := tx.Save(&snapshot).Error; err != nil {
			return fmt.Errorf("save inventory snapshot: %w", err)
		}
		movement := models.StockMovement{
			Type: models.MovementTypeAdjustment, ProductID: input.ProductID, QuantityDelta: input.QuantityDelta,
			CostUnitCents: snapshot.MovingAverageCostCents, CostAmountCents: input.QuantityDelta * snapshot.MovingAverageCostCents,
			Reason: input.Reason, OperatorID: input.OperatorID,
		}
		return tx.Create(&movement).Error
	})
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
