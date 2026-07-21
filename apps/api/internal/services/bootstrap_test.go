package services

import (
	"errors"
	"strings"
	"testing"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_EnsureBootstrapAdmin_creates_admin_on_empty_database(t *testing.T) {
	db := newServiceTestDB(t, &models.User{})
	err := EnsureBootstrapAdmin(db, "Admin", "admin@example.com", "password123")
	if err != nil {
		t.Fatalf("EnsureBootstrapAdmin() error = %v", err)
	}
	var user models.User
	if err := db.First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if user.Role != models.RoleAdmin || !user.Enabled || user.Name != "Admin" {
		t.Fatalf("user = %+v, want enabled admin named Admin", user)
	}
	if !PasswordMatches(user.PasswordHash, "password123") {
		t.Fatal("password hash does not match")
	}
}

func Test_EnsureBootstrapAdmin_requires_fields_on_empty_database(t *testing.T) {
	db := newServiceTestDB(t, &models.User{})
	err := EnsureBootstrapAdmin(db, "", "admin@example.com", "password123")
	if !errors.Is(err, ErrInitialAdminIncomplete) {
		t.Fatalf("error = %v, want ErrInitialAdminIncomplete", err)
	}
}

func Test_EnsureBootstrapAdmin_rejects_invalid_email_and_short_password(t *testing.T) {
	db := newServiceTestDB(t, &models.User{})
	err := EnsureBootstrapAdmin(db, "Admin", "not-an-email", "password123")
	if !errors.Is(err, ErrInitialAdminInvalid) {
		t.Fatalf("error = %v, want ErrInitialAdminInvalid", err)
	}
	err = EnsureBootstrapAdmin(db, "Admin", "admin@example.com", "short")
	if !errors.Is(err, ErrInitialAdminInvalid) {
		t.Fatalf("error = %v, want ErrInitialAdminInvalid", err)
	}
	if strings.Contains(err.Error(), "short") && strings.Contains(err.Error(), "password123") {
		t.Fatal("error should not contain the password value")
	}
}

func Test_EnsureBootstrapAdmin_ignores_env_when_users_exist(t *testing.T) {
	db := newServiceTestDB(t, &models.User{})
	hash, err := HashPassword("existing-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	existing := models.User{Name: "Old", Email: "old@example.com", PasswordHash: hash, Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing: %v", err)
	}
	err = EnsureBootstrapAdmin(db, "New", "new@example.com", "password123")
	if err != nil {
		t.Fatalf("EnsureBootstrapAdmin() error = %v", err)
	}
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	var user models.User
	if err := db.First(&user, "id = ?", existing.ID).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if user.Email != "old@example.com" || !PasswordMatches(user.PasswordHash, "existing-password") {
		t.Fatalf("existing user was modified: %+v", user)
	}
}

func Test_EnsureBootstrapAdmin_fails_when_no_enabled_admin(t *testing.T) {
	db := newServiceTestDB(t, &models.User{})
	hash, _ := HashPassword("password123")
	user := models.User{Name: "Staff", Email: "staff@example.com", PasswordHash: hash, Role: models.RoleStaff, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	err := EnsureBootstrapAdmin(db, "Admin", "admin@example.com", "password123")
	if !errors.Is(err, ErrNoEnabledAdmin) {
		t.Fatalf("error = %v, want ErrNoEnabledAdmin", err)
	}
}

func newServiceTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
