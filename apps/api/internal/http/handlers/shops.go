package handlers

import (
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
