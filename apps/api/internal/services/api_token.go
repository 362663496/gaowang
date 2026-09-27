package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	APITokenPrefix      = "gw_"
	apiTokenPrefixLen   = 8
	apiTokenRandomBytes = 32
)

var (
	ErrAPITokenNotFound = errors.New("api token not found")
)

type APITokenService struct {
	DB     *gorm.DB
	Secret string
}

func (s APITokenService) HashToken(rawToken string) string {
	return (SessionService{Secret: s.Secret}).HashToken(rawToken)
}

func (s APITokenService) GetForUser(userID uuid.UUID) (models.APIToken, error) {
	var token models.APIToken
	err := s.DB.First(&token, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.APIToken{}, ErrAPITokenNotFound
	}
	if err != nil {
		return models.APIToken{}, fmt.Errorf("load api token: %w", err)
	}
	return token, nil
}

func (s APITokenService) ReplaceForUser(userID uuid.UUID) (rawToken string, token models.APIToken, replaced bool, err error) {
	raw, err := randomAPIToken()
	if err != nil {
		return "", models.APIToken{}, false, err
	}
	now := time.Now().UTC()
	token = models.APIToken{
		TokenHash:   s.HashToken(raw),
		UserID:      userID,
		TokenPrefix: raw[:apiTokenPrefixLen],
		CreatedAt:   now,
	}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&models.APIToken{}).Where("user_id = ?", userID).Count(&existing).Error; err != nil {
			return fmt.Errorf("count api token: %w", err)
		}
		replaced = existing > 0
		if err := tx.Where("user_id = ?", userID).Delete(&models.APIToken{}).Error; err != nil {
			return fmt.Errorf("delete api token: %w", err)
		}
		if err := tx.Create(&token).Error; err != nil {
			return fmt.Errorf("create api token: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", models.APIToken{}, false, err
	}
	return raw, token, replaced, nil
}

func (s APITokenService) DeleteForUser(userID uuid.UUID) (bool, error) {
	result := s.DB.Where("user_id = ?", userID).Delete(&models.APIToken{})
	if result.Error != nil {
		return false, fmt.Errorf("delete api token: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (s APITokenService) DeleteForUserTx(tx *gorm.DB, userID uuid.UUID) error {
	if err := tx.Where("user_id = ?", userID).Delete(&models.APIToken{}).Error; err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	return nil
}

func (s APITokenService) LookupActiveUser(rawToken string) (models.User, models.APIToken, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return models.User{}, models.APIToken{}, ErrAPITokenNotFound
	}
	hash := s.HashToken(rawToken)
	var token models.APIToken
	err := s.DB.Preload("User").First(&token, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.User{}, models.APIToken{}, ErrAPITokenNotFound
	}
	if err != nil {
		return models.User{}, models.APIToken{}, fmt.Errorf("load api token: %w", err)
	}
	if !token.User.Enabled || token.User.DeletedAt != nil || token.User.ID == uuid.Nil {
		return models.User{}, models.APIToken{}, ErrAPITokenNotFound
	}
	now := time.Now().UTC()
	_ = s.DB.Model(&models.APIToken{}).Where("token_hash = ?", hash).Update("last_used_at", now).Error
	token.LastUsedAt = &now
	return token.User, token, nil
}

func randomAPIToken() (string, error) {
	raw, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("generate api token: %w", err)
	}
	return APITokenPrefix + raw, nil
}
