package db

import (
	"testing"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_Migrate_copies_shared_permissions_once_for_existing_staff(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.LegacyStaffPermission{}); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	users := []models.User{
		{Name: "Admin", Email: "admin@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true},
		{Name: "Staff A", Email: "a@example.com", PasswordHash: "hash", Role: models.RoleStaff, Enabled: true},
		{Name: "Staff B", Email: "b@example.com", PasswordHash: "hash", Role: models.RoleStaff, Enabled: true},
	}
	if err := database.Create(&users).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
	admin, staffA, staffB := users[0], users[1], users[2]
	if err := database.Create(&models.LegacyStaffPermission{Permission: "product.read"}).Error; err != nil {
		t.Fatalf("seed shared permission: %v", err)
	}

	if err := Migrate(database); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	var grants []models.UserPermission
	if err := database.Order("user_id").Find(&grants).Error; err != nil {
		t.Fatalf("load migrated grants: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("migrated grants = %d, want 2", len(grants))
	}
	for _, grant := range grants {
		if grant.UserID == admin.ID || grant.Permission != "product.read" {
			t.Fatalf("unexpected migrated grant: %+v", grant)
		}
	}

	if err := database.Where("user_id = ?", staffA.ID).Delete(&models.UserPermission{}).Error; err != nil {
		t.Fatalf("customize staff A: %v", err)
	}
	staffC := models.User{Name: "Staff C", Email: "c@example.com", PasswordHash: "hash", Role: models.RoleStaff, Enabled: true}
	if err := database.Create(&staffC).Error; err != nil {
		t.Fatalf("create new staff: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var counts []struct {
		UserID uuid.UUID
		Count  int64
	}
	if err := database.Model(&models.UserPermission{}).Select("user_id, count(*) AS count").Group("user_id").Scan(&counts).Error; err != nil {
		t.Fatalf("count grants: %v", err)
	}
	if len(counts) != 1 || counts[0].UserID != staffB.ID || counts[0].Count != 1 {
		t.Fatalf("grants after repeat migration = %+v, want only staff B", counts)
	}
}
