package handlers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ProductHandler struct {
	DB  *gorm.DB
	Cfg config.Config
}

type updateProductEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

func (h ProductHandler) List(c *gin.Context) {
	var products []models.Product
	query := h.DB.Order("created_at desc")
	if keyword := c.Query("q"); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name ILIKE ? OR code ILIKE ?", like, like)
	}
	if err := query.Find(&products).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list products")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": products})
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
	if err := h.DB.First(&product, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "PRODUCT_LOOKUP_FAILED", "查询商品失败")
		return
	}
	product.Enabled = *req.Enabled
	if err := h.DB.Model(&product).Update("enabled", product.Enabled).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "PRODUCT_UPDATE_FAILED", "更新商品状态失败")
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
	if err := h.DB.First(&product, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "PRODUCT_LOOKUP_FAILED", "查询商品失败")
		return
	}
	var references int64
	if err := h.DB.Model(&models.InventorySnapshot{}).Where("product_id = ?", id).Count(&references).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "PRODUCT_REFERENCE_CHECK_FAILED", "检查商品关联记录失败")
		return
	}
	if references == 0 {
		if err := h.DB.Model(&models.StockMovement{}).Where("product_id = ?", id).Count(&references).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "PRODUCT_REFERENCE_CHECK_FAILED", "检查商品关联记录失败")
			return
		}
	}
	if references > 0 {
		writeError(c, http.StatusConflict, "PRODUCT_IN_USE", "商品已有库存或出入库记录，请禁用而不是删除")
		return
	}
	if err := h.DB.Delete(&product).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "PRODUCT_DELETE_FAILED", "删除商品失败")
		return
	}
	recordAudit(c, h.DB, "product.delete", "product", product.ID.String(), map[string]string{"code": product.Code, "name": product.Name})
	if h.Cfg.UploadDir != "" && product.ImagePath != "" {
		_ = os.Remove(filepath.Join(h.Cfg.UploadDir, filepath.Base(product.ImagePath)))
	}
	c.Status(http.StatusNoContent)
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
