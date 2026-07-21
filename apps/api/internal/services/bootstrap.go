package services

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"gaowang/apps/api/internal/models"
	"gorm.io/gorm"
)

var (
	ErrInitialAdminIncomplete = errors.New("INITIAL_ADMIN_NAME, INITIAL_ADMIN_EMAIL, and INITIAL_ADMIN_PASSWORD are required for empty database")
	ErrInitialAdminInvalid    = errors.New("initial admin credentials are invalid")
	ErrNoEnabledAdmin         = errors.New("database has users but no enabled admin")
)

// EnsureBootstrapAdmin creates the first admin only when the users table is empty.
// Non-empty databases never consume initial-admin env values.
func EnsureBootstrapAdmin(db *gorm.DB, name string, email string, password string) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		var adminCount int64
		if err := db.Model(&models.User{}).Where("role = ? AND enabled = ?", models.RoleAdmin, true).Count(&adminCount).Error; err != nil {
			return fmt.Errorf("count enabled admins: %w", err)
		}
		if adminCount == 0 {
			return ErrNoEnabledAdmin
		}
		return nil
	}

	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" || email == "" || password == "" {
		return ErrInitialAdminIncomplete
	}
	if len(password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrInitialAdminInvalid)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%w: email is invalid", ErrInitialAdminInvalid)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	user := models.User{
		Name:         name,
		Email:        email,
		PasswordHash: hash,
		Role:         models.RoleAdmin,
		Enabled:      true,
	}
	if err := db.Create(&user).Error; err != nil {
		return fmt.Errorf("create initial admin: %w", err)
	}
	return nil
}
