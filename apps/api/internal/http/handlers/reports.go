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

const (
	salesQuantityExpr = "(-stock_movements.quantity_delta)"
	salesCostExpr     = salesQuantityExpr + " * products.default_purchase_cents"
)

type salesSummary struct {
	QuantitySold  int64 `json:"quantity_sold"`
	MovementCount int64 `json:"movement_count"`
	CostCents     int64 `json:"cost_cents"`
}

type salesTrendRow struct {
	Day           string `json:"day"`
	QuantitySold  int64  `json:"quantity_sold"`
	MovementCount int64  `json:"movement_count"`
	CostCents     int64  `json:"cost_cents"`
}

type productRankingRow struct {
	ProductID        string `json:"product_id"`
	ProductName      string `json:"product_name"`
	ProductCode      string `json:"product_code"`
	ProductImagePath string `json:"product_image_path"`
	Archived         bool   `json:"archived"`
	CostCents        int64  `json:"cost_cents"`
	QuantitySold     int64  `json:"quantity_sold"`
	MovementCount    int64  `json:"movement_count"`
}

type shopRankingRow struct {
	ShopID        string `json:"shop_id"`
	ShopName      string `json:"shop_name"`
	CostCents     int64  `json:"cost_cents"`
	QuantitySold  int64  `json:"quantity_sold"`
	MovementCount int64  `json:"movement_count"`
}

func (h ReportHandler) SalesSummary(c *gin.Context) {
	var summary salesSummary
	err := h.DB.Table("stock_movements").
		Joins("JOIN products ON products.id = stock_movements.product_id").
		Select("COALESCE(SUM("+salesQuantityExpr+"),0) AS quantity_sold, COUNT(*) AS movement_count, COALESCE(SUM("+salesCostExpr+"),0) AS cost_cents").
		Where("stock_movements.type = ?", "sales_outbound").
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
		Select(reportDateExpr(h.DB) + " AS day, COALESCE(SUM(" + salesQuantityExpr + "),0) AS quantity_sold, COUNT(*) AS movement_count, COALESCE(SUM(" + salesCostExpr + "),0) AS cost_cents").
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
		Select("stock_movements.product_id AS product_id, products.name AS product_name, products.code AS product_code, products.image_path AS product_image_path, products.archived_at IS NOT NULL AS archived, COALESCE(SUM(" + salesQuantityExpr + "),0) AS quantity_sold, COUNT(*) AS movement_count, COALESCE(SUM(" + salesCostExpr + "),0) AS cost_cents").
		Group("stock_movements.product_id, products.name, products.code, products.image_path, products.archived_at").
		Order("quantity_sold desc").Order("movement_count desc").Order("products.name asc").Order("stock_movements.product_id asc").
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
		Select("stock_movements.shop_id AS shop_id, shops.name AS shop_name, COALESCE(SUM(" + salesQuantityExpr + "),0) AS quantity_sold, COUNT(*) AS movement_count, COALESCE(SUM(" + salesCostExpr + "),0) AS cost_cents").
		Group("stock_movements.shop_id, shops.name").
		Order("quantity_sold desc").Order("movement_count desc").Order("shops.name asc").Order("stock_movements.shop_id asc").
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
		Joins("JOIN products ON products.id = stock_movements.product_id").
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
