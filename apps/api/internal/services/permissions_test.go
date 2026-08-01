package services

import (
	"reflect"
	"testing"

	"gaowang/apps/api/internal/models"
	"gorm.io/gorm"
)

func Test_ExpandPermissionClosure_includes_transitive_deps(t *testing.T) {
	got, err := ExpandPermissionClosure([]string{PermInventoryInbound})
	if err != nil {
		t.Fatalf("ExpandPermissionClosure() error = %v", err)
	}
	want := []string{PermInventoryInbound, PermInventoryRead, PermProductRead, PermShopRead}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test_ExpandPermissionClosure_movement_update_requires_read_dependencies(t *testing.T) {
	got, err := ExpandPermissionClosure([]string{PermMovementUpdate})
	if err != nil {
		t.Fatalf("ExpandPermissionClosure() error = %v", err)
	}
	want := []string{PermMovementRead, PermMovementUpdate, PermProductRead, PermShopRead}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test_ExpandPermissionClosure_rejects_unknown_and_admin_only(t *testing.T) {
	if _, err := ExpandPermissionClosure([]string{"nope.read"}); err == nil {
		t.Fatal("expected unknown permission error")
	}
	if _, err := ExpandPermissionClosure([]string{PermUserRead}); err == nil {
		t.Fatal("expected admin-only permission error")
	}
}

func Test_ExpandPermissionClosure_dedupes(t *testing.T) {
	got, err := ExpandPermissionClosure([]string{PermProductCreate, PermProductCreate, PermProductRead})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	want := []string{PermProductCreate, PermProductRead}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test_EffectivePermissions_admin_and_staff(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.UserPermission{})
	adminKeys, err := EffectivePermissions(db, models.User{Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("admin permissions: %v", err)
	}
	if len(adminKeys) != len(PermissionCatalog()) {
		t.Fatalf("admin key count = %d, want %d", len(adminKeys), len(PermissionCatalog()))
	}

	staffA := createPermissionTestStaff(t, db, "a@example.com")
	staffB := createPermissionTestStaff(t, db, "b@example.com")
	if err := db.Create(&models.UserPermission{UserID: staffA.ID, Permission: PermProductDelete}).Error; err != nil {
		t.Fatalf("create grant: %v", err)
	}
	// Unknown and admin-only rows must be ignored.
	if err := db.Create(&models.UserPermission{UserID: staffA.ID, Permission: "legacy.unknown"}).Error; err != nil {
		t.Fatalf("create unknown: %v", err)
	}
	if err := db.Create(&models.UserPermission{UserID: staffA.ID, Permission: PermUserRead}).Error; err != nil {
		t.Fatalf("create admin-only: %v", err)
	}
	if err := db.Create(&models.UserPermission{UserID: staffB.ID, Permission: PermAuditRead}).Error; err != nil {
		t.Fatalf("create other user grant: %v", err)
	}

	staffKeys, err := EffectivePermissions(db, staffA)
	if err != nil {
		t.Fatalf("staff permissions: %v", err)
	}
	if !reflect.DeepEqual(staffKeys, []string{PermProductDelete}) {
		t.Fatalf("staff keys = %v, want only product.delete", staffKeys)
	}
	otherKeys, err := EffectivePermissions(db, staffB)
	if err != nil || !reflect.DeepEqual(otherKeys, []string{PermAuditRead}) {
		t.Fatalf("other staff keys = %v, err = %v", otherKeys, err)
	}
}

func Test_ReplaceUserPermissions_atomic_replace_closure_and_isolation(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.UserPermission{})
	staffA := createPermissionTestStaff(t, db, "a@example.com")
	staffB := createPermissionTestStaff(t, db, "b@example.com")
	if err := db.Create(&models.UserPermission{UserID: staffA.ID, Permission: PermShopRead}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Create(&models.UserPermission{UserID: staffB.ID, Permission: PermAuditRead}).Error; err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		before, after, err := ReplaceUserPermissions(tx, staffA.ID, []string{PermProductCreate})
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(before, []string{PermShopRead}) {
			t.Fatalf("before = %v, want [shop.read]", before)
		}
		if !reflect.DeepEqual(after, []string{PermProductCreate, PermProductRead}) {
			t.Fatalf("after = %v", after)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}

	keys, err := EffectivePermissions(db, staffA)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{PermProductCreate, PermProductRead}) {
		t.Fatalf("keys = %v", keys)
	}
	otherKeys, err := EffectivePermissions(db, staffB)
	if err != nil || !reflect.DeepEqual(otherKeys, []string{PermAuditRead}) {
		t.Fatalf("other staff keys = %v, err = %v", otherKeys, err)
	}
}

func Test_ReplaceUserPermissions_rolls_back_on_failure(t *testing.T) {
	db := newServiceTestDB(t, &models.User{}, &models.UserPermission{})
	staff := createPermissionTestStaff(t, db, "staff@example.com")
	if err := db.Create(&models.UserPermission{UserID: staff.ID, Permission: PermAuditRead}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := ReplaceUserPermissions(tx, staff.ID, []string{PermProductRead})
		if err != nil {
			return err
		}
		return gorm.ErrInvalidTransaction
	})
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	keys, err := EffectivePermissions(db, staff)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{PermAuditRead}) {
		t.Fatalf("keys after rollback = %v, want [audit.read]", keys)
	}
}

func createPermissionTestStaff(t *testing.T, db *gorm.DB, email string) models.User {
	t.Helper()
	user := models.User{Name: email, Email: email, PasswordHash: "hash", Role: models.RoleStaff, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create staff: %v", err)
	}
	return user
}
