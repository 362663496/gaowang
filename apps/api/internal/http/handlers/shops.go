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
	if err := h.DB.Order("created_at desc").Find(&shops).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list shops")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": shops})
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
	c.JSON(http.StatusCreated, gin.H{"item": shop})
}
