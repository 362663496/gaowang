package services

import (
	"testing"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func Test_SessionService_create_lookup_and_delete(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.Session{})
	user := createTestUser(t, db, models.RoleAdmin, true)
	svc := SessionService{DB: db, Secret: "abcdefghijklmnopqrstuvwxyz123456"}

	raw, session, err := svc.Create(user.ID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if raw == "" || session.TokenHash == "" {
		t.Fatal("expected token and hash")
	}
	if session.TokenHash == raw {
		t.Fatal("database must not store raw token")
	}
	if time.Until(session.ExpiresAt) < 6*24*time.Hour {
		t.Fatalf("expires_at too soon: %v", session.ExpiresAt)
	}

	found, _, err := svc.LookupActiveUser(raw)
	if err != nil {
		t.Fatalf("LookupActiveUser() error = %v", err)
	}
	if found.ID != user.ID {
		t.Fatalf("user id = %s, want %s", found.ID, user.ID)
	}

	if err := svc.DeleteByToken(raw); err != nil {
		t.Fatalf("DeleteByToken() error = %v", err)
	}
	if _, _, err := svc.LookupActiveUser(raw); err == nil {
		t.Fatal("expected lookup failure after delete")
	}
}

func Test_SessionService_rejects_expired_and_disabled_users(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.Session{})
	user := createTestUser(t, db, models.RoleStaff, true)
	svc := SessionService{DB: db, Secret: "abcdefghijklmnopqrstuvwxyz123456"}

	raw, session, err := svc.Create(user.ID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := db.Model(&models.Session{}).Where("token_hash = ?", session.TokenHash).Update("expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("expire session: %v", err)
	}
	if _, _, err := svc.LookupActiveUser(raw); err == nil {
		t.Fatal("expected expired session rejection")
	}

	raw2, _, err := svc.Create(user.ID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", user.ID).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, _, err := svc.LookupActiveUser(raw2); err == nil {
		t.Fatal("expected disabled user rejection")
	}
}

func Test_SessionService_delete_all_for_user(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.Session{})
	user := createTestUser(t, db, models.RoleAdmin, true)
	other := createTestUser(t, db, models.RoleStaff, true)
	svc := SessionService{DB: db, Secret: "abcdefghijklmnopqrstuvwxyz123456"}

	raw1, _, err := svc.Create(user.ID)
	if err != nil {
		t.Fatalf("Create user session: %v", err)
	}
	raw2, _, err := svc.Create(user.ID)
	if err != nil {
		t.Fatalf("Create second session: %v", err)
	}
	otherRaw, _, err := svc.Create(other.ID)
	if err != nil {
		t.Fatalf("Create other session: %v", err)
	}
	if err := svc.DeleteAllForUser(user.ID); err != nil {
		t.Fatalf("DeleteAllForUser: %v", err)
	}
	if _, _, err := svc.LookupActiveUser(raw1); err == nil {
		t.Fatal("expected first session revoked")
	}
	if _, _, err := svc.LookupActiveUser(raw2); err == nil {
		t.Fatal("expected second session revoked")
	}
	if _, _, err := svc.LookupActiveUser(otherRaw); err != nil {
		t.Fatalf("other user session should remain: %v", err)
	}
}

func createTestUser(t *testing.T, db *gorm.DB, role models.Role, enabled bool) models.User {
	t.Helper()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	id := uuid.NewString()
	user := models.User{
		Name:         "User-" + string(role) + "-" + id[:8],
		Email:        id + "@example.com",
		PasswordHash: hash,
		Role:         role,
		Enabled:      enabled,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}
