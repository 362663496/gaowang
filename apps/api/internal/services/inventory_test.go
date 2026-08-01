package services

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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

func Test_StockMovement_migration_defaults_existing_revision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Shop{}, &models.Product{}, &legacyStockMovement{}); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	product := models.Product{Name: "Legacy", Code: "LEGACY", Enabled: true}
	operator := models.User{Name: "Legacy", Email: "legacy@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	legacySaleUnit := int64(250)
	legacy := legacyStockMovement{
		ID: uuid.New(), Type: models.MovementTypeInbound, ProductID: product.ID,
		QuantityDelta: 1, SaleUnitCents: &legacySaleUnit, RevenueCents: 250, GrossProfitCents: 150,
		OperatorID: operator.ID, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy movement: %v", err)
	}
	if err := db.AutoMigrate(&models.StockMovement{}); err != nil {
		t.Fatalf("migrate current movement: %v", err)
	}
	var movement models.StockMovement
	if err := db.First(&movement, "id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("load migrated movement: %v", err)
	}
	if movement.Revision != 1 || movement.LastEditedByID != nil {
		t.Fatalf("migrated revision/editor = %d/%v, want 1/nil", movement.Revision, movement.LastEditedByID)
	}
	var preserved legacyStockMovement
	if err := db.First(&preserved, "id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("load preserved legacy amounts: %v", err)
	}
	if preserved.SaleUnitCents == nil || *preserved.SaleUnitCents != legacySaleUnit || preserved.RevenueCents != 250 || preserved.GrossProfitCents != 150 {
		t.Fatalf("legacy amounts changed after migration: %+v", preserved)
	}
}

func Test_InventoryService_records_inbound_sale_and_adjustment(t *testing.T) {
	// Given
	db := newInventoryTestDB(t)
	product := models.Product{
		Name: "Tea", Code: "TEA", DefaultPurchaseCents: 100, Enabled: true,
	}
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
	inboundChange, err := service.CreateInbound(InboundInput{ProductID: product.ID, ShopID: &shop.ID, Quantity: 10, OperatorID: operator.ID})
	if err != nil {
		t.Fatalf("CreateInbound() error = %v", err)
	}
	saleChange, err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 4, OperatorID: operator.ID})
	if err != nil {
		t.Fatalf("CreateSalesOutbound() error = %v", err)
	}
	adjustmentChange, err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: -2, Reason: "stocktake", OperatorID: operator.ID})
	if err != nil {
		t.Fatalf("CreateAdjustment() error = %v", err)
	}

	// Then
	if inboundChange.QuantityBefore != 0 || inboundChange.QuantityAfter != 10 ||
		saleChange.QuantityBefore != 10 || saleChange.QuantityAfter != 6 ||
		adjustmentChange.QuantityBefore != 6 || adjustmentChange.QuantityAfter != 4 {
		t.Fatalf("inventory changes = %+v / %+v / %+v", inboundChange, saleChange, adjustmentChange)
	}
	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Quantity != 4 || snapshot.MovingAverageCostCents != 100 || snapshot.InventoryValueCents != 400 {
		t.Fatalf("snapshot = qty %d cost %d value %d, want 4/100/400", snapshot.Quantity, snapshot.MovingAverageCostCents, snapshot.InventoryValueCents)
	}
	var inbound models.StockMovement
	if err := db.First(&inbound, "type = ?", models.MovementTypeInbound).Error; err != nil {
		t.Fatalf("load inbound movement: %v", err)
	}
	if inbound.ShopID == nil || *inbound.ShopID != shop.ID {
		t.Fatalf("inbound shop = %v, want %s", inbound.ShopID, shop.ID)
	}
	var outbound models.StockMovement
	if err := db.First(&outbound, "type = ?", models.MovementTypeSalesOutbound).Error; err != nil {
		t.Fatalf("load outbound movement: %v", err)
	}
	if outbound.CostAmountCents != 400 {
		t.Fatalf("outbound cost = %d, want 400", outbound.CostAmountCents)
	}
}

func Test_InventoryService_normalizes_and_validates_optional_notes(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{Name: "Tea", Code: "NOTE-TEA", DefaultPurchaseCents: 100, Enabled: true}
	operator := models.User{Name: "Admin", Email: "notes@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
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

	blank, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 2, Note: " \n\t ", OperatorID: operator.ID})
	if err != nil {
		t.Fatalf("CreateInbound() blank note error = %v", err)
	}
	if blank.Movement.Reason != "" {
		t.Fatalf("blank note = %q, want empty", blank.Movement.Reason)
	}
	boundary := strings.Repeat("界", 500)
	withNote, err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 1, Note: "  " + boundary + "  ", OperatorID: operator.ID})
	if err != nil {
		t.Fatalf("CreateSalesOutbound() 500-rune note error = %v", err)
	}
	if withNote.Movement.Reason != boundary {
		t.Fatalf("normalized note rune count = %d, want 500", utf8.RuneCountInString(withNote.Movement.Reason))
	}

	if _, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 1, Note: strings.Repeat("界", 501), OperatorID: operator.ID}); err == nil {
		t.Fatal("CreateInbound() overlong note error = nil")
	}
	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	var movementCount int64
	if err := db.Model(&models.StockMovement{}).Where("product_id = ?", product.ID).Count(&movementCount).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if snapshot.Quantity != 1 || movementCount != 2 {
		t.Fatalf("state after rejected note = quantity %d movements %d, want 1/2", snapshot.Quantity, movementCount)
	}
}

func Test_InventoryService_allows_zero_product_prices_for_all_operations(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{Name: "Water", Code: "WATER", Enabled: true}
	operator := models.User{Name: "Admin", Email: "water@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "Zero Price Shop", Enabled: true}
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
	if _, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 3, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateInbound() error = %v", err)
	}
	if _, err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 1, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateSalesOutbound() error = %v", err)
	}
	if _, err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: -1, Reason: "盘点", OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateAdjustment() error = %v", err)
	}
	var movements []models.StockMovement
	if err := db.Order("created_at").Find(&movements, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load movements: %v", err)
	}
	if len(movements) != 3 || movements[0].ShopID != nil {
		t.Fatalf("movements = %+v, want three operations and inbound without shop", movements)
	}
	for _, movement := range movements {
		if movement.PurchaseAmountCents != 0 || movement.CostAmountCents != 0 {
			t.Fatalf("zero-price movement amounts = %+v, want all zero", movement)
		}
	}
}

func Test_InventoryService_rejects_all_writes_for_archived_product(t *testing.T) {
	db := newInventoryTestDB(t)
	archivedAt := time.Now()
	product := models.Product{Name: "Archived", Code: "ARCHIVED", Enabled: false, ArchivedAt: &archivedAt}
	operator := models.User{Name: "Admin", Email: "archived@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
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
	operations := []struct {
		name string
		run  func() error
	}{
		{name: "inbound", run: func() error {
			_, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 1, OperatorID: operator.ID})
			return err
		}},
		{name: "outbound", run: func() error {
			_, err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 1, OperatorID: operator.ID})
			return err
		}},
		{name: "adjustment", run: func() error {
			_, err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: 1, Reason: "test", OperatorID: operator.ID})
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(); !errors.Is(err, ErrProductArchived) {
				t.Fatalf("error = %v, want %v", err, ErrProductArchived)
			}
		})
	}
	var snapshots int64
	var movements int64
	if err := db.Model(&models.InventorySnapshot{}).Where("product_id = ?", product.ID).Count(&snapshots).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if err := db.Model(&models.StockMovement{}).Where("product_id = ?", product.ID).Count(&movements).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if snapshots != 0 || movements != 0 {
		t.Fatalf("archived writes persisted snapshots/movements = %d/%d, want 0/0", snapshots, movements)
	}
}

func Test_InventoryService_reprices_stock_and_movements_after_product_price_change(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{
		Name: "Coffee", Code: "COF", DefaultPurchaseCents: 100, Enabled: true,
	}
	operator := models.User{Name: "Admin", Email: "admin2@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "Second", Enabled: true}
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
	if _, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 5, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateInbound() error = %v", err)
	}
	if err := db.Model(&product).Update("default_purchase_cents", 125).Error; err != nil {
		t.Fatalf("change product purchase price: %v", err)
	}

	if _, err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 2, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateSalesOutbound() error = %v", err)
	}

	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Quantity != 3 || snapshot.MovingAverageCostCents != 125 || snapshot.InventoryValueCents != 375 {
		t.Fatalf("snapshot = %d/%d/%d, want 3/125/375", snapshot.Quantity, snapshot.MovingAverageCostCents, snapshot.InventoryValueCents)
	}
	var outbound models.StockMovement
	if err := db.First(&outbound, "type = ?", models.MovementTypeSalesOutbound).Error; err != nil {
		t.Fatalf("load outbound movement: %v", err)
	}
	if outbound.CostAmountCents != 250 {
		t.Fatalf("outbound cost = %d, want 250", outbound.CostAmountCents)
	}
	var inbound models.StockMovement
	if err := db.Preload("Product").First(&inbound, "type = ?", models.MovementTypeInbound).Error; err != nil {
		t.Fatalf("load inbound movement: %v", err)
	}
	priced, err := CurrentPriceMovement(inbound)
	if err != nil {
		t.Fatalf("price inbound movement: %v", err)
	}
	if priced.PurchaseAmountCents != 625 {
		t.Fatalf("repriced inbound amount = %d, want 625", priced.PurchaseAmountCents)
	}
}

func Test_InventoryService_updates_latest_inbound_sale_and_adjustment(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{
		Name: "Tea", Code: "EDIT-TEA", DefaultPurchaseCents: 100, Enabled: true,
	}
	operator := models.User{Name: "Operator", Email: "operator@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	editor := models.User{Name: "Editor", Email: "editor@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "Edit Shop", Enabled: true}
	for _, value := range []any{&product, &operator, &editor, &shop} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	service := InventoryService{DB: db}
	if _, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 10, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err := db.Model(&product).Update("default_purchase_cents", 110).Error; err != nil {
		t.Fatalf("change purchase price: %v", err)
	}
	inbound := latestTestMovement(t, db, product.ID)
	createdAt := inbound.CreatedAt
	quantity := int64(12)
	input := MovementUpdateInput{
		MovementID: inbound.ID, ExpectedRevision: inbound.Revision, Quantity: &quantity,
		Note: "修正入库", ChangeReason: "数量录错", EditorID: editor.ID, IPAddress: "127.0.0.1",
	}
	preview, err := service.PreviewMovementUpdate(input)
	if err != nil {
		t.Fatalf("preview inbound: %v", err)
	}
	if preview.Impact.ResultQuantity != 12 || preview.Impact.ResultInventoryValueCents != 1320 {
		t.Fatalf("inbound preview impact = %+v, want qty/value 12/1320", preview.Impact)
	}
	var auditsBefore int64
	if err := db.Model(&models.AuditLog{}).Count(&auditsBefore).Error; err != nil || auditsBefore != 0 {
		t.Fatalf("preview audit count = %d error=%v, want 0", auditsBefore, err)
	}
	updated, updatedResult, err := service.UpdateMovement(input)
	if err != nil {
		t.Fatalf("update inbound: %v", err)
	}
	if updated.ID != inbound.ID || updated.OperatorID != operator.ID || !updated.CreatedAt.Equal(createdAt) || updated.Revision != 2 {
		t.Fatalf("inbound identity/revision changed incorrectly: %+v", updated)
	}
	if updated.LastEditedBy == nil || updated.LastEditedBy.ID != editor.ID || updated.Reason != "修正入库" {
		t.Fatalf("inbound edit metadata = %+v", updated)
	}
	if !reflect.DeepEqual(updatedResult.Impact, preview.Impact) {
		t.Fatalf("preview impact = %+v, saved impact = %+v", preview.Impact, updatedResult.Impact)
	}

	if _, err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 4, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create sale: %v", err)
	}
	sale := latestTestMovement(t, db, product.ID)
	saleCreatedAt := sale.CreatedAt
	quantity = 5
	saleInput := MovementUpdateInput{
		MovementID: sale.ID, ExpectedRevision: sale.Revision, Quantity: &quantity, ShopID: &shop.ID,
		Note: "修正销售", ChangeReason: "销售数量录错", EditorID: editor.ID,
	}
	saleUpdated, saleResult, err := service.UpdateMovement(saleInput)
	if err != nil {
		t.Fatalf("update sale: %v", err)
	}
	if saleUpdated.QuantityDelta != -5 || saleUpdated.CostAmountCents != 550 {
		t.Fatalf("sale delta/cost = %d/%d, want -5/550", saleUpdated.QuantityDelta, saleUpdated.CostAmountCents)
	}
	if !saleUpdated.CreatedAt.Equal(saleCreatedAt) {
		t.Fatalf("sale created_at changed from %s to %s", saleCreatedAt, saleUpdated.CreatedAt)
	}
	if saleResult.Impact.ResultQuantity != 7 || saleResult.Impact.ResultInventoryValueCents != 770 {
		t.Fatalf("sale impact = %+v, want qty/value 7/770", saleResult.Impact)
	}

	if _, err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: -2, Reason: "盘点", OperatorID: operator.ID}); err != nil {
		t.Fatalf("create adjustment: %v", err)
	}
	adjustment := latestTestMovement(t, db, product.ID)
	delta := int64(-3)
	adjustmentInput := MovementUpdateInput{
		MovementID: adjustment.ID, ExpectedRevision: adjustment.Revision, QuantityDelta: &delta,
		Note: "复盘盘点", ChangeReason: "盘点数修正", EditorID: editor.ID,
	}
	adjusted, adjustmentResult, err := service.UpdateMovement(adjustmentInput)
	if err != nil {
		t.Fatalf("update adjustment: %v", err)
	}
	if adjusted.QuantityDelta != -3 || adjusted.CostAmountCents != -330 || adjustmentResult.Impact.ResultQuantity != 4 || adjustmentResult.Impact.ResultInventoryValueCents != 440 {
		t.Fatalf("adjustment/result = %+v / %+v", adjusted, adjustmentResult.Impact)
	}

	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load final snapshot: %v", err)
	}
	if snapshot.Quantity != 4 || snapshot.MovingAverageCostCents != 110 || snapshot.InventoryValueCents != 440 {
		t.Fatalf("final snapshot = %d/%d/%d, want 4/110/440", snapshot.Quantity, snapshot.MovingAverageCostCents, snapshot.InventoryValueCents)
	}
	var audit models.AuditLog
	if err := db.Order("created_at desc").First(&audit, "action = ?", "movement.updated").Error; err != nil {
		t.Fatalf("load movement audit: %v", err)
	}
	metadata := string(audit.Metadata)
	for _, value := range []string{"before", "after", "impact", "change_reason", "盘点数修正"} {
		if !strings.Contains(metadata, value) {
			t.Fatalf("audit metadata %s missing %q", metadata, value)
		}
	}
	for _, secret := range []string{"hash", "password", "cookie"} {
		if strings.Contains(strings.ToLower(metadata), secret) {
			t.Fatalf("audit metadata contains sensitive value %q: %s", secret, metadata)
		}
	}
	var auditValues map[string]string
	if err := json.Unmarshal(audit.Metadata, &auditValues); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	var before MovementRevisionValues
	if err := json.Unmarshal([]byte(auditValues["before"]), &before); err != nil {
		t.Fatalf("decode audit before values: %v", err)
	}
	if before.ID != adjustment.ID || before.ProductID != product.ID || before.OperatorID != operator.ID || before.Type != models.MovementTypeAdjustment || before.CreatedAt.IsZero() {
		t.Fatalf("audit before identity is incomplete: %+v", before)
	}
}

func Test_InventoryService_rejects_stale_and_archived_numeric_movement_updates(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{Name: "Coffee", Code: "EDIT-COFFEE", DefaultPurchaseCents: 100, Enabled: true}
	operator := models.User{Name: "Admin", Email: "stale@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	service := InventoryService{DB: db}
	if _, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 10, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	inbound := latestTestMovement(t, db, product.ID)
	if _, err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: -1, Reason: "盘点", OperatorID: operator.ID}); err != nil {
		t.Fatalf("create adjustment: %v", err)
	}
	quantity := int64(11)
	staleInput := MovementUpdateInput{MovementID: inbound.ID, ExpectedRevision: inbound.Revision, Quantity: &quantity, ChangeReason: "旧流水", EditorID: operator.ID}
	if _, _, err := service.UpdateMovement(staleInput); !errors.Is(err, ErrMovementStale) {
		t.Fatalf("non-latest error = %v, want stale", err)
	}

	latest := latestTestMovement(t, db, product.ID)
	delta := latest.QuantityDelta
	wrongVersion := MovementUpdateInput{MovementID: latest.ID, ExpectedRevision: latest.Revision + 1, QuantityDelta: &delta, Note: "盘点", ChangeReason: "旧版本", EditorID: operator.ID}
	if _, _, err := service.UpdateMovement(wrongVersion); !errors.Is(err, ErrMovementStale) {
		t.Fatalf("wrong revision error = %v, want stale", err)
	}
	archivedAt := time.Now().UTC()
	if err := db.Model(&product).Updates(map[string]any{"archived_at": archivedAt, "enabled": false}).Error; err != nil {
		t.Fatalf("archive product: %v", err)
	}
	changedDelta := delta - 1
	archivedNumeric := MovementUpdateInput{MovementID: latest.ID, ExpectedRevision: latest.Revision, QuantityDelta: &changedDelta, Note: "盘点", ChangeReason: "归档数字", EditorID: operator.ID}
	if _, _, err := service.UpdateMovement(archivedNumeric); !errors.Is(err, ErrProductArchived) {
		t.Fatalf("archived numeric error = %v, want product archived", err)
	}
	metadataOnly := MovementUpdateInput{MovementID: latest.ID, ExpectedRevision: latest.Revision, QuantityDelta: &delta, Note: "归档后补充备注", ChangeReason: "补充说明", EditorID: operator.ID}
	var snapshotBefore models.InventorySnapshot
	if err := db.First(&snapshotBefore, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot before metadata edit: %v", err)
	}
	updated, _, err := service.UpdateMovement(metadataOnly)
	if err != nil {
		t.Fatalf("archived metadata update: %v", err)
	}
	if updated.Reason != "归档后补充备注" || updated.Revision != latest.Revision+1 {
		t.Fatalf("archived metadata result = %+v", updated)
	}
	var snapshotAfter models.InventorySnapshot
	if err := db.First(&snapshotAfter, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot after metadata edit: %v", err)
	}
	if snapshotAfter.Quantity != snapshotBefore.Quantity || snapshotAfter.MovingAverageCostCents != snapshotBefore.MovingAverageCostCents || snapshotAfter.InventoryValueCents != snapshotBefore.InventoryValueCents {
		t.Fatalf("metadata edit changed snapshot from %+v to %+v", snapshotBefore, snapshotAfter)
	}
}

func Test_InventoryService_rolls_back_rejected_or_unaudited_movement_update(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{
		Name: "Milk", Code: "EDIT-MILK", DefaultPurchaseCents: 100, Enabled: true,
	}
	operator := models.User{Name: "Admin", Email: "rollback@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "Rollback Shop", Enabled: true}
	for _, value := range []any{&product, &operator, &shop} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	service := InventoryService{DB: db}
	if _, err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 10, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if _, err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 2, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create sale: %v", err)
	}
	sale := latestTestMovement(t, db, product.ID)
	quantity := int64(11)
	input := MovementUpdateInput{MovementID: sale.ID, ExpectedRevision: sale.Revision, Quantity: &quantity, ShopID: &shop.ID, ChangeReason: "超卖", EditorID: operator.ID}
	if _, _, err := service.UpdateMovement(input); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("insufficient update error = %v", err)
	}
	assertMovementState(t, db, product.ID, sale.ID, 8, 800, sale.Revision, sale.QuantityDelta)

	quantity = 3
	if err := db.Migrator().DropTable(&models.AuditLog{}); err != nil {
		t.Fatalf("drop audit table: %v", err)
	}
	if _, _, err := service.UpdateMovement(input); err == nil {
		t.Fatal("update without audit table succeeded, want rollback")
	}
	assertMovementState(t, db, product.ID, sale.ID, 8, 800, sale.Revision, sale.QuantityDelta)
}

func Test_InventoryService_returns_empty_change_when_write_fails(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{Name: "No Stock", Code: "NO-STOCK", Enabled: true}
	operator := models.User{Name: "Admin", Email: "no-stock@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "No Stock Shop", Enabled: true}
	for _, value := range []any{&product, &operator, &shop} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	change, err := (InventoryService{DB: db}).CreateSalesOutbound(OutboundInput{
		ProductID: product.ID, ShopID: shop.ID, Quantity: 1, OperatorID: operator.ID,
	})
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("error = %v, want %v", err, ErrInsufficientStock)
	}
	if change.Product.ID != uuid.Nil || change.Movement.ID != uuid.Nil || change.QuantityBefore != 0 || change.QuantityAfter != 0 {
		t.Fatalf("failed change = %+v, want zero value", change)
	}
}

func latestTestMovement(t *testing.T, db *gorm.DB, productID uuid.UUID) models.StockMovement {
	t.Helper()
	var movement models.StockMovement
	if err := db.Where("product_id = ?", productID).Order("created_at desc").Order("id desc").First(&movement).Error; err != nil {
		t.Fatalf("load latest movement: %v", err)
	}
	return movement
}

func assertMovementState(t *testing.T, db *gorm.DB, productID uuid.UUID, movementID uuid.UUID, quantity int64, value int64, revision int64, delta int64) {
	t.Helper()
	var snapshot models.InventorySnapshot
	if err := db.First(&snapshot, "product_id = ?", productID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	var movement models.StockMovement
	if err := db.First(&movement, "id = ?", movementID).Error; err != nil {
		t.Fatalf("load movement: %v", err)
	}
	if snapshot.Quantity != quantity || snapshot.InventoryValueCents != value || movement.Revision != revision || movement.QuantityDelta != delta {
		t.Fatalf("state = qty/value/revision/delta %d/%d/%d/%d", snapshot.Quantity, snapshot.InventoryValueCents, movement.Revision, movement.QuantityDelta)
	}
}

func newInventoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

// legacyStockMovement keeps rollback-only finance columns visible to the migration regression.
type legacyStockMovement struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey"`
	Type                models.MovementType
	ProductID           uuid.UUID
	ShopID              *uuid.UUID
	QuantityDelta       int64
	PurchaseUnitCents   *int64
	SaleUnitCents       *int64
	CostUnitCents       int64
	PurchaseAmountCents int64
	RevenueCents        int64
	CostAmountCents     int64
	GrossProfitCents    int64
	Reason              string
	OperatorID          uuid.UUID
	CreatedAt           time.Time
}

func (legacyStockMovement) TableName() string {
	return "stock_movements"
}
