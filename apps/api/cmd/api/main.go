package main

import (
	"log/slog"
	"os"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/db"
	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/services"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", slog.Any("err", err))
		os.Exit(1)
	}

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		slog.Error("open database", slog.Any("err", err))
		os.Exit(1)
	}

	sqlDB, err := database.DB()
	if err != nil {
		slog.Error("get database handle", slog.Any("err", err))
		os.Exit(1)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			slog.Error("close database", slog.Any("err", err))
		}
	}()

	if err := db.Migrate(database); err != nil {
		slog.Error("migrate database", slog.Any("err", err))
		os.Exit(1)
	}

	if err := services.EnsureBootstrapAdmin(database, cfg.InitialAdminName, cfg.InitialAdminEmail, cfg.InitialAdminPassword); err != nil {
		// Never log the initial password value.
		slog.Error("bootstrap admin", slog.Any("err", err))
		os.Exit(1)
	}

	router := apihttp.NewRouter(cfg, database)
	if err := router.Run(cfg.APIAddr); err != nil {
		slog.Error("run api", slog.Any("err", err))
		os.Exit(1)
	}
}
