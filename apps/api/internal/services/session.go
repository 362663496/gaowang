package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SessionCookieName = "gaowang_session"
	SessionCookiePath = "/api/v1"
	SessionTTL        = 7 * 24 * time.Hour
	sessionTokenBytes = 32
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

type SessionService struct {
	DB     *gorm.DB
	Secret string
}

func (s SessionService) HashToken(rawToken string) string {
	mac := hmac.New(sha256.New, []byte(s.Secret))
	_, _ = mac.Write([]byte(rawToken))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s SessionService) Create(userID uuid.UUID) (rawToken string, session models.Session, err error) {
	raw, err := randomToken()
	if err != nil {
		return "", models.Session{}, err
	}
	now := time.Now().UTC()
	session = models.Session{
		TokenHash: s.HashToken(raw),
		UserID:    userID,
		ExpiresAt: now.Add(SessionTTL),
		CreatedAt: now,
	}
	if err := s.DB.Create(&session).Error; err != nil {
		return "", models.Session{}, fmt.Errorf("create session: %w", err)
	}
	// Opportunistic cleanup of expired rows keeps the table small without a scheduler.
	_ = s.DB.Where("expires_at < ?", now).Delete(&models.Session{}).Error
	return raw, session, nil
}

func (s SessionService) LookupActiveUser(rawToken string) (models.User, models.Session, error) {
	if rawToken == "" {
		return models.User{}, models.Session{}, ErrSessionNotFound
	}
	hash := s.HashToken(rawToken)
	var session models.Session
	err := s.DB.Preload("User").First(&session, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.User{}, models.Session{}, ErrSessionNotFound
	}
	if err != nil {
		return models.User{}, models.Session{}, fmt.Errorf("load session: %w", err)
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.DB.Delete(&models.Session{}, "token_hash = ?", hash).Error
		return models.User{}, models.Session{}, ErrSessionExpired
	}
	if !session.User.Enabled || session.User.ID == uuid.Nil {
		_ = s.DB.Delete(&models.Session{}, "token_hash = ?", hash).Error
		return models.User{}, models.Session{}, ErrSessionNotFound
	}
	return session.User, session, nil
}

func (s SessionService) DeleteByToken(rawToken string) error {
	if rawToken == "" {
		return nil
	}
	if err := s.DB.Delete(&models.Session{}, "token_hash = ?", s.HashToken(rawToken)).Error; err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s SessionService) DeleteAllForUser(userID uuid.UUID) error {
	if err := s.DB.Where("user_id = ?", userID).Delete(&models.Session{}).Error; err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func (s SessionService) DeleteAllForUserTx(tx *gorm.DB, userID uuid.UUID) error {
	if err := tx.Where("user_id = ?", userID).Delete(&models.Session{}).Error; err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func randomToken() (string, error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
