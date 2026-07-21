package services

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_CalculateInboundAverage_updates_quantity_cost_and_value(t *testing.T) {
	// When
	gotQty, gotCost, gotValue := calculateInboundAverage(10, 1000, 10, 200)

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
	legacy := legacyStockMovement{
		ID: uuid.New(), Type: models.MovementTypeInbound, ProductID: product.ID,
		QuantityDelta: 1, OperatorID: operator.ID, CreatedAt: time.Now().UTC(),
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
	if err := service.CreateInbound(InboundInput{ProductID: product.ID, ShopID: &shop.ID, Quantity: 10, UnitCents: 100, OperatorID: operator.ID}); err != nil {
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
	if outbound.RevenueCents != 1000 || outbound.CostAmountCents != 400 || outbound.GrossProfitCents != 600 {
		t.Fatalf("outbound amounts = revenue %d cost %d gross %d, want 1000/400/600", outbound.RevenueCents, outbound.CostAmountCents, outbound.GrossProfitCents)
	}
}

func Test_InventoryService_allows_inbound_without_shop(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{Name: "Water", Code: "WATER", Enabled: true}
	operator := models.User{Name: "Admin", Email: "water@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := (InventoryService{DB: db}).CreateInbound(InboundInput{ProductID: product.ID, Quantity: 1, UnitCents: 50, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateInbound() error = %v", err)
	}
	var movement models.StockMovement
	if err := db.First(&movement, "type = ?", models.MovementTypeInbound).Error; err != nil {
		t.Fatalf("load inbound movement: %v", err)
	}
	if movement.ShopID != nil {
		t.Fatalf("inbound shop = %v, want nil", movement.ShopID)
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
			return service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 1, UnitCents: 100, OperatorID: operator.ID})
		}},
		{name: "outbound", run: func() error {
			return service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 1, SaleUnitCents: 100, OperatorID: operator.ID})
		}},
		{name: "adjustment", run: func() error {
			return service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: 1, Reason: "test", OperatorID: operator.ID})
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

func Test_InventoryService_charges_remaining_value_when_sale_empties_stock(t *testing.T) {
	// Given
	db := newInventoryTestDB(t)
	product := models.Product{Name: "Coffee", Code: "COF", Enabled: true}
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
	if err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 3, UnitCents: 100, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateInbound() error = %v", err)
	}
	if err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 2, UnitCents: 101, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateInbound() error = %v", err)
	}

	// When
	if err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 5, SaleUnitCents: 150, OperatorID: operator.ID}); err != nil {
		t.Fatalf("CreateSalesOutbound() error = %v", err)
	}

	// Then
	var outbound models.StockMovement
	if err := db.First(&outbound, "type = ?", models.MovementTypeSalesOutbound).Error; err != nil {
		t.Fatalf("load outbound movement: %v", err)
	}
	if outbound.CostAmountCents != 502 || outbound.GrossProfitCents != 248 {
		t.Fatalf("outbound cost/gross = %d/%d, want 502/248", outbound.CostAmountCents, outbound.GrossProfitCents)
	}
}

func Test_InventoryService_updates_latest_inbound_sale_and_adjustment(t *testing.T) {
	db := newInventoryTestDB(t)
	product := models.Product{Name: "Tea", Code: "EDIT-TEA", Enabled: true}
	operator := models.User{Name: "Operator", Email: "operator@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	editor := models.User{Name: "Editor", Email: "editor@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "Edit Shop", Enabled: true}
	for _, value := range []any{&product, &operator, &editor, &shop} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	service := InventoryService{DB: db}
	if err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 10, UnitCents: 100, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	inbound := latestTestMovement(t, db, product.ID)
	createdAt := inbound.CreatedAt
	quantity, unit := int64(12), int64(110)
	input := MovementUpdateInput{
		MovementID: inbound.ID, ExpectedRevision: inbound.Revision, Quantity: &quantity, UnitCents: &unit,
		Note: "修正入库", ChangeReason: "数量录错", EditorID: editor.ID, IPAddress: "127.0.0.1",
	}
	preview, err := service.PreviewMovementUpdate(input)
	if err != nil {
		t.Fatalf("preview inbound: %v", err)
	}
	if preview.Impact.ResultQuantity != 12 || preview.Impact.ResultInventoryValueCents != 1320 || preview.Impact.ResultMovingAverageCostCents != 110 {
		t.Fatalf("inbound preview impact = %+v, want qty/value/avg 12/1320/110", preview.Impact)
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

	if err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 4, SaleUnitCents: 250, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create sale: %v", err)
	}
	sale := latestTestMovement(t, db, product.ID)
	saleCreatedAt := sale.CreatedAt
	quantity, unit = 5, 300
	saleInput := MovementUpdateInput{
		MovementID: sale.ID, ExpectedRevision: sale.Revision, Quantity: &quantity, UnitCents: &unit, ShopID: &shop.ID,
		Note: "修正销售", ChangeReason: "销售数量录错", EditorID: editor.ID,
	}
	saleUpdated, saleResult, err := service.UpdateMovement(saleInput)
	if err != nil {
		t.Fatalf("update sale: %v", err)
	}
	if saleUpdated.QuantityDelta != -5 || saleUpdated.RevenueCents != 1500 || saleUpdated.CostAmountCents != 550 || saleUpdated.GrossProfitCents != 950 {
		t.Fatalf("sale amounts = delta/revenue/cost/gross %d/%d/%d/%d", saleUpdated.QuantityDelta, saleUpdated.RevenueCents, saleUpdated.CostAmountCents, saleUpdated.GrossProfitCents)
	}
	if !saleUpdated.CreatedAt.Equal(saleCreatedAt) {
		t.Fatalf("sale created_at changed from %s to %s", saleCreatedAt, saleUpdated.CreatedAt)
	}
	if saleResult.Impact.ResultQuantity != 7 || saleResult.Impact.ResultInventoryValueCents != 770 {
		t.Fatalf("sale impact = %+v, want qty/value 7/770", saleResult.Impact)
	}

	if err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: -2, Reason: "盘点", OperatorID: operator.ID}); err != nil {
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
	product := models.Product{Name: "Coffee", Code: "EDIT-COFFEE", Enabled: true}
	operator := models.User{Name: "Admin", Email: "stale@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	service := InventoryService{DB: db}
	if err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 10, UnitCents: 100, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	inbound := latestTestMovement(t, db, product.ID)
	if err := service.CreateAdjustment(AdjustmentInput{ProductID: product.ID, QuantityDelta: -1, Reason: "盘点", OperatorID: operator.ID}); err != nil {
		t.Fatalf("create adjustment: %v", err)
	}
	quantity, unit := int64(11), int64(100)
	staleInput := MovementUpdateInput{MovementID: inbound.ID, ExpectedRevision: inbound.Revision, Quantity: &quantity, UnitCents: &unit, ChangeReason: "旧流水", EditorID: operator.ID}
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
	product := models.Product{Name: "Milk", Code: "EDIT-MILK", Enabled: true}
	operator := models.User{Name: "Admin", Email: "rollback@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "Rollback Shop", Enabled: true}
	for _, value := range []any{&product, &operator, &shop} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	service := InventoryService{DB: db}
	if err := service.CreateInbound(InboundInput{ProductID: product.ID, Quantity: 10, UnitCents: 100, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err := service.CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 2, SaleUnitCents: 200, OperatorID: operator.ID}); err != nil {
		t.Fatalf("create sale: %v", err)
	}
	sale := latestTestMovement(t, db, product.ID)
	quantity, unit := int64(11), int64(200)
	input := MovementUpdateInput{MovementID: sale.ID, ExpectedRevision: sale.Revision, Quantity: &quantity, UnitCents: &unit, ShopID: &shop.ID, ChangeReason: "超卖", EditorID: operator.ID}
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
