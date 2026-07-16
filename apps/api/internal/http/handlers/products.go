package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errProductHasStock = errors.New("product has stock")

type ProductHandler struct {
	DB  *gorm.DB
	Cfg config.Config
}

type updateProductEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type productListSummary struct {
	Total            int64 `json:"total"`
	Enabled          int64 `json:"enabled"`
	DefaultSaleCents int64 `json:"default_sale_cents"`
}

func (h ProductHandler) List(c *gin.Context) {
	var products []models.Product
	query := h.DB.Model(&models.Product{})
	if c.Query("include_archived") != "true" {
		query = query.Where("archived_at IS NULL")
	}
	if keyword := c.Query("q"); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name ILIKE ? OR code ILIKE ?", like, like)
	}
	var summary productListSummary
	if err := query.Session(&gorm.Session{}).Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN enabled = ? THEN 1 ELSE 0 END), 0) AS enabled, COALESCE(SUM(default_sale_cents), 0) AS default_sale_cents", true).Scan(&summary).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to summarize products")
		return
	}
	pageQuery, meta, err := paginate(c, query)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to count products")
		return
	}
	if err := pageQuery.Order("created_at desc").Find(&products).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list products")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": products, "pagination": meta, "summary": summary})
}

func (h ProductHandler) Create(c *gin.Context) {
	product, ok := productFromForm(c)
	if !ok {
		return
	}
	file, header, err := c.Request.FormFile("image")
	if err == nil {
		defer func() { _ = file.Close() }()
		path, err := services.SaveProductImage(h.Cfg.UploadDir, file, header)
		if err != nil {
			writeError(c, http.StatusBadRequest, "UPLOAD_INVALID", err.Error())
			return
		}
		product.ImagePath = path
	}
	if err := h.DB.Create(&product).Error; err != nil {
		writeError(c, http.StatusBadRequest, "PRODUCT_CREATE_FAILED", err.Error())
		return
	}
	recordAudit(c, h.DB, "product.create", "product", product.ID.String(), map[string]string{"code": product.Code, "name": product.Name})
	c.JSON(http.StatusCreated, gin.H{"item": product})
}

func (h ProductHandler) Update(c *gin.Context) {
	id, ok := parseUUID(c, c.Param("id"), "id")
	if !ok {
		return
	}
	var current models.Product
	if err := h.DB.First(&current, "id = ? AND archived_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "PRODUCT_LOOKUP_FAILED", "查询商品失败")
		return
	}
	updated, ok := productFromForm(c)
	if !ok {
		return
	}
	newImagePath := ""
	file, header, err := c.Request.FormFile("image")
	if err == nil {
		defer func() { _ = file.Close() }()
		newImagePath, err = services.SaveProductImage(h.Cfg.UploadDir, file, header)
		if err != nil {
			writeError(c, http.StatusBadRequest, "UPLOAD_INVALID", err.Error())
			return
		}
	}
	updates := map[string]any{
		"name": updated.Name, "code": updated.Code, "note": updated.Note,
		"default_purchase_cents": updated.DefaultPurchaseCents,
		"default_sale_cents":     updated.DefaultSaleCents,
		"low_stock_threshold":    updated.LowStockThreshold,
	}
	if newImagePath != "" {
		updates["image_path"] = newImagePath
	}
	result := h.DB.Model(&models.Product{}).Where("id = ? AND archived_at IS NULL", id).Updates(updates)
	if result.Error != nil {
		removeProductImage(h.Cfg.UploadDir, newImagePath)
		writeError(c, http.StatusBadRequest, "PRODUCT_UPDATE_FAILED", "商品编码已存在或数据无效")
		return
	}
	if result.RowsAffected == 0 {
		removeProductImage(h.Cfg.UploadDir, newImagePath)
		writeError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
		return
	}
	if newImagePath != "" {
		removeProductImage(h.Cfg.UploadDir, current.ImagePath)
	}
	if err := h.DB.First(&current, "id = ?", id).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "PRODUCT_LOOKUP_FAILED", "查询商品失败")
		return
	}
	recordAudit(c, h.DB, "product.update", "product", current.ID.String(), map[string]string{"code": current.Code, "name": current.Name})
	c.JSON(http.StatusOK, gin.H{"item": current})
}

func (h ProductHandler) SetEnabled(c *gin.Context) {
	id, ok := parseUUID(c, c.Param("id"), "id")
	if !ok {
		return
	}
	var req updateProductEnabledRequest
	if !bindJSON(c, &req) {
		return
	}
	var product models.Product
	if err := h.DB.First(&product, "id = ? AND archived_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "PRODUCT_LOOKUP_FAILED", "查询商品失败")
		return
	}
	product.Enabled = *req.Enabled
	result := h.DB.Model(&models.Product{}).Where("id = ? AND archived_at IS NULL", product.ID).Update("enabled", product.Enabled)
	if result.Error != nil {
		writeError(c, http.StatusInternalServerError, "PRODUCT_UPDATE_FAILED", "更新商品状态失败")
		return
	}
	if result.RowsAffected == 0 {
		writeError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
		return
	}
	action := "product.disable"
	if product.Enabled {
		action = "product.enable"
	}
	recordAudit(c, h.DB, action, "product", product.ID.String(), map[string]string{"code": product.Code})
	c.JSON(http.StatusOK, gin.H{"item": product})
}

func (h ProductHandler) Delete(c *gin.Context) {
	id, ok := parseUUID(c, c.Param("id"), "id")
	if !ok {
		return
	}
	var product models.Product
	archived := false
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, "id = ? AND archived_at IS NULL", id).Error; err != nil {
			return fmt.Errorf("load product: %w", err)
		}
		var snapshot models.InventorySnapshot
		snapshotErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&snapshot, "product_id = ?", id).Error
		if snapshotErr != nil && !errors.Is(snapshotErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load inventory snapshot: %w", snapshotErr)
		}
		if snapshotErr == nil && snapshot.Quantity != 0 {
			return errProductHasStock
		}
		var movementCount int64
		if err := tx.Model(&models.StockMovement{}).Where("product_id = ?", id).Count(&movementCount).Error; err != nil {
			return fmt.Errorf("count stock movements: %w", err)
		}
		if errors.Is(snapshotErr, gorm.ErrRecordNotFound) && movementCount == 0 {
			if err := tx.Delete(&product).Error; err != nil {
				return fmt.Errorf("delete product: %w", err)
			}
			return nil
		}
		archivedAt := time.Now()
		if err := tx.Model(&product).Updates(map[string]any{"archived_at": archivedAt, "enabled": false}).Error; err != nil {
			return fmt.Errorf("archive product: %w", err)
		}
		product.ArchivedAt = &archivedAt
		product.Enabled = false
		archived = true
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
		return
	}
	if errors.Is(err, errProductHasStock) {
		writeError(c, http.StatusConflict, "PRODUCT_HAS_STOCK", "商品当前库存不为 0，请先出库或调整为 0 后再删除")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "PRODUCT_DELETE_FAILED", "删除商品失败")
		return
	}
	action := "product.delete"
	if archived {
		action = "product.archive"
	}
	recordAudit(c, h.DB, action, "product", product.ID.String(), map[string]string{"code": product.Code, "name": product.Name})
	if !archived {
		removeProductImage(h.Cfg.UploadDir, product.ImagePath)
	}
	c.Status(http.StatusNoContent)
}

func removeProductImage(uploadDir string, imagePath string) {
	if uploadDir != "" && imagePath != "" {
		_ = os.Remove(filepath.Join(uploadDir, filepath.Base(imagePath)))
	}
}

func productFromForm(c *gin.Context) (models.Product, bool) {
	product := models.Product{Name: c.PostForm("name"), Code: c.PostForm("code"), Note: c.PostForm("note"), Enabled: true}
	if product.Name == "" || product.Code == "" {
		writeError(c, http.StatusBadRequest, "VALIDATION", "name and code are required")
		return models.Product{}, false
	}
	var ok bool
	product.DefaultPurchaseCents, ok = formInt(c, "default_purchase_cents")
	if !ok {
		return models.Product{}, false
	}
	product.DefaultSaleCents, ok = formInt(c, "default_sale_cents")
	if !ok {
		return models.Product{}, false
	}
	product.LowStockThreshold, ok = formInt(c, "low_stock_threshold")
	return product, ok
}

func formInt(c *gin.Context, name string) (int64, bool) {
	value := c.PostForm(name)
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION", name+" must be an integer")
		return 0, false
	}
	if parsed < 0 {
		writeError(c, http.StatusBadRequest, "VALIDATION", name+" cannot be negative")
		return 0, false
	}
	return parsed, true
}
