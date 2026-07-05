package handlers

import (
	"net/http"
	"time"

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

type salesTrendRow struct {
	Day              string `json:"day"`
	RevenueCents     int64  `json:"revenue_cents"`
	CostCents        int64  `json:"cost_cents"`
	GrossProfitCents int64  `json:"gross_profit_cents"`
	QuantitySold     int64  `json:"quantity_sold"`
}

type productRankingRow struct {
	ProductID        string `json:"product_id"`
	ProductName      string `json:"product_name"`
	ProductCode      string `json:"product_code"`
	RevenueCents     int64  `json:"revenue_cents"`
	CostCents        int64  `json:"cost_cents"`
	GrossProfitCents int64  `json:"gross_profit_cents"`
	QuantitySold     int64  `json:"quantity_sold"`
	MovementCount    int64  `json:"movement_count"`
}

type shopRankingRow struct {
	ShopID           string `json:"shop_id"`
	ShopName         string `json:"shop_name"`
	RevenueCents     int64  `json:"revenue_cents"`
	CostCents        int64  `json:"cost_cents"`
	GrossProfitCents int64  `json:"gross_profit_cents"`
	QuantitySold     int64  `json:"quantity_sold"`
	MovementCount    int64  `json:"movement_count"`
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

func (h ReportHandler) SalesTrend(c *gin.Context) {
	from, to := reportRange(c)
	items := make([]salesTrendRow, 0)
	err := salesReportBase(h.DB, from, to).
		Select(reportDateExpr(h.DB) + " AS day, COALESCE(SUM(revenue_cents),0) AS revenue_cents, COALESCE(SUM(cost_amount_cents),0) AS cost_cents, COALESCE(SUM(gross_profit_cents),0) AS gross_profit_cents, COALESCE(SUM(-quantity_delta),0) AS quantity_sold").
		Group("day").
		Order("day asc").
		Scan(&items).Error
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load sales trend")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h ReportHandler) ProductRanking(c *gin.Context) {
	from, to := reportRange(c)
	items := make([]productRankingRow, 0)
	err := salesReportBase(h.DB, from, to).
		Joins("JOIN products ON products.id = stock_movements.product_id").
		Select("stock_movements.product_id AS product_id, products.name AS product_name, products.code AS product_code, COALESCE(SUM(stock_movements.revenue_cents),0) AS revenue_cents, COALESCE(SUM(stock_movements.cost_amount_cents),0) AS cost_cents, COALESCE(SUM(stock_movements.gross_profit_cents),0) AS gross_profit_cents, COALESCE(SUM(-stock_movements.quantity_delta),0) AS quantity_sold, COUNT(*) AS movement_count").
		Group("stock_movements.product_id, products.name, products.code").
		Order("revenue_cents desc").
		Limit(queryLimit(c, 10, 50)).
		Scan(&items).Error
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load product ranking")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h ReportHandler) ShopRanking(c *gin.Context) {
	from, to := reportRange(c)
	items := make([]shopRankingRow, 0)
	err := salesReportBase(h.DB, from, to).
		Joins("JOIN shops ON shops.id = stock_movements.shop_id").
		Select("stock_movements.shop_id AS shop_id, shops.name AS shop_name, COALESCE(SUM(stock_movements.revenue_cents),0) AS revenue_cents, COALESCE(SUM(stock_movements.cost_amount_cents),0) AS cost_cents, COALESCE(SUM(stock_movements.gross_profit_cents),0) AS gross_profit_cents, COALESCE(SUM(-stock_movements.quantity_delta),0) AS quantity_sold, COUNT(*) AS movement_count").
		Group("stock_movements.shop_id, shops.name").
		Order("revenue_cents desc").
		Limit(queryLimit(c, 10, 50)).
		Scan(&items).Error
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load shop ranking")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func salesReportBase(db *gorm.DB, from time.Time, to time.Time) *gorm.DB {
	return db.Table("stock_movements").
		Where("stock_movements.type = ?", "sales_outbound").
		Where("stock_movements.created_at >= ? AND stock_movements.created_at < ?", from, to)
}

func reportRange(c *gin.Context) (time.Time, time.Time) {
	to := time.Now().AddDate(0, 0, 1)
	if value, ok := queryDate(c, "to"); ok {
		to = value.AddDate(0, 0, 1)
	}
	from := to.AddDate(0, 0, -30)
	if value, ok := queryDate(c, "from"); ok {
		from = value
	}
	return from, to
}

func reportDateExpr(db *gorm.DB) string {
	if db.Dialector.Name() == "postgres" {
		return "TO_CHAR(DATE(stock_movements.created_at), 'YYYY-MM-DD')"
	}
	return "DATE(stock_movements.created_at)"
}
