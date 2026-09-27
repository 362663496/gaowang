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

func Test_CreateSalesOutboundBatch_rolls_back_every_item(t *testing.T) {
	db := openBatchDB(t)
	user := models.User{Name: "Admin", Email: "batch@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "总店", Enabled: true}
	short := models.Product{Name: "缺货", Code: "SHORT", Enabled: true, DefaultPurchaseCents: 100}
	okProduct := models.Product{Name: "充足", Code: "OK", Enabled: true, DefaultPurchaseCents: 200}
	if err := db.Create(&user).Error; err != nil || db.Create(&shop).Error != nil || db.Create(&short).Error != nil || db.Create(&okProduct).Error != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: okProduct.ID, Quantity: 5}).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	_, err := InventoryService{DB: db}.CreateSalesOutboundBatch(OutboundBatchInput{
		ShopID: shop.ID, OperatorID: user.ID,
		Items: []OutboundBatchItem{{ProductID: okProduct.ID, ProductCode: okProduct.Code, Quantity: 1}, {ProductID: short.ID, ProductCode: short.Code, Quantity: 1}},
	}, false)
	var rejected *OutboundBatchRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("error = %v, want batch rejection", err)
	}
	var movements int64
	var snapshots int64
	if err := db.Model(&models.StockMovement{}).Count(&movements).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if err := db.Model(&models.InventorySnapshot{}).Where("product_id = ?", short.ID).Count(&snapshots).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	var kept models.InventorySnapshot
	if err := db.First(&kept, "product_id = ?", okProduct.ID).Error; err != nil {
		t.Fatalf("load kept snapshot: %v", err)
	}
	if movements != 0 || snapshots != 0 || kept.Quantity != 5 {
		t.Fatalf("partial batch persisted movements=%d shortSnapshots=%d kept=%d", movements, snapshots, kept.Quantity)
	}
}

func Test_CreateSalesOutbound_rolls_back_with_outer_transaction(t *testing.T) {
	db := openBatchDB(t)
	user := models.User{Name: "Admin", Email: "outer@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	shop := models.Shop{Name: "总店", Enabled: true}
	product := models.Product{Name: "绿茶", Code: "TEA", Enabled: true, DefaultPurchaseCents: 100}
	if err := db.Create(&user).Error; err != nil || db.Create(&shop).Error != nil || db.Create(&product).Error != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: product.ID, Quantity: 4}).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := (InventoryService{DB: tx}).CreateSalesOutbound(OutboundInput{ProductID: product.ID, ShopID: shop.ID, Quantity: 1, OperatorID: user.ID})
		if err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil || err.Error() != "force rollback" {
		t.Fatalf("outer error = %v", err)
	}
	var snapshot models.InventorySnapshot
	var movements int64
	if err := db.First(&snapshot, "product_id = ?", product.ID).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := db.Model(&models.StockMovement{}).Count(&movements).Error; err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if snapshot.Quantity != 4 || movements != 0 {
		t.Fatalf("nested outbound persisted qty=%d movements=%d", snapshot.Quantity, movements)
	}
}

func openBatchDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
