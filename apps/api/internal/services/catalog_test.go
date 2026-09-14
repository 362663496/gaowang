package services

import (
	"errors"
	"testing"

	"gaowang/apps/api/internal/models"
)

func Test_ResolveProduct_prefers_code_then_unique_name(t *testing.T) {
	db := newServiceTestDB(t, &models.Product{})
	green := models.Product{Name: "绿茶", Code: "TEA-GREEN", Enabled: true}
	leaf := models.Product{Name: "绿茶", Code: "TEA-LEAF", Enabled: true}
	black := models.Product{Name: "红茶", Code: "TEA-BLACK", Enabled: true}
	if err := db.Create(&green).Error; err != nil {
		t.Fatalf("seed green: %v", err)
	}
	if err := db.Create(&leaf).Error; err != nil {
		t.Fatalf("seed leaf: %v", err)
	}
	if err := db.Create(&black).Error; err != nil {
		t.Fatalf("seed black: %v", err)
	}

	found, _, err := ResolveProduct(db, "", "tea-green", "")
	if err != nil || found.Code != "TEA-GREEN" {
		t.Fatalf("code lookup = %+v err=%v", found, err)
	}

	found, _, err = ResolveProduct(db, "", "", "红茶")
	if err != nil || found.Code != "TEA-BLACK" {
		t.Fatalf("unique name = %+v err=%v", found, err)
	}

	_, refs, err := ResolveProduct(db, "", "", "绿茶")
	if !errors.Is(err, ErrCatalogAmbiguous) || len(refs) != 2 {
		t.Fatalf("ambiguous = refs %+v err=%v", refs, err)
	}

	_, refs, err = ResolveProduct(db, "", "", "不存在")
	if !errors.Is(err, ErrCatalogNotFound) || len(refs) != 0 {
		t.Fatalf("missing = refs %+v err=%v", refs, err)
	}

	found, _, err = ResolveProduct(db, green.ID.String(), "", "")
	if err != nil || found.ID != green.ID {
		t.Fatalf("id lookup = %+v err=%v", found, err)
	}
}

func Test_ResolveShop_unique_name_or_candidates(t *testing.T) {
	db := newServiceTestDB(t, &models.Shop{})
	main := models.Shop{Name: "总店", Enabled: true}
	east := models.Shop{Name: "东门店", Enabled: true}
	if err := db.Create(&[]models.Shop{main, east}).Error; err != nil {
		t.Fatalf("seed shops: %v", err)
	}

	found, _, err := ResolveShop(db, "", "总店")
	if err != nil || found.Name != "总店" {
		t.Fatalf("unique shop = %+v err=%v", found, err)
	}
	_, refs, err := ResolveShop(db, "", "店")
	if !errors.Is(err, ErrCatalogNotFound) || len(refs) != 2 {
		t.Fatalf("partial shop = refs %+v err=%v", refs, err)
	}
}
