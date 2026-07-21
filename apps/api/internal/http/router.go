package apihttp

import (
	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/http/handlers"
	"gaowang/apps/api/internal/services"

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
	api.Use(RequireSameOrigin())
	api.GET("/health", handlers.Health)

	authHandler := handlers.AuthHandler{DB: database, Cfg: cfg}
	api.POST("/auth/login", authHandler.Login)

	protected := api.Group("")
	protected.Use(RequireAuth(database, cfg))
	mountProtected(protected, cfg, database)

	return router
}

func mountProtected(group *gin.RouterGroup, cfg config.Config, database *gorm.DB) {
	authHandler := handlers.AuthHandler{DB: database, Cfg: cfg}
	productHandler := handlers.ProductHandler{DB: database, Cfg: cfg}
	shopHandler := handlers.ShopHandler{DB: database}
	inventoryHandler := handlers.InventoryHandler{DB: database, Cfg: cfg}
	movementHandler := handlers.MovementHandler{DB: database}
	reportHandler := handlers.ReportHandler{DB: database}
	userHandler := handlers.UserHandler{DB: database}
	backupHandler := handlers.BackupHandler{DB: database, Cfg: cfg}
	settingHandler := handlers.SettingHandler{DB: database, Cfg: cfg}
	auditHandler := handlers.AuditHandler{DB: database}
	permissionHandler := handlers.PermissionHandler{DB: database}

	// Session-only account routes (no business permission required).
	group.GET("/auth/me", authHandler.Me)
	group.POST("/auth/logout", authHandler.Logout)
	group.POST("/auth/password", authHandler.ChangePassword)

	group.GET("/products", RequirePermission(services.PermProductRead), productHandler.List)
	group.POST("/products", RequirePermission(services.PermProductCreate), productHandler.Create)
	group.PATCH("/products/:id", RequirePermission(services.PermProductUpdate), productHandler.Update)
	group.PATCH("/products/:id/enabled", RequirePermission(services.PermProductToggle), productHandler.SetEnabled)
	group.DELETE("/products/:id", RequirePermission(services.PermProductDelete), productHandler.Delete)

	group.GET("/shops", RequirePermission(services.PermShopRead), shopHandler.List)
	group.POST("/shops", RequirePermission(services.PermShopCreate), shopHandler.Create)

	group.GET("/inventory", RequirePermission(services.PermInventoryRead), inventoryHandler.ListCurrent)
	group.GET("/inventory/export", RequirePermission(services.PermInventoryRead), inventoryHandler.ExportCurrent)
	group.POST("/inventory/inbound", RequirePermission(services.PermInventoryInbound), inventoryHandler.CreateInbound)
	group.POST("/inventory/sales-outbound", RequirePermission(services.PermInventorySalesOutbound), inventoryHandler.CreateSalesOutbound)
	group.POST("/inventory/adjustments", RequirePermission(services.PermInventoryAdjust), inventoryHandler.CreateAdjustment)

	group.GET("/stock-movements", RequirePermission(services.PermMovementRead), movementHandler.List)
	group.POST("/stock-movements/:id/preview", RequirePermission(services.PermMovementUpdate), movementHandler.PreviewUpdate)
	group.PATCH("/stock-movements/:id", RequirePermission(services.PermMovementUpdate), movementHandler.Update)

	group.GET("/reports/sales-summary", RequirePermission(services.PermReportSalesSummary), reportHandler.SalesSummary)
	group.GET("/reports/sales-trend", RequirePermission(services.PermReportSalesTrend), reportHandler.SalesTrend)
	group.GET("/reports/product-ranking", RequirePermission(services.PermReportProductRanking), reportHandler.ProductRanking)
	group.GET("/reports/shop-ranking", RequirePermission(services.PermReportShopRanking), reportHandler.ShopRanking)

	group.GET("/audit-logs", RequirePermission(services.PermAuditRead), auditHandler.List)
	group.GET("/backups/latest", RequirePermission(services.PermBackupRead), backupHandler.Latest)
	group.POST("/backups/run", RequirePermission(services.PermBackupRun), backupHandler.Run)
	group.GET("/settings", RequirePermission(services.PermSettingRead), settingHandler.Get)
	group.POST("/settings", RequirePermission(services.PermSettingUpdate), settingHandler.Update)
	group.GET("/users", RequirePermission(services.PermUserRead), userHandler.List)
	group.POST("/users", RequirePermission(services.PermUserCreate), userHandler.Create)

	group.GET("/permissions", RequirePermission(services.PermPermissionRead), permissionHandler.Get)
	group.PUT("/permissions", RequirePermission(services.PermPermissionUpdate), permissionHandler.Update)
}
