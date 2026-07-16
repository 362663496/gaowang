package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type paginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func paginate(c *gin.Context, query *gorm.DB) (*gorm.DB, paginationMeta, error) {
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, paginationMeta{}, err
	}
	query = query.Session(&gorm.Session{})
	if c.Query("all") == "true" {
		totalPages := 0
		if total > 0 {
			totalPages = 1
		}
		return query, paginationMeta{Page: 1, PageSize: int(total), Total: total, TotalPages: totalPages}, nil
	}

	page := positiveQueryInt(c.Query("page"), 1)
	pageSizeRaw := c.Query("page_size")
	if pageSizeRaw == "" {
		pageSizeRaw = c.Query("limit")
	}
	pageSize := positiveQueryInt(pageSizeRaw, defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	meta := paginationMeta{Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}
	return query.Offset((page - 1) * pageSize).Limit(pageSize), meta, nil
}

func positiveQueryInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func writePage(c *gin.Context, items any, meta paginationMeta) {
	c.JSON(http.StatusOK, gin.H{"items": items, "pagination": meta})
}
