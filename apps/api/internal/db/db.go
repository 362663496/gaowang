package db

import (
	"fmt"

	"gaowang/apps/api/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(databaseURL string) (*gorm.DB, error) {
	database, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}
	return database, nil
}

func Migrate(database *gorm.DB) error {
	if err := database.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.StaffPermission{},
		&models.Shop{},
		&models.Product{},
		&models.InventorySnapshot{},
		&models.StockMovement{},
		&models.AuditLog{},
		&models.BackupJob{},
		&models.Setting{},
	); err != nil {
		return fmt.Errorf("migrate database schema: %w", err)
	}
	return nil
}
