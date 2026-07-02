package apihttp

import (
	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/http/handlers"
	"gaowang/apps/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(cfg config.Config, database *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	if cfg.UploadDir != "" {
		router.Static("/uploads", cfg.UploadDir)
	}

	api := router.Group("/api/v1")
	api.GET("/health", handlers.Health)
	authHandler := handlers.AuthHandler{DB: database}
	api.POST("/auth/login", authHandler.Login)

	protected := api.Group("")
	protected.Use(RequireAuth())
	mountProtected(protected, cfg, database)

	admin := protected.Group("")
	admin.Use(RequireRole(models.RoleAdmin))
	mountAdmin(admin, cfg, database)

	return router
}

func mountProtected(group *gin.RouterGroup, cfg config.Config, database *gorm.DB) {
	productHandler := handlers.ProductHandler{DB: database, Cfg: cfg}
	shopHandler := handlers.ShopHandler{DB: database}
	inventoryHandler := handlers.InventoryHandler{DB: database}
	movementHandler := handlers.MovementHandler{DB: database}
	reportHandler := handlers.ReportHandler{DB: database}

	group.GET("/products", productHandler.List)
	group.POST("/products", productHandler.Create)
	group.GET("/shops", shopHandler.List)
	group.POST("/shops", shopHandler.Create)
	group.GET("/inventory", inventoryHandler.ListCurrent)
	group.POST("/inventory/inbound", inventoryHandler.CreateInbound)
	group.POST("/inventory/sales-outbound", inventoryHandler.CreateSalesOutbound)
	group.POST("/inventory/adjustments", inventoryHandler.CreateAdjustment)
	group.GET("/stock-movements", movementHandler.List)
	group.GET("/reports/sales-summary", reportHandler.SalesSummary)
}

func mountAdmin(group *gin.RouterGroup, cfg config.Config, database *gorm.DB) {
	userHandler := handlers.UserHandler{DB: database}
	backupHandler := handlers.BackupHandler{DB: database, Cfg: cfg}

	group.GET("/users", userHandler.List)
	group.POST("/users", userHandler.Create)
	group.GET("/backups/latest", backupHandler.Latest)
	group.POST("/backups/run", backupHandler.Run)
}
