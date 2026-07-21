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
	db := newServiceTestDB(t, &models.StaffPermission{})
	adminKeys, err := EffectivePermissions(db, models.User{Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("admin permissions: %v", err)
	}
	if len(adminKeys) != len(PermissionCatalog()) {
		t.Fatalf("admin key count = %d, want %d", len(adminKeys), len(PermissionCatalog()))
	}

	if err := db.Create(&models.StaffPermission{Permission: PermProductDelete}).Error; err != nil {
		t.Fatalf("create grant: %v", err)
	}
	// Unknown and admin-only rows must be ignored.
	if err := db.Create(&models.StaffPermission{Permission: "legacy.unknown"}).Error; err != nil {
		t.Fatalf("create unknown: %v", err)
	}
	if err := db.Create(&models.StaffPermission{Permission: PermUserRead}).Error; err != nil {
		t.Fatalf("create admin-only: %v", err)
	}

	staffKeys, err := EffectivePermissions(db, models.User{Role: models.RoleStaff})
	if err != nil {
		t.Fatalf("staff permissions: %v", err)
	}
	if !reflect.DeepEqual(staffKeys, []string{PermProductDelete}) {
		t.Fatalf("staff keys = %v, want only product.delete", staffKeys)
	}
}

func Test_ReplaceStaffPermissions_atomic_replace_and_closure(t *testing.T) {
	db := newServiceTestDB(t, &models.StaffPermission{}, &models.AuditLog{})
	if err := db.Create(&models.StaffPermission{Permission: PermShopRead}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		before, after, err := ReplaceStaffPermissions(tx, []string{PermProductCreate})
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

	keys, err := EffectivePermissions(db, models.User{Role: models.RoleStaff})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{PermProductCreate, PermProductRead}) {
		t.Fatalf("keys = %v", keys)
	}
}

func Test_ReplaceStaffPermissions_rolls_back_on_failure(t *testing.T) {
	db := newServiceTestDB(t, &models.StaffPermission{})
	if err := db.Create(&models.StaffPermission{Permission: PermAuditRead}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := ReplaceStaffPermissions(tx, []string{PermProductRead})
		if err != nil {
			return err
		}
		return gorm.ErrInvalidTransaction
	})
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	keys, err := EffectivePermissions(db, models.User{Role: models.RoleStaff})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{PermAuditRead}) {
		t.Fatalf("keys after rollback = %v, want [audit.read]", keys)
	}
}
