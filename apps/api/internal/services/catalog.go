package services

import (
	"errors"
	"fmt"
	"strings"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrCatalogNotFound  = errors.New("catalog item not found")
	ErrCatalogAmbiguous = errors.New("catalog item is ambiguous")
)

type CatalogRef struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code,omitempty"`
	Name string    `json:"name"`
}

func ResolveProduct(db *gorm.DB, productID string, productCode string, productQuery string) (models.Product, []CatalogRef, error) {
	if id := strings.TrimSpace(productID); id != "" {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return models.Product{}, nil, fmt.Errorf("%w: invalid product_id", ErrCatalogNotFound)
		}
		var product models.Product
		err = db.Where("id = ? AND archived_at IS NULL", parsed).First(&product).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Product{}, nil, fmt.Errorf("%w: product not found", ErrCatalogNotFound)
		}
		if err != nil {
			return models.Product{}, nil, fmt.Errorf("load product: %w", err)
		}
		return product, nil, nil
	}

	keyword := strings.TrimSpace(productCode)
	if keyword == "" {
		keyword = strings.TrimSpace(productQuery)
	}
	if keyword == "" {
		return models.Product{}, nil, fmt.Errorf("%w: product_id, product_code, or product_query is required", ErrCatalogNotFound)
	}

	var byCode []models.Product
	if err := db.Where("archived_at IS NULL AND LOWER(code) = LOWER(?)", keyword).Find(&byCode).Error; err != nil {
		return models.Product{}, nil, fmt.Errorf("lookup product code: %w", err)
	}
	if len(byCode) == 1 {
		return byCode[0], nil, nil
	}

	var byName []models.Product
	if err := db.Where("archived_at IS NULL AND LOWER(name) = LOWER(?)", keyword).Order("code asc").Find(&byName).Error; err != nil {
		return models.Product{}, nil, fmt.Errorf("lookup product name: %w", err)
	}
	if len(byName) == 1 {
		return byName[0], nil, nil
	}
	if len(byName) > 1 {
		return models.Product{}, productRefs(byName), fmt.Errorf("%w: multiple products named %q", ErrCatalogAmbiguous, keyword)
	}

	var candidates []models.Product
	like := "%" + keyword + "%"
	if err := db.Where("archived_at IS NULL AND (LOWER(name) LIKE LOWER(?) OR LOWER(code) LIKE LOWER(?))", like, like).
		Order("name asc").Order("code asc").Limit(10).Find(&candidates).Error; err != nil {
		return models.Product{}, nil, fmt.Errorf("search products: %w", err)
	}
	return models.Product{}, productRefs(candidates), fmt.Errorf("%w: no unique product for %q", ErrCatalogNotFound, keyword)
}

func ResolveShop(db *gorm.DB, shopID string, shopQuery string) (models.Shop, []CatalogRef, error) {
	if id := strings.TrimSpace(shopID); id != "" {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return models.Shop{}, nil, fmt.Errorf("%w: invalid shop_id", ErrCatalogNotFound)
		}
		var shop models.Shop
		err = db.Where("id = ?", parsed).First(&shop).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Shop{}, nil, fmt.Errorf("%w: shop not found", ErrCatalogNotFound)
		}
		if err != nil {
			return models.Shop{}, nil, fmt.Errorf("load shop: %w", err)
		}
		return shop, nil, nil
	}

	keyword := strings.TrimSpace(shopQuery)
	if keyword == "" {
		return models.Shop{}, nil, fmt.Errorf("%w: shop_id or shop_query is required", ErrCatalogNotFound)
	}

	var byName []models.Shop
	if err := db.Where("LOWER(name) = LOWER(?)", keyword).Order("id asc").Find(&byName).Error; err != nil {
		return models.Shop{}, nil, fmt.Errorf("lookup shop name: %w", err)
	}
	if len(byName) == 1 {
		return byName[0], nil, nil
	}
	if len(byName) > 1 {
		return models.Shop{}, shopRefs(byName), fmt.Errorf("%w: multiple shops named %q", ErrCatalogAmbiguous, keyword)
	}

	var candidates []models.Shop
	if err := db.Where("LOWER(name) LIKE LOWER(?)", "%"+keyword+"%").Order("name asc").Limit(10).Find(&candidates).Error; err != nil {
		return models.Shop{}, nil, fmt.Errorf("search shops: %w", err)
	}
	return models.Shop{}, shopRefs(candidates), fmt.Errorf("%w: no unique shop for %q", ErrCatalogNotFound, keyword)
}

func productRefs(products []models.Product) []CatalogRef {
	refs := make([]CatalogRef, 0, len(products))
	for _, product := range products {
		refs = append(refs, CatalogRef{ID: product.ID, Code: product.Code, Name: product.Name})
	}
	return refs
}

func shopRefs(shops []models.Shop) []CatalogRef {
	refs := make([]CatalogRef, 0, len(shops))
	for _, shop := range shops {
		refs = append(refs, CatalogRef{ID: shop.ID, Name: shop.Name})
	}
	return refs
}
