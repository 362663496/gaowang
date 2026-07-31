package handlers

import (
	"errors"
	"net/http"

	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ShopHandler struct {
	DB *gorm.DB
}

type createShopRequest struct {
	Name string `json:"name" binding:"required,min=1,max=120"`
	Note string `json:"note" binding:"max=500"`
}

func (h ShopHandler) List(c *gin.Context) {
	var shops []models.Shop
	query, meta, err := paginate(c, h.DB.Model(&models.Shop{}))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to count shops")
		return
	}
	if err := query.Order("created_at desc").Find(&shops).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list shops")
		return
	}
	writePage(c, shops, meta)
}

func (h ShopHandler) Create(c *gin.Context) {
	var req createShopRequest
	if !bindJSON(c, &req) {
		return
	}
	shop := models.Shop{Name: req.Name, Note: req.Note, Enabled: true}
	if err := h.DB.Create(&shop).Error; err != nil {
		writeError(c, http.StatusBadRequest, "SHOP_CREATE_FAILED", err.Error())
		return
	}
	recordAudit(c, h.DB, "shop.create", "shop", shop.ID.String(), map[string]string{"name": shop.Name})
	c.JSON(http.StatusCreated, gin.H{"item": shop})
}

func (h ShopHandler) Update(c *gin.Context) {
	id, ok := parseUUID(c, c.Param("id"), "id")
	if !ok {
		return
	}
	var req createShopRequest
	if !bindJSON(c, &req) {
		return
	}
	var shop models.Shop
	if err := h.DB.First(&shop, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "SHOP_NOT_FOUND", "店铺不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "SHOP_LOOKUP_FAILED", "查询店铺失败")
		return
	}
	shop.Name = req.Name
	shop.Note = req.Note
	if err := h.DB.Save(&shop).Error; err != nil {
		writeError(c, http.StatusBadRequest, "SHOP_UPDATE_FAILED", "店铺名称已存在或数据无效")
		return
	}
	recordAudit(c, h.DB, "shop.update", "shop", shop.ID.String(), map[string]string{"name": shop.Name})
	c.JSON(http.StatusOK, gin.H{"item": shop})
}
