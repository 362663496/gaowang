package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct {
	DB *gorm.DB
}

type salesSummary struct {
	RevenueCents     int64 `json:"revenue_cents"`
	CostCents        int64 `json:"cost_cents"`
	GrossProfitCents int64 `json:"gross_profit_cents"`
}

func (h ReportHandler) SalesSummary(c *gin.Context) {
	var summary salesSummary
	err := h.DB.Table("stock_movements").
		Select("COALESCE(SUM(revenue_cents),0) AS revenue_cents, COALESCE(SUM(cost_amount_cents),0) AS cost_cents, COALESCE(SUM(gross_profit_cents),0) AS gross_profit_cents").
		Where("type = ?", "sales_outbound").
		Scan(&summary).Error
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load report")
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary})
}
