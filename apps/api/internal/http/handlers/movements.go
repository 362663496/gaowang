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
	query := h.DB.Preload("Product").Preload("Shop").Preload("Operator").Order("created_at desc").Limit(100)
	if movementType := c.Query("type"); movementType != "" {
		query = query.Where("type = ?", movementType)
	}
	if productID := c.Query("product_id"); productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	if shopID := c.Query("shop_id"); shopID != "" {
		query = query.Where("shop_id = ?", shopID)
	}
	if err := query.Find(&items).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list movements")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
