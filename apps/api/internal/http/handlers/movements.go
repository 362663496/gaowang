package handlers

import (
	"net/http"

	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MovementHandler struct {
	DB *gorm.DB
}

func (h MovementHandler) List(c *gin.Context) {
	var items []models.StockMovement
	query := h.DB.Model(&models.StockMovement{})
	if movementType := c.Query("type"); movementType != "" {
		query = query.Where("type = ?", movementType)
	}
	if productID := c.Query("product_id"); productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	if shopID := c.Query("shop_id"); shopID != "" {
		query = query.Where("shop_id = ?", shopID)
	}
	query, meta, err := paginate(c, query)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to count movements")
		return
	}
	if err := query.Preload("Product").Preload("Shop").Preload("Operator").Order("created_at desc").Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list movements")
		return
	}
	writePage(c, items, meta)
}
