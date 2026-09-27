package services

import (
	"errors"
	"strings"
	"testing"

	"gaowang/apps/api/internal/models"
)

func Test_APITokenService_create_lookup_replace_and_delete(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.APIToken{})
	user := createTestUser(t, db, models.RoleAdmin, true)
	other := createTestUser(t, db, models.RoleStaff, true)
	svc := APITokenService{DB: db, Secret: "abcdefghijklmnopqrstuvwxyz123456"}

	raw, token, replaced, err := svc.ReplaceForUser(user.ID)
	if err != nil {
		t.Fatalf("ReplaceForUser() error = %v", err)
	}
	if replaced || !strings.HasPrefix(raw, APITokenPrefix) || token.TokenHash == raw || token.TokenPrefix != raw[:8] {
		t.Fatalf("created token = %+v raw=%q replaced=%v", token, raw, replaced)
	}

	found, loaded, err := svc.LookupActiveUser(raw)
	if err != nil {
		t.Fatalf("LookupActiveUser() error = %v", err)
	}
	if found.ID != user.ID || loaded.UserID != user.ID || loaded.LastUsedAt == nil {
		t.Fatalf("lookup user = %+v token = %+v", found, loaded)
	}

	session := SessionService{DB: db, Secret: svc.Secret}
	if _, _, err := session.LookupActiveUser(raw); err == nil {
		t.Fatal("api token must not authenticate as a session")
	}

	raw2, _, replaced, err := svc.ReplaceForUser(user.ID)
	if err != nil || !replaced {
		t.Fatalf("regenerate error = %v replaced = %v", err, replaced)
	}
	if _, _, err := svc.LookupActiveUser(raw); err == nil {
		t.Fatal("expected old token to fail after replace")
	}
	if _, _, err := svc.LookupActiveUser(raw2); err != nil {
		t.Fatalf("new token lookup: %v", err)
	}

	otherRaw, _, _, err := svc.ReplaceForUser(other.ID)
	if err != nil {
		t.Fatalf("other token: %v", err)
	}
	deleted, err := svc.DeleteForUser(user.ID)
	if err != nil || !deleted {
		t.Fatalf("DeleteForUser = %v, %v", deleted, err)
	}
	if _, _, err := svc.LookupActiveUser(raw2); !errors.Is(err, ErrAPITokenNotFound) {
		t.Fatalf("deleted token error = %v", err)
	}
	if _, _, err := svc.LookupActiveUser(otherRaw); err != nil {
		t.Fatalf("other token should remain: %v", err)
	}
}

func Test_APITokenService_rejects_disabled_and_deleted_users(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.APIToken{})
	user := createTestUser(t, db, models.RoleStaff, true)
	svc := APITokenService{DB: db, Secret: "abcdefghijklmnopqrstuvwxyz123456"}
	raw, _, _, err := svc.ReplaceForUser(user.ID)
	if err != nil {
		t.Fatalf("ReplaceForUser() error = %v", err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", user.ID).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, _, err := svc.LookupActiveUser(raw); !errors.Is(err, ErrAPITokenNotFound) {
		t.Fatalf("disabled user error = %v", err)
	}
}
