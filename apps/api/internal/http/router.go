package apihttp

import (
	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/http/handlers"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(cfg config.Config, database *gorm.DB) *gin.Engine {
	_ = cfg
	_ = database

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	api := router.Group("/api/v1")
	api.GET("/health", handlers.Health)

	return router
}
