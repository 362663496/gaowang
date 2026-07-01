package services

import (
	"errors"
	"testing"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_CalculateInboundAverage_updates_quantity_cost_and_value(t *testing.T) {
	// When
	gotQty, gotCost, gotValue := calculateInboundAverage(10, 100, 1000, 10, 200)

	// Then
	if gotQty != 20 {
		t.Fatalf("quantity = %d, want 20", gotQty)
	}
	if gotCost != 150 {
		t.Fatalf("cost = %d, want 150", gotCost)
	}
	if gotValue != 3000 {
		t.Fatalf("value = %d, want 3000", gotValue)
	}
}

func Test_ValidateOutbound_rejects_insufficient_stock(t *testing.T) {
	// When
	err := validateOutbound(3, 4)

	// Then
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("error = %v, want %v", err, ErrInsufficientStock)
	}
}

func Test_ValidateOutbound_allows_exact_stock(t *testing.T) {
	// When
	err := validateOutbound(4, 4)

	// Then
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
}

func Test_ValidateAdjustment_rejects_zero_delta(t *testing.T) {
	// When
	err := validateAdjustment(0, "stocktake")

	// Then
	if err == nil {
		t.Fatal("error = nil, want adjustment error")
	}
}

func Test_ValidateAdjustment_requires_reason(t *testing.T) {
	// When
	err := validateAdjustment(1, "")

	// Then
	if err == nil {
		t.Fatal("error = nil, want reason error")
	}
}

func Test_InventoryService_records_inbound_sale_and_adjustment(t *testing.T) {
	// Given
	db := newInventoryTestDB(t)
	product := models.Product{Name: "Tea", Code: "TEA", Enabled: true}
	operator := models.User{Name: "Admin", Email: "admin@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "Main", Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("create shop: %v", err)
	}
	service := InventoryService{DB: db}

	// When
	if err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 10, UnitCents: 100, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateInbound() error = %v", err)
	}
	if err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 4, SaleUnitCents: 250, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateSalesOutbound() error = %v", err)
	}
	if err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: -2, Reason: "stocktake", OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateAdjustment() error = %v", err)
	}

	// Then
	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Quantity != 4 || snapshot.MovingAverageCostCents != 100 || snapshot.InventoryValueCents != 400 {
		t.Fatalf("snapshot = qty %d cost %d value %d, want 4/100/400", snapshot.Quantity, snapshot.MovingAverageCostCents, snapshot.InventoryValueCents)
	}
	var outbound models.StockMovement
	if err := db.First(&outbound, "type = ?", models.MovementTypeSalesOutbound).Error; err != nil {
		t.Fatalf("load outbound movement: %v", err)
	}
	if outbound.RevenueCents != 1000 || outbound.CostAmountCents != 400 || outbound.GrossProfitCents != 600 {
		t.Fatalf("outbound amounts = revenue %d cost %d gross %d, want 1000/400/600", outbound.RevenueCents, outbound.CostAmountCents, outbound.GrossProfitCents)
	}
}

func newInventoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}
