package db

import (
	"fmt"

	"gaowang/apps/api/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const userPermissionsMigrationKey = "migration.user_permissions.v1"

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
		&models.UserPermission{},
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
	if err := migrateStaffPermissions(database); err != nil {
		return fmt.Errorf("migrate shared staff permissions: %w", err)
	}
	return nil
}

func migrateStaffPermissions(database *gorm.DB) error {
	if !database.Migrator().HasTable(&models.LegacyStaffPermission{}) {
		return nil
	}

	return database.Transaction(func(tx *gorm.DB) error {
		var migrated int64
		if err := tx.Model(&models.Setting{}).Where("key = ?", userPermissionsMigrationKey).Count(&migrated).Error; err != nil {
			return fmt.Errorf("check migration marker: %w", err)
		}
		if migrated > 0 {
			return nil
		}

		var staff []models.User
		if err := tx.Where("role = ? AND deleted_at IS NULL", models.RoleStaff).Find(&staff).Error; err != nil {
			return fmt.Errorf("load staff users: %w", err)
		}
		var legacy []models.LegacyStaffPermission
		if err := tx.Find(&legacy).Error; err != nil {
			return fmt.Errorf("load shared permissions: %w", err)
		}
		for _, user := range staff {
			for _, grant := range legacy {
				row := models.UserPermission{UserID: user.ID, Permission: grant.Permission}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
					return fmt.Errorf("copy permission %s for user %s: %w", grant.Permission, user.ID, err)
				}
			}
		}

		marker := models.Setting{Key: userPermissionsMigrationKey, Value: "done"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker).Error; err != nil {
			return fmt.Errorf("record migration marker: %w", err)
		}
		return nil
	})
}
