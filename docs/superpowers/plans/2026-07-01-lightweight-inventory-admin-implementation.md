# Lightweight Inventory Admin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first release of a self-hosted inventory admin system with product images, users, shops, inbound stock, sales outbound, stock adjustments, inventory reports, backups, and backup email delivery.

**Architecture:** Use a small monorepo with a Next.js frontend and a Go Gin API. PostgreSQL stores all business data; stock-changing operations run through GORM transactions and row locks so stock movements and inventory snapshots stay consistent.

**Tech Stack:** Next.js, TypeScript, Tailwind CSS v4, shadcn/ui, Go, Gin, GORM, PostgreSQL, Docker Compose, Nginx, SMTP.

---

## Scope Note

The approved spec covers UI, backend, deployment, and backups. This plan keeps them in one first-release plan because the user-facing acceptance flow depends on a complete vertical slice: create product, upload image, receive stock, sell stock, view inventory/report, and verify backups. Implementation should still commit task-by-task.

## File Map

Create these top-level paths:

- `README.md`: local development and deployment entry points.
- `DESIGN.md`: project design system contract before frontend screens.
- `.env.example`: required environment variables.
- `Makefile`: repeatable commands for API, web, tests, and compose.
- `docker-compose.yml`: local and server service graph.
- `deploy/nginx/app.conf`: reverse proxy and uploads serving.
- `deploy/scripts/restore-db.sh`: documented database restore helper.
- `apps/api`: Go Gin API.
- `apps/web`: Next.js frontend.

Backend files:

- `apps/api/go.mod`: Go module dependencies.
- `apps/api/cmd/api/main.go`: API entry point.
- `apps/api/internal/config/config.go`: environment loading and validation.
- `apps/api/internal/db/db.go`: PostgreSQL and GORM initialization.
- `apps/api/internal/models/models.go`: GORM models and enums.
- `apps/api/internal/http/router.go`: Gin router and route registration.
- `apps/api/internal/http/middleware.go`: auth, role, and error helpers.
- `apps/api/internal/http/handlers/*.go`: HTTP handlers by module.
- `apps/api/internal/services/*.go`: business logic by module.
- `apps/api/internal/services/inventory_test.go`: stock transaction tests.
- `apps/api/internal/services/backup_test.go`: backup behavior tests.
- `apps/api/uploads/.gitkeep`: tracked marker so the upload directory exists in Git.

Frontend files:

- `apps/web/package.json`: frontend dependencies and scripts.
- `apps/web/src/app`: Next.js app routes.
- `apps/web/src/components/ui`: shadcn/ui generated primitives.
- `apps/web/src/components/layout`: shell, sidebar, topbar, drawer layout.
- `apps/web/src/features/*`: feature-specific API calls, forms, and tables.
- `apps/web/src/lib/api.ts`: typed API client.
- `apps/web/src/lib/format.ts`: money, quantity, and date format helpers.
- `apps/web/src/styles/globals.css`: Tailwind import and theme tokens.

## Milestone 1: Foundation

### Task 1: Repository Scaffold And Commands

**Files:**
- Create: `README.md`
- Create: `.env.example`
- Create: `Makefile`
- Create: `docker-compose.yml`
- Create: `apps/api/uploads/.gitkeep`

- [ ] **Step 1: Create the directory structure**

Run:

```bash
mkdir -p apps/api/cmd/api apps/api/internal/{config,db,http/handlers,models,services} apps/api/uploads apps/web deploy/nginx deploy/scripts
touch apps/api/uploads/.gitkeep
```

Expected: directories exist and `find apps -maxdepth 3 -type d` lists `apps/api` and `apps/web`.

- [ ] **Step 2: Add `.env.example`**

Create `.env.example`:

```dotenv
POSTGRES_DB=gaowang
POSTGRES_USER=gaowang
POSTGRES_PASSWORD=local-dev-password
DATABASE_URL=host=postgres user=gaowang password=local-dev-password dbname=gaowang port=5432 sslmode=disable TimeZone=Asia/Shanghai
API_ADDR=:8080
AUTH_SECRET=local-dev-auth-secret-at-least-32-bytes
UPLOAD_DIR=/app/uploads
BACKUP_DIR=/app/backups
BACKUP_RETENTION_DAYS=7
BACKUP_ATTACHMENT_LIMIT_MB=20
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=backup@example.com
SMTP_PASSWORD=example-smtp-password
SMTP_FROM=backup@example.com
SMTP_TO=owner@example.com
SMTP_TLS=starttls
NEXT_PUBLIC_API_BASE_URL=/api/v1
```

- [ ] **Step 3: Add `Makefile`**

Create `Makefile`:

```makefile
.PHONY: api-test api-run web-install web-dev compose-up compose-down

api-test:
	cd apps/api && go test ./...

api-run:
	cd apps/api && go run ./cmd/api

web-install:
	cd apps/web && npm install

web-dev:
	cd apps/web && npm run dev

compose-up:
	docker compose up --build

compose-down:
	docker compose down
```

- [ ] **Step 4: Add initial Docker Compose**

Create `docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-gaowang}
      POSTGRES_USER: ${POSTGRES_USER:-gaowang}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-local-dev-password}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  api:
    build:
      context: ./apps/api
    env_file: .env
    depends_on:
      - postgres
    volumes:
      - uploads:/app/uploads
      - backups:/app/backups
    ports:
      - "8080:8080"

  web:
    build:
      context: ./apps/web
    environment:
      NEXT_PUBLIC_API_BASE_URL: /api/v1
    depends_on:
      - api
    ports:
      - "3000:3000"

volumes:
  postgres_data:
  uploads:
  backups:
```

- [ ] **Step 5: Add README**

Create `README.md`:

```markdown
# Gaowang Inventory Admin

Self-hosted lightweight inventory admin system.

## Local Setup

1. Copy `.env.example` to `.env`.
2. Run `make compose-up`.
3. Open `http://localhost:3000`.

## Core Commands

- `make api-test`: run Go tests.
- `make api-run`: run Go API locally.
- `make web-install`: install web dependencies.
- `make web-dev`: run Next.js locally.
- `make compose-up`: run the stack.
- `make compose-down`: stop the stack.
```

- [ ] **Step 6: Commit**

Run:

```bash
git add README.md .env.example Makefile docker-compose.yml apps/api/uploads/.gitkeep
git commit -m "chore: scaffold project commands"
```

Expected: commit succeeds.

### Task 2: Backend Module, Config, Database, And Health Endpoint

**Files:**
- Create: `apps/api/go.mod`
- Create: `apps/api/Dockerfile`
- Create: `apps/api/cmd/api/main.go`
- Create: `apps/api/internal/config/config.go`
- Create: `apps/api/internal/db/db.go`
- Create: `apps/api/internal/http/router.go`
- Create: `apps/api/internal/http/handlers/health.go`

- [ ] **Step 1: Initialize Go module**

Run:

```bash
cd apps/api
go mod init gaowang/apps/api
go get github.com/gin-gonic/gin gorm.io/gorm gorm.io/driver/postgres golang.org/x/crypto/bcrypt github.com/google/uuid
```

Expected: `go.mod` and `go.sum` exist.

- [ ] **Step 2: Add config loader**

Create `apps/api/internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	APIAddr                 string
	DatabaseURL             string
	AuthSecret              string
	UploadDir               string
	BackupDir               string
	BackupRetentionDays     int
	BackupAttachmentLimitMB int
	SMTPHost                string
	SMTPPort                int
	SMTPUsername            string
	SMTPPassword            string
	SMTPFrom                string
	SMTPTo                  string
	SMTPTLS                 string
}

func Load() (Config, error) {
	cfg := Config{
		APIAddr:     getenv("API_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		AuthSecret:  os.Getenv("AUTH_SECRET"),
		UploadDir:   getenv("UPLOAD_DIR", "./uploads"),
		BackupDir:   getenv("BACKUP_DIR", "./backups"),
		SMTPHost:    os.Getenv("SMTP_HOST"),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("SMTP_FROM"),
		SMTPTo:       os.Getenv("SMTP_TO"),
		SMTPTLS:      getenv("SMTP_TLS", "starttls"),
	}

	var err error
	cfg.BackupRetentionDays, err = getenvInt("BACKUP_RETENTION_DAYS", 7)
	if err != nil {
		return Config{}, err
	}
	cfg.BackupAttachmentLimitMB, err = getenvInt("BACKUP_ATTACHMENT_LIMIT_MB", 20)
	if err != nil {
		return Config{}, err
	}
	cfg.SMTPPort, err = getenvInt("SMTP_PORT", 587)
	if err != nil {
		return Config{}, err
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.AuthSecret) < 32 {
		return Config{}, fmt.Errorf("AUTH_SECRET must be at least 32 bytes")
	}
	return cfg, nil
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, nil
}
```

- [ ] **Step 3: Add database connector**

Create `apps/api/internal/db/db.go`:

```go
package db

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(databaseURL string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		SkipDefaultTransaction: true,
	})
}
```

- [ ] **Step 4: Add router and health handler**

Create `apps/api/internal/http/router.go`:

```go
package http

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/http/handlers"
)

func NewRouter(cfg config.Config, database *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	api := router.Group("/api/v1")
	api.GET("/health", handlers.Health)

	return router
}
```

Create `apps/api/internal/http/handlers/health.go`:

```go
package handlers

import "github.com/gin-gonic/gin"

func Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}
```

- [ ] **Step 5: Add main**

Create `apps/api/cmd/api/main.go`:

```go
package main

import (
	"log"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/db"
	apihttp "gaowang/apps/api/internal/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	router := apihttp.NewRouter(cfg, database)
	if err := router.Run(cfg.APIAddr); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 6: Add API Dockerfile**

Create `apps/api/Dockerfile`:

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache postgresql-client ca-certificates
WORKDIR /app
COPY --from=build /out/api /app/api
EXPOSE 8080
CMD ["/app/api"]
```

- [ ] **Step 7: Verify API compiles**

Run:

```bash
cd apps/api
go test ./...
```

Expected: all packages compile and tests pass or report no test files.

- [ ] **Step 8: Commit**

Run:

```bash
git add apps/api
git commit -m "feat: add Go API foundation"
```

Expected: commit succeeds.

### Task 3: Backend Models And Auto-Migration

**Files:**
- Create: `apps/api/internal/models/models.go`
- Modify: `apps/api/internal/db/db.go`
- Modify: `apps/api/cmd/api/main.go`

- [ ] **Step 1: Add GORM models**

Create `apps/api/internal/models/models.go`:

```go
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleStaff Role = "staff"
)

type MovementType string

const (
	MovementInbound       MovementType = "inbound"
	MovementSalesOutbound MovementType = "sales_outbound"
	MovementAdjustment    MovementType = "adjustment"
)

type BackupStatus string

const (
	BackupStatusRunning BackupStatus = "running"
	BackupStatusSuccess BackupStatus = "success"
	BackupStatusFailed  BackupStatus = "failed"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name         string    `gorm:"size:80;not null"`
	Email        string    `gorm:"size:160;uniqueIndex;not null"`
	PasswordHash string    `gorm:"size:255;not null"`
	Role         Role      `gorm:"size:20;not null;index"`
	Enabled      bool      `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Shop struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name      string    `gorm:"size:120;uniqueIndex;not null"`
	Note      string    `gorm:"size:500;not null;default:''"`
	Enabled   bool      `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Product struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name                 string    `gorm:"size:160;not null;index"`
	Code                 string    `gorm:"size:80;uniqueIndex;not null"`
	ImagePath            string    `gorm:"size:500;not null;default:''"`
	DefaultPurchaseCents int64     `gorm:"not null;default:0"`
	DefaultSaleCents     int64     `gorm:"not null;default:0"`
	LowStockThreshold    int64     `gorm:"not null;default:0"`
	Note                 string    `gorm:"size:500;not null;default:''"`
	Enabled              bool      `gorm:"not null;default:true"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type InventorySnapshot struct {
	ProductID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	Product                Product   `gorm:"foreignKey:ProductID"`
	Quantity               int64     `gorm:"not null;default:0"`
	MovingAverageCostCents int64     `gorm:"not null;default:0"`
	InventoryValueCents    int64     `gorm:"not null;default:0"`
	UpdatedAt              time.Time
}

type StockMovement struct {
	ID                  uuid.UUID    `gorm:"type:uuid;primaryKey"`
	Type                MovementType `gorm:"size:40;not null;index"`
	ProductID           uuid.UUID    `gorm:"type:uuid;not null;index"`
	Product             Product      `gorm:"foreignKey:ProductID"`
	ShopID              *uuid.UUID   `gorm:"type:uuid;index"`
	Shop                *Shop        `gorm:"foreignKey:ShopID"`
	QuantityDelta       int64        `gorm:"not null"`
	PurchaseUnitCents   *int64
	SaleUnitCents       *int64
	CostUnitCents       int64 `gorm:"not null;default:0"`
	PurchaseAmountCents int64 `gorm:"not null;default:0"`
	RevenueCents        int64 `gorm:"not null;default:0"`
	CostAmountCents     int64 `gorm:"not null;default:0"`
	GrossProfitCents    int64 `gorm:"not null;default:0"`
	Reason              string `gorm:"size:500;not null;default:''"`
	OperatorID          uuid.UUID `gorm:"type:uuid;not null;index"`
	Operator            User      `gorm:"foreignKey:OperatorID"`
	CreatedAt           time.Time
}

type AuditLog struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ActorID      *uuid.UUID     `gorm:"type:uuid;index"`
	Action       string         `gorm:"size:120;not null;index"`
	ResourceType string         `gorm:"size:80;not null;index"`
	ResourceID   string         `gorm:"size:120;not null;default:''"`
	Metadata     datatypes.JSON `gorm:"type:jsonb"`
	IPAddress    string         `gorm:"size:80;not null;default:''"`
	CreatedAt    time.Time
}

type BackupJob struct {
	ID           uuid.UUID    `gorm:"type:uuid;primaryKey"`
	StartedAt    time.Time    `gorm:"not null"`
	FinishedAt   *time.Time
	Status       BackupStatus `gorm:"size:30;not null;index"`
	FilePath     string       `gorm:"size:500;not null;default:''"`
	FileSize     int64        `gorm:"not null;default:0"`
	EmailStatus  string       `gorm:"size:30;not null;default:'not_sent'"`
	Recipient    string       `gorm:"size:160;not null;default:''"`
	ErrorMessage string       `gorm:"size:1000;not null;default:''"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Setting struct {
	Key       string `gorm:"size:120;primaryKey"`
	Value     string `gorm:"type:text;not null"`
	UpdatedAt time.Time
}

func (u *User) BeforeCreate(tx *gorm.DB) error          { return assignUUID(&u.ID) }
func (s *Shop) BeforeCreate(tx *gorm.DB) error          { return assignUUID(&s.ID) }
func (p *Product) BeforeCreate(tx *gorm.DB) error       { return assignUUID(&p.ID) }
func (m *StockMovement) BeforeCreate(tx *gorm.DB) error { return assignUUID(&m.ID) }
func (a *AuditLog) BeforeCreate(tx *gorm.DB) error      { return assignUUID(&a.ID) }
func (b *BackupJob) BeforeCreate(tx *gorm.DB) error     { return assignUUID(&b.ID) }

func assignUUID(id *uuid.UUID) error {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
	return nil
}
```

- [ ] **Step 2: Add migration helper**

Modify `apps/api/internal/db/db.go`:

```go
package db

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/models"
)

func Open(databaseURL string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		SkipDefaultTransaction: true,
	})
}

func Migrate(database *gorm.DB) error {
	return database.AutoMigrate(
		&models.User{},
		&models.Shop{},
		&models.Product{},
		&models.InventorySnapshot{},
		&models.StockMovement{},
		&models.AuditLog{},
		&models.BackupJob{},
		&models.Setting{},
	)
}
```

- [ ] **Step 3: Call migration on startup**

Modify `apps/api/cmd/api/main.go` after opening the database:

```go
if err := db.Migrate(database); err != nil {
	log.Fatal(err)
}
```

- [ ] **Step 4: Run compile check**

Run:

```bash
cd apps/api
go test ./...
```

Expected: PASS. If GORM datatypes is missing, run `go get gorm.io/datatypes`.

- [ ] **Step 5: Commit**

Run:

```bash
git add apps/api
git commit -m "feat: add database models"
```

Expected: commit succeeds.

## Milestone 2: Backend Business Core

### Task 4: Auth, Users, And Role Middleware

**Files:**
- Create: `apps/api/internal/services/auth.go`
- Create: `apps/api/internal/http/middleware.go`
- Create: `apps/api/internal/http/handlers/auth.go`
- Create: `apps/api/internal/http/handlers/users.go`
- Modify: `apps/api/internal/http/router.go`

- [ ] **Step 1: Add auth service**

Create `apps/api/internal/services/auth.go`:

```go
package services

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/models"
)

type AuthService struct {
	DB *gorm.DB
}

func (s AuthService) CreateUser(name, email, password string, role models.Role) (models.User, error) {
	if name == "" || email == "" || len(password) < 8 {
		return models.User{}, errors.New("name, email, and an 8 character password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return models.User{}, err
	}
	user := models.User{Name: name, Email: email, PasswordHash: string(hash), Role: role, Enabled: true}
	return user, s.DB.Create(&user).Error
}

func (s AuthService) Verify(email, password string) (models.User, error) {
	var user models.User
	if err := s.DB.Where("email = ? AND enabled = ?", email, true).First(&user).Error; err != nil {
		return models.User{}, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return models.User{}, errors.New("invalid credentials")
	}
	return user, nil
}

type Session struct {
	UserID    uuid.UUID   `json:"user_id"`
	Role      models.Role `json:"role"`
	ExpiresAt time.Time   `json:"expires_at"`
}
```

- [ ] **Step 2: Add middleware context helpers**

Create `apps/api/internal/http/middleware.go`:

```go
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"gaowang/apps/api/internal/models"
)

const userIDKey = "userID"
const roleKey = "role"

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-Dev-User-ID")
		role := c.GetHeader("X-Dev-Role")
		parsed, err := uuid.Parse(userID)
		if err != nil || role == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "login required"}})
			c.Abort()
			return
		}
		c.Set(userIDKey, parsed)
		c.Set(roleKey, models.Role(role))
		c.Next()
	}
}

func RequireRole(roles ...models.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		current := CurrentRole(c)
		for _, role := range roles {
			if current == role {
				c.Next()
				return
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "FORBIDDEN", "message": "permission denied"}})
		c.Abort()
	}
}

func CurrentUserID(c *gin.Context) uuid.UUID {
	value, _ := c.Get(userIDKey)
	id, _ := value.(uuid.UUID)
	return id
}

func CurrentRole(c *gin.Context) models.Role {
	value, _ := c.Get(roleKey)
	role, _ := value.(models.Role)
	return role
}
```

- [ ] **Step 3: Add auth handlers**

Create `apps/api/internal/http/handlers/auth.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/services"
)

type AuthHandler struct {
	DB *gorm.DB
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

func (h AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": err.Error()}})
		return
	}
	user, err := services.AuthService{DB: h.DB}.Verify(req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "INVALID_CREDENTIALS", "message": "invalid email or password"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "role": user.Role}})
}
```

- [ ] **Step 4: Register auth routes**

Modify `apps/api/internal/http/router.go`:

```go
authHandler := handlers.AuthHandler{DB: database}
api.POST("/auth/login", authHandler.Login)
```

Protected route groups should be added as:

```go
protected := api.Group("")
protected.Use(RequireAuth())
admin := protected.Group("")
admin.Use(RequireRole(models.RoleAdmin))
```

- [ ] **Step 5: Run tests**

Run:

```bash
cd apps/api
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add apps/api
git commit -m "feat: add auth foundation"
```

Expected: commit succeeds.

### Task 5: Products, Shops, And Image Upload

**Files:**
- Create: `apps/api/internal/http/handlers/products.go`
- Create: `apps/api/internal/http/handlers/shops.go`
- Create: `apps/api/internal/services/uploads.go`
- Modify: `apps/api/internal/http/router.go`

- [ ] **Step 1: Add upload service**

Create `apps/api/internal/services/uploads.go`:

```go
package services

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

var allowedImageExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

func SaveProductImage(uploadDir string, file multipart.File, header *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedImageExt[ext] {
		return "", fmt.Errorf("image type must be jpg, png, or webp")
	}
	if header.Size > 5*1024*1024 {
		return "", fmt.Errorf("image must be 5MB or smaller")
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", err
	}
	name := uuid.New().String() + ext
	fullPath := filepath.Join(uploadDir, name)
	dst, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}
	return "/uploads/" + name, nil
}
```

- [ ] **Step 2: Add product handler**

Create `apps/api/internal/http/handlers/products.go` with list and create endpoints:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
)

type ProductHandler struct {
	DB  *gorm.DB
	Cfg config.Config
}

func (h ProductHandler) List(c *gin.Context) {
	var products []models.Product
	query := h.DB.Order("created_at desc")
	if keyword := c.Query("q"); keyword != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := query.Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": "failed to list products"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": products})
}

func (h ProductHandler) Create(c *gin.Context) {
	var product models.Product
	product.Name = c.PostForm("name")
	product.Code = c.PostForm("code")
	product.Note = c.PostForm("note")
	product.Enabled = true
	if product.Name == "" || product.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": "name and code are required"}})
		return
	}
	file, header, err := c.Request.FormFile("image")
	if err == nil {
		defer file.Close()
		path, err := services.SaveProductImage(h.Cfg.UploadDir, file, header)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "UPLOAD_INVALID", "message": err.Error()}})
			return
		}
		product.ImagePath = path
	}
	if err := h.DB.Create(&product).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "PRODUCT_CREATE_FAILED", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": product})
}
```

- [ ] **Step 3: Add shop handler**

Create `apps/api/internal/http/handlers/shops.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/models"
)

type ShopHandler struct {
	DB *gorm.DB
}

type createShopRequest struct {
	Name string `json:"name" binding:"required,min=1,max=120"`
	Note string `json:"note" binding:"max=500"`
}

func (h ShopHandler) List(c *gin.Context) {
	var shops []models.Shop
	if err := h.DB.Order("created_at desc").Find(&shops).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": "failed to list shops"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": shops})
}

func (h ShopHandler) Create(c *gin.Context) {
	var req createShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": err.Error()}})
		return
	}
	shop := models.Shop{Name: req.Name, Note: req.Note, Enabled: true}
	if err := h.DB.Create(&shop).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "SHOP_CREATE_FAILED", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": shop})
}
```

- [ ] **Step 4: Register routes**

Modify `apps/api/internal/http/router.go`:

```go
productHandler := handlers.ProductHandler{DB: database, Cfg: cfg}
shopHandler := handlers.ShopHandler{DB: database}
protected.GET("/products", productHandler.List)
protected.POST("/products", productHandler.Create)
protected.GET("/shops", shopHandler.List)
protected.POST("/shops", shopHandler.Create)
```

- [ ] **Step 5: Run tests**

Run:

```bash
cd apps/api
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add apps/api
git commit -m "feat: add products shops and uploads"
```

Expected: commit succeeds.

### Task 6: Inventory Transaction Service

**Files:**
- Create: `apps/api/internal/services/inventory.go`
- Create: `apps/api/internal/services/inventory_test.go`
- Create: `apps/api/internal/http/handlers/inventory.go`
- Modify: `apps/api/internal/http/router.go`

- [ ] **Step 1: Write inventory tests**

Create `apps/api/internal/services/inventory_test.go`:

```go
package services

import (
	"testing"

	"github.com/google/uuid"
)

func TestMovingAverageInbound(t *testing.T) {
	gotQty, gotCost, gotValue := calculateInboundAverage(10, 100, 1000, 10, 200)
	if gotQty != 20 {
		t.Fatalf("quantity = %d, want 20", gotQty)
	}
	if gotCost != 150 {
		t.Fatalf("cost = %d, want 150", gotCost)
	}
	if gotValue != 3000 {
		t.Fatalf("value = %d, want 3000", gotValue)
	}
}

func TestOutboundRejectsInsufficientStock(t *testing.T) {
	err := validateOutbound(3, 4)
	if err == nil {
		t.Fatal("expected insufficient stock error")
	}
}

func TestOutboundAllowsExactStock(t *testing.T) {
	err := validateOutbound(4, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdjustmentRejectsZero(t *testing.T) {
	err := validateAdjustment(0, "stocktake")
	if err == nil {
		t.Fatal("expected zero adjustment error")
	}
}

func TestAdjustmentRequiresReason(t *testing.T) {
	err := validateAdjustment(1, "")
	if err == nil {
		t.Fatal("expected reason error")
	}
}

func testUUID() uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-000000000001")
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd apps/api
go test ./internal/services -run 'TestMovingAverageInbound|TestOutbound|TestAdjustment' -v
```

Expected: FAIL because calculation and validation functions do not exist.

- [ ] **Step 3: Add inventory service helpers and transaction methods**

Create `apps/api/internal/services/inventory.go`:

```go
package services

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gaowang/apps/api/internal/models"
)

var ErrInsufficientStock = errors.New("insufficient stock")

type InventoryService struct {
	DB *gorm.DB
}

func calculateInboundAverage(currentQty int64, currentCostCents int64, currentValueCents int64, inboundQty int64, inboundUnitCents int64) (int64, int64, int64) {
	newQty := currentQty + inboundQty
	newValue := currentValueCents + inboundQty*inboundUnitCents
	if newQty == 0 {
		return 0, 0, 0
	}
	return newQty, newValue / newQty, newValue
}

func validateOutbound(currentQty int64, outboundQty int64) error {
	if outboundQty <= 0 {
		return errors.New("quantity must be greater than zero")
	}
	if currentQty < outboundQty {
		return ErrInsufficientStock
	}
	return nil
}

func validateAdjustment(delta int64, reason string) error {
	if delta == 0 {
		return errors.New("adjustment quantity cannot be zero")
	}
	if reason == "" {
		return errors.New("adjustment reason is required")
	}
	return nil
}

type InboundInput struct {
	ProductID uuid.UUID
	Quantity int64
	UnitCents int64
	OperatorID uuid.UUID
}

func (s InventoryService) CreateInbound(input InboundInput) error {
	if input.Quantity <= 0 {
		return errors.New("quantity must be greater than zero")
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		newQty, newCost, newValue := calculateInboundAverage(snapshot.Quantity, snapshot.MovingAverageCostCents, snapshot.InventoryValueCents, input.Quantity, input.UnitCents)
		snapshot.Quantity = newQty
		snapshot.MovingAverageCostCents = newCost
		snapshot.InventoryValueCents = newValue
		if err := tx.Save(&snapshot).Error; err != nil {
			return err
		}
		unit := input.UnitCents
		movement := models.StockMovement{
			Type: models.MovementInbound, ProductID: input.ProductID, QuantityDelta: input.Quantity,
			PurchaseUnitCents: &unit, CostUnitCents: input.UnitCents, PurchaseAmountCents: input.Quantity * input.UnitCents,
			OperatorID: input.OperatorID,
		}
		return tx.Create(&movement).Error
	})
}

func lockSnapshot(tx *gorm.DB, productID uuid.UUID) (models.InventorySnapshot, error) {
	var snapshot models.InventorySnapshot
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("product_id = ?", productID).First(&snapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		snapshot = models.InventorySnapshot{ProductID: productID}
		return snapshot, tx.Create(&snapshot).Error
	}
	return snapshot, err
}
```

- [ ] **Step 4: Run service tests**

Run:

```bash
cd apps/api
go test ./internal/services -v
```

Expected: PASS.

- [ ] **Step 5: Add outbound and adjustment methods**

Append to `apps/api/internal/services/inventory.go`:

```go
type OutboundInput struct {
	ProductID uuid.UUID
	ShopID uuid.UUID
	Quantity int64
	SaleUnitCents int64
	OperatorID uuid.UUID
}

func (s InventoryService) CreateSalesOutbound(input OutboundInput) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		if err := validateOutbound(snapshot.Quantity, input.Quantity); err != nil {
			return err
		}
		costAmount := input.Quantity * snapshot.MovingAverageCostCents
		revenue := input.Quantity * input.SaleUnitCents
		snapshot.Quantity -= input.Quantity
		snapshot.InventoryValueCents -= costAmount
		if snapshot.Quantity == 0 {
			snapshot.MovingAverageCostCents = 0
			snapshot.InventoryValueCents = 0
		}
		if err := tx.Save(&snapshot).Error; err != nil {
			return err
		}
		sale := input.SaleUnitCents
		movement := models.StockMovement{
			Type: models.MovementSalesOutbound, ProductID: input.ProductID, ShopID: &input.ShopID,
			QuantityDelta: -input.Quantity, SaleUnitCents: &sale, CostUnitCents: snapshot.MovingAverageCostCents,
			RevenueCents: revenue, CostAmountCents: costAmount, GrossProfitCents: revenue - costAmount,
			OperatorID: input.OperatorID,
		}
		return tx.Create(&movement).Error
	})
}

type AdjustmentInput struct {
	ProductID uuid.UUID
	QuantityDelta int64
	Reason string
	OperatorID uuid.UUID
}

func (s InventoryService) CreateAdjustment(input AdjustmentInput) error {
	if err := validateAdjustment(input.QuantityDelta, input.Reason); err != nil {
		return err
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		snapshot, err := lockSnapshot(tx, input.ProductID)
		if err != nil {
			return err
		}
		if snapshot.Quantity+input.QuantityDelta < 0 {
			return ErrInsufficientStock
		}
		snapshot.Quantity += input.QuantityDelta
		snapshot.InventoryValueCents = snapshot.Quantity * snapshot.MovingAverageCostCents
		if err := tx.Save(&snapshot).Error; err != nil {
			return err
		}
		movement := models.StockMovement{
			Type: models.MovementAdjustment, ProductID: input.ProductID, QuantityDelta: input.QuantityDelta,
			CostUnitCents: snapshot.MovingAverageCostCents, CostAmountCents: input.QuantityDelta * snapshot.MovingAverageCostCents,
			Reason: input.Reason, OperatorID: input.OperatorID,
		}
		return tx.Create(&movement).Error
	})
}
```

- [ ] **Step 6: Add inventory handler**

Create `apps/api/internal/http/handlers/inventory.go`:

```go
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
)

type InventoryHandler struct{ DB *gorm.DB }

type inboundRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	Quantity int64 `json:"quantity" binding:"required,gt=0"`
	UnitCents int64 `json:"unit_cents" binding:"required,gte=0"`
}

func (h InventoryHandler) CreateInbound(c *gin.Context) {
	var req inboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": err.Error()}})
		return
	}
	productID, err := uuid.Parse(req.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": "invalid product_id"}})
		return
	}
	err = services.InventoryService{DB: h.DB}.CreateInbound(services.InboundInput{
		ProductID: productID, Quantity: req.Quantity, UnitCents: req.UnitCents, OperatorID: apihttp.CurrentUserID(c),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrInsufficientStock) { status = http.StatusConflict }
		c.JSON(status, gin.H{"error": gin.H{"code": "STOCK_OPERATION_FAILED", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

func (h InventoryHandler) ListCurrent(c *gin.Context) {
	var items []models.InventorySnapshot
	if err := h.DB.Preload("Product").Order("updated_at desc").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": "failed to list inventory"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type outboundRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	ShopID string `json:"shop_id" binding:"required"`
	Quantity int64 `json:"quantity" binding:"required,gt=0"`
	SaleUnitCents int64 `json:"sale_unit_cents" binding:"required,gte=0"`
}

func (h InventoryHandler) CreateSalesOutbound(c *gin.Context) {
	var req outboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": err.Error()}})
		return
	}
	productID, err := uuid.Parse(req.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": "invalid product_id"}})
		return
	}
	shopID, err := uuid.Parse(req.ShopID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": "invalid shop_id"}})
		return
	}
	err = services.InventoryService{DB: h.DB}.CreateSalesOutbound(services.OutboundInput{
		ProductID: productID, ShopID: shopID, Quantity: req.Quantity, SaleUnitCents: req.SaleUnitCents, OperatorID: apihttp.CurrentUserID(c),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrInsufficientStock) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": gin.H{"code": "STOCK_OPERATION_FAILED", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

type adjustmentRequest struct {
	ProductID string `json:"product_id" binding:"required"`
	QuantityDelta int64 `json:"quantity_delta" binding:"required"`
	Reason string `json:"reason" binding:"required,min=1,max=500"`
}

func (h InventoryHandler) CreateAdjustment(c *gin.Context) {
	var req adjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": err.Error()}})
		return
	}
	productID, err := uuid.Parse(req.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": "invalid product_id"}})
		return
	}
	err = services.InventoryService{DB: h.DB}.CreateAdjustment(services.AdjustmentInput{
		ProductID: productID, QuantityDelta: req.QuantityDelta, Reason: req.Reason, OperatorID: apihttp.CurrentUserID(c),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrInsufficientStock) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": gin.H{"code": "STOCK_OPERATION_FAILED", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}
```

- [ ] **Step 7: Register inventory routes**

Modify `apps/api/internal/http/router.go`:

```go
inventoryHandler := handlers.InventoryHandler{DB: database}
protected.GET("/inventory", inventoryHandler.ListCurrent)
protected.POST("/inventory/inbound", inventoryHandler.CreateInbound)
protected.POST("/inventory/sales-outbound", inventoryHandler.CreateSalesOutbound)
protected.POST("/inventory/adjustments", inventoryHandler.CreateAdjustment)
```

- [ ] **Step 8: Run all API tests**

Run:

```bash
cd apps/api
go test ./...
```

Expected: PASS.

- [ ] **Step 9: Commit**

Run:

```bash
git add apps/api
git commit -m "feat: add inventory transactions"
```

Expected: commit succeeds.

### Task 7: Movements And Reports API

**Files:**
- Create: `apps/api/internal/http/handlers/movements.go`
- Create: `apps/api/internal/http/handlers/reports.go`
- Modify: `apps/api/internal/http/router.go`

- [ ] **Step 1: Add movement list handler**

Create `apps/api/internal/http/handlers/movements.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/models"
)

type MovementHandler struct{ DB *gorm.DB }

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": "failed to list movements"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
```

- [ ] **Step 2: Add report handler**

Create `apps/api/internal/http/handlers/reports.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct{ DB *gorm.DB }

type salesSummary struct {
	RevenueCents int64 `json:"revenue_cents"`
	CostCents int64 `json:"cost_cents"`
	GrossProfitCents int64 `json:"gross_profit_cents"`
}

func (h ReportHandler) SalesSummary(c *gin.Context) {
	var summary salesSummary
	err := h.DB.Table("stock_movements").
		Select("COALESCE(SUM(revenue_cents),0) AS revenue_cents, COALESCE(SUM(cost_amount_cents),0) AS cost_cents, COALESCE(SUM(gross_profit_cents),0) AS gross_profit_cents").
		Where("type = ?", "sales_outbound").
		Scan(&summary).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": "failed to load report"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary})
}
```

- [ ] **Step 3: Register routes**

Modify `apps/api/internal/http/router.go`:

```go
movementHandler := handlers.MovementHandler{DB: database}
reportHandler := handlers.ReportHandler{DB: database}
protected.GET("/stock-movements", movementHandler.List)
protected.GET("/reports/sales-summary", reportHandler.SalesSummary)
```

- [ ] **Step 4: Run API tests**

Run:

```bash
cd apps/api
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add apps/api
git commit -m "feat: add movements and reports API"
```

Expected: commit succeeds.

### Task 8: Backup And Email Service

**Files:**
- Create: `apps/api/internal/services/backup.go`
- Create: `apps/api/internal/services/backup_test.go`
- Create: `apps/api/internal/http/handlers/backups.go`
- Modify: `apps/api/internal/http/router.go`

- [ ] **Step 1: Add backup tests for failure-safe behavior**

Create `apps/api/internal/services/backup_test.go`:

```go
package services

import "testing"

func TestShouldAttachBackup(t *testing.T) {
	if !shouldAttachBackup(10*1024*1024, 20) {
		t.Fatal("10MB should fit a 20MB limit")
	}
	if shouldAttachBackup(21*1024*1024, 20) {
		t.Fatal("21MB should not fit a 20MB limit")
	}
}

func TestBackupFilename(t *testing.T) {
	name := backupFilename("20260701-120000")
	if name != "gaowang-20260701-120000.sql.gz" {
		t.Fatalf("filename = %q", name)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd apps/api
go test ./internal/services -run 'TestShouldAttachBackup|TestBackupFilename' -v
```

Expected: FAIL because backup helpers do not exist.

- [ ] **Step 3: Add backup service**

Create `apps/api/internal/services/backup.go`:

```go
package services

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type BackupService struct {
	DatabaseURL string
	BackupDir string
	AttachmentLimitMB int
}

func shouldAttachBackup(fileSize int64, limitMB int) bool {
	return fileSize <= int64(limitMB)*1024*1024
}

func backupFilename(stamp string) string {
	return fmt.Sprintf("gaowang-%s.sql.gz", stamp)
}

func (s BackupService) Run(ctx context.Context) (string, int64, error) {
	if err := os.MkdirAll(s.BackupDir, 0755); err != nil {
		return "", 0, err
	}
	stamp := time.Now().Format("20060102-150405")
	sqlPath := filepath.Join(s.BackupDir, fmt.Sprintf("gaowang-%s.sql", stamp))
	gzPath := filepath.Join(s.BackupDir, backupFilename(stamp))
	cmd := exec.CommandContext(ctx, "pg_dump", s.DatabaseURL, "-f", sqlPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("pg_dump failed: %s: %w", string(output), err)
	}
	if err := gzipFile(sqlPath, gzPath); err != nil {
		return "", 0, err
	}
	_ = os.Remove(sqlPath)
	info, err := os.Stat(gzPath)
	if err != nil {
		return "", 0, err
	}
	return gzPath, info.Size(), nil
}

func gzipFile(src string, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer output.Close()
	writer := gzip.NewWriter(output)
	defer writer.Close()
	_, err = io.Copy(writer, input)
	return err
}
```

- [ ] **Step 4: Add email dependency and mail function**

Run:

```bash
cd apps/api
go get github.com/shyim/go-mailer github.com/shyim/go-mailer/transport
```

Modify the import block in `apps/api/internal/services/backup.go` to include:

```go
	gomailer "github.com/shyim/go-mailer"
	"github.com/shyim/go-mailer/transport"
	_ "github.com/shyim/go-mailer/transport/smtp"
```

Append this code to `apps/api/internal/services/backup.go`:

```go
type MailConfig struct {
	DSN string
	From string
	To string
}

func SendBackupMail(ctx context.Context, cfg MailConfig, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	message := gomailer.NewMessage().
		SetFrom(gomailer.MustAddress(cfg.From, "Gaowang Backup")).
		SetTo(gomailer.MustAddress(cfg.To, "Backup Recipient")).
		SetSubject("Gaowang database backup").
		SetText([]byte("Database backup is attached.")).
		Attach(gomailer.Attachment{
			Filename: filepath.Base(filePath),
			ContentType: "application/gzip",
			Data: data,
		})

	mailTransport, err := transport.FromDSN(cfg.DSN, transport.Deps{})
	if err != nil {
		return err
	}
	mailer := gomailer.NewMailer(mailTransport)
	defer mailer.Close()
	return mailer.Send(ctx, message, nil)
}
```

- [ ] **Step 5: Add backup handler**

Create `apps/api/internal/http/handlers/backups.go`:

```go
package handlers

import (
	"context"
	"fmt"
	"net/url"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
)

type BackupHandler struct {
	DB *gorm.DB
	Cfg config.Config
}

func (h BackupHandler) Run(c *gin.Context) {
	started := time.Now()
	job := models.BackupJob{StartedAt: started, Status: models.BackupStatusRunning, Recipient: h.Cfg.SMTPTo}
	_ = h.DB.Create(&job).Error
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	path, size, err := services.BackupService{DatabaseURL: h.Cfg.DatabaseURL, BackupDir: h.Cfg.BackupDir, AttachmentLimitMB: h.Cfg.BackupAttachmentLimitMB}.Run(ctx)
	finished := time.Now()
	job.FinishedAt = &finished
	job.FilePath = path
	job.FileSize = size
	if err != nil {
		job.Status = models.BackupStatusFailed
		job.ErrorMessage = err.Error()
		_ = h.DB.Save(&job).Error
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "BACKUP_FAILED", "message": err.Error()}})
		return
	}
	job.Status = models.BackupStatusSuccess
	job.EmailStatus = "not_configured"
	if h.Cfg.SMTPHost != "" && h.Cfg.SMTPTo != "" {
		if size > int64(h.Cfg.BackupAttachmentLimitMB)*1024*1024 {
			job.EmailStatus = "skipped_oversize"
		} else {
			scheme := "smtp"
			if h.Cfg.SMTPTLS == "smtps" {
				scheme = "smtps"
			}
			dsn := fmt.Sprintf("%s://%s:%s@%s:%d", scheme, url.QueryEscape(h.Cfg.SMTPUsername), url.QueryEscape(h.Cfg.SMTPPassword), h.Cfg.SMTPHost, h.Cfg.SMTPPort)
			err = services.SendBackupMail(ctx, services.MailConfig{DSN: dsn, From: h.Cfg.SMTPFrom, To: h.Cfg.SMTPTo}, path)
			if err != nil {
				job.EmailStatus = "failed"
				job.ErrorMessage = err.Error()
			} else {
				job.EmailStatus = "sent"
			}
		}
	}
	_ = h.DB.Save(&job).Error
	c.JSON(http.StatusCreated, gin.H{"job": job})
}

func (h BackupHandler) Latest(c *gin.Context) {
	var job models.BackupJob
	if err := h.DB.Order("created_at desc").First(&job).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"job": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": job})
}
```

- [ ] **Step 6: Register backup routes**

Modify `apps/api/internal/http/router.go`:

```go
backupHandler := handlers.BackupHandler{DB: database, Cfg: cfg}
admin.POST("/backups/run", backupHandler.Run)
admin.GET("/backups/latest", backupHandler.Latest)
```

- [ ] **Step 7: Run tests**

Run:

```bash
cd apps/api
go test ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

Run:

```bash
git add apps/api
git commit -m "feat: add database backup service"
```

Expected: commit succeeds.

## Milestone 3: Frontend Experience

### Task 9: Next.js App And Design System

**Files:**
- Create: `DESIGN.md`
- Create: `apps/web/package.json`
- Create: `apps/web/Dockerfile`
- Create: `apps/web/src/app/layout.tsx`
- Create: `apps/web/src/app/page.tsx`
- Create: `apps/web/src/styles/globals.css`

- [ ] **Step 1: Scaffold Next.js**

Run:

```bash
npx create-next-app@latest apps/web --typescript --eslint --app --src-dir --import-alias "@/*"
cd apps/web
npm install
npx shadcn@latest init
npx shadcn@latest add button input label dialog sheet table dropdown-menu select textarea badge card toast
```

Expected: `apps/web/package.json`, `apps/web/src/app`, `components.json`, and `src/components/ui` exist.

- [ ] **Step 2: Add design system**

Create `DESIGN.md`:

```markdown
# Gaowang Inventory Admin Design System

## 1. Atmosphere & Identity

A precise desktop inventory command center. The signature is quiet contrast: a deep navigation rail, clean working canvas, compact data tables, and smooth drawers that keep operators in flow.

## 2. Color

| Role | Token | Light | Dark | Usage |
|------|-------|-------|------|-------|
| Surface/primary | --surface-primary | #f7f8fa | #08090a | Page background |
| Surface/panel | --surface-panel | #ffffff | #0f1011 | Main panels |
| Surface/elevated | --surface-elevated | #ffffff | #191a1b | Dialogs and drawers |
| Text/primary | --text-primary | #17181c | #f7f8f8 | Primary text |
| Text/secondary | --text-secondary | #5f6673 | #d0d6e0 | Secondary text |
| Text/muted | --text-muted | #8a8f98 | #8a8f98 | Metadata |
| Border/subtle | --border-subtle | #e6e8ee | rgba(255,255,255,0.08) | Default border |
| Accent/primary | --accent-primary | #5e6ad2 | #7170ff | Primary actions |
| Status/success | --status-success | #0f8a42 | #10b981 | Good status |
| Status/warning | --status-warning | #b76e00 | #f59e0b | Low stock |
| Status/error | --status-error | #c93434 | #ef4444 | Errors |

## 3. Typography

Primary font: Geist, system-ui, sans-serif.
Mono font: Geist Mono, ui-monospace, monospace.
Body text is 14px or larger. Tables use 14px with 20px line height. Page titles use 24px weight 600.

## 4. Spacing & Layout

Base unit is 4px. Desktop shell has a 240px sidebar, 64px topbar, and a main content max width of 1440px.

## 5. Components

Buttons, inputs, selects, dialogs, sheets, tables, badges, cards, toasts, and skeletons must include default, hover, active, focus, disabled, loading, empty, and error states.

## 6. Motion & Interaction

Use 120ms for button feedback and 200ms for drawers/dialogs. Animate only opacity and transform. Respect prefers-reduced-motion.

## 7. Depth & Surface

Use tonal shifts and subtle borders. Avoid heavy shadows. Tables use row hover and selected-row state instead of card nesting.
```

- [ ] **Step 3: Add frontend Dockerfile**

Create `apps/web/Dockerfile`:

```dockerfile
FROM node:22-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app ./
EXPOSE 3000
CMD ["npm", "start"]
```

- [ ] **Step 4: Run frontend checks**

Run:

```bash
cd apps/web
npm run lint
npm run build
```

Expected: both commands exit 0.

- [ ] **Step 5: Commit**

Run:

```bash
git add DESIGN.md apps/web
git commit -m "feat: add web app foundation"
```

Expected: commit succeeds.

### Task 10: Frontend API Client And App Shell

**Files:**
- Create: `apps/web/src/lib/api.ts`
- Create: `apps/web/src/lib/format.ts`
- Create: `apps/web/src/components/layout/app-shell.tsx`
- Modify: `apps/web/src/app/layout.tsx`
- Create: `apps/web/src/app/(app)/dashboard/page.tsx`

- [ ] **Step 1: Add typed API client**

Create `apps/web/src/lib/api.ts`:

```ts
const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api/v1";

export async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, { credentials: "include" });
  if (!response.ok) {
    throw new Error(await readError(response));
  }
  return response.json() as Promise<T>;
}

export async function apiPost<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(await readError(response));
  }
  return response.json() as Promise<T>;
}

async function readError(response: Response): Promise<string> {
  try {
    const data = await response.json();
    return data?.error?.message ?? `Request failed with ${response.status}`;
  } catch {
    return `Request failed with ${response.status}`;
  }
}
```

- [ ] **Step 2: Add format helpers**

Create `apps/web/src/lib/format.ts`:

```ts
export function formatMoney(cents: number): string {
  return new Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY" }).format(cents / 100);
}

export function formatQuantity(quantity: number): string {
  return new Intl.NumberFormat("zh-CN").format(quantity);
}

export function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}
```

- [ ] **Step 3: Add app shell**

Create `apps/web/src/components/layout/app-shell.tsx`:

```tsx
import Link from "next/link";

const navItems = [
  ["仪表盘", "/dashboard"],
  ["商品", "/products"],
  ["当前库存", "/inventory"],
  ["入库", "/inventory/inbound"],
  ["销售出库", "/inventory/sales-outbound"],
  ["流水记录", "/stock-movements"],
  ["报表", "/reports"],
  ["系统设置", "/settings"],
];

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-dvh bg-[var(--surface-primary)] text-[var(--text-primary)]">
      <aside className="fixed inset-y-0 left-0 w-60 border-r border-[var(--border-subtle)] bg-[#0f1011] px-4 py-5 text-[#d0d6e0]">
        <div className="mb-6 text-sm font-semibold text-white">Gaowang</div>
        <nav className="grid gap-1">
          {navItems.map(([label, href]) => (
            <Link className="rounded-md px-3 py-2 text-sm hover:bg-white/5 hover:text-white" href={href} key={href}>
              {label}
            </Link>
          ))}
        </nav>
      </aside>
      <main className="ml-60 min-h-dvh">
        <header className="sticky top-0 z-10 flex h-16 items-center border-b border-[var(--border-subtle)] bg-white/90 px-6 backdrop-blur">
          <div className="text-sm text-[var(--text-secondary)]">库存后台</div>
        </header>
        <div className="p-6">{children}</div>
      </main>
    </div>
  );
}
```

- [ ] **Step 4: Add dashboard page**

Create `apps/web/src/app/(app)/dashboard/page.tsx`:

```tsx
import { AppShell } from "@/components/layout/app-shell";

export default function DashboardPage() {
  return (
    <AppShell>
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-semibold">仪表盘</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">查看销售、毛利、低库存和最近操作。</p>
        </div>
        <div className="grid grid-cols-4 gap-4">
          {["今日销售额", "今日毛利", "低库存商品", "最近操作"].map((label) => (
            <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4" key={label}>
              <div className="text-sm text-[var(--text-secondary)]">{label}</div>
              <div className="mt-3 text-2xl font-semibold">0</div>
            </section>
          ))}
        </div>
      </div>
    </AppShell>
  );
}
```

- [ ] **Step 5: Run frontend checks**

Run:

```bash
cd apps/web
npm run lint
npm run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add apps/web
git commit -m "feat: add admin shell"
```

Expected: commit succeeds.

### Task 11: Product, Shop, Inventory, Movement, Report, And Backup Screens

**Files:**
- Create: `apps/web/src/features/products/products-table.tsx`
- Create: `apps/web/src/app/(app)/products/page.tsx`
- Create: `apps/web/src/app/(app)/inventory/page.tsx`
- Create: `apps/web/src/app/(app)/stock-movements/page.tsx`
- Create: `apps/web/src/app/(app)/reports/page.tsx`
- Create: `apps/web/src/app/(app)/settings/backups/page.tsx`

- [ ] **Step 1: Add product table component**

Create `apps/web/src/features/products/products-table.tsx`:

```tsx
"use client";

type Product = {
  id: string;
  name: string;
  code: string;
  imagePath: string;
  defaultPurchaseCents: number;
  defaultSaleCents: number;
  enabled: boolean;
};

export function ProductsTable({ products }: { products: Product[] }) {
  if (products.length === 0) {
    return <div className="rounded-lg border border-dashed p-10 text-center text-sm text-[var(--text-secondary)]">还没有商品。新增商品后会显示在这里。</div>;
  }
  return (
    <table className="w-full text-left text-sm">
      <thead className="text-[var(--text-secondary)]">
        <tr className="border-b">
          <th className="py-3">商品</th>
          <th>编码</th>
          <th>默认进货价</th>
          <th>默认销售价</th>
          <th>状态</th>
        </tr>
      </thead>
      <tbody>
        {products.map((product) => (
          <tr className="border-b hover:bg-black/[0.02]" key={product.id}>
            <td className="py-3 font-medium">{product.name}</td>
            <td>{product.code}</td>
            <td>{product.defaultPurchaseCents}</td>
            <td>{product.defaultSaleCents}</td>
            <td>{product.enabled ? "启用" : "禁用"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
```

- [ ] **Step 2: Add product page**

Create `apps/web/src/app/(app)/products/page.tsx`:

```tsx
import { AppShell } from "@/components/layout/app-shell";
import { ProductsTable } from "@/features/products/products-table";

export default function ProductsPage() {
  return (
    <AppShell>
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold">商品</h1>
            <p className="text-sm text-[var(--text-secondary)]">管理商品图片、名称、编码和默认价格。</p>
          </div>
          <button className="rounded-md bg-[var(--accent-primary)] px-4 py-2 text-sm font-medium text-white">新增商品</button>
        </div>
        <ProductsTable products={[]} />
      </div>
    </AppShell>
  );
}
```

- [ ] **Step 3: Add inventory page**

Create `apps/web/src/app/(app)/inventory/page.tsx`:

```tsx
import { AppShell } from "@/components/layout/app-shell";

export default function InventoryPage() {
  return (
    <AppShell>
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-semibold">当前库存</h1>
          <p className="text-sm text-[var(--text-secondary)]">查看商品数量、移动平均成本、库存金额和低库存状态。</p>
        </div>
        <div className="rounded-lg border border-dashed p-10 text-center text-sm text-[var(--text-secondary)]">
          当前没有库存记录。完成入库后会显示库存数量和成本。
        </div>
      </div>
    </AppShell>
  );
}
```

- [ ] **Step 4: Add stock movements page**

Create `apps/web/src/app/(app)/stock-movements/page.tsx`:

```tsx
import { AppShell } from "@/components/layout/app-shell";

export default function StockMovementsPage() {
  return (
    <AppShell>
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-semibold">流水记录</h1>
          <p className="text-sm text-[var(--text-secondary)]">按商品、类型、店铺、操作人和时间筛选所有库存变化。</p>
        </div>
        <div className="rounded-lg border border-dashed p-10 text-center text-sm text-[var(--text-secondary)]">
          当前没有库存流水。入库、销售出库或调整后会显示记录。
        </div>
      </div>
    </AppShell>
  );
}
```

- [ ] **Step 5: Add reports page**

Create `apps/web/src/app/(app)/reports/page.tsx`:

```tsx
import { AppShell } from "@/components/layout/app-shell";

export default function ReportsPage() {
  return (
    <AppShell>
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-semibold">报表</h1>
          <p className="text-sm text-[var(--text-secondary)]">查看销售额、成本、毛利、库存金额和低库存列表。</p>
        </div>
        <div className="rounded-lg border border-dashed p-10 text-center text-sm text-[var(--text-secondary)]">
          当前没有可统计的销售记录。完成销售出库后会生成报表。
        </div>
      </div>
    </AppShell>
  );
}
```

- [ ] **Step 6: Add backups page**

Create `apps/web/src/app/(app)/settings/backups/page.tsx`:

```tsx
import { AppShell } from "@/components/layout/app-shell";

export default function BackupsPage() {
  return (
    <AppShell>
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-semibold">备份</h1>
          <p className="text-sm text-[var(--text-secondary)]">查看最近数据库备份和邮件发送状态。</p>
        </div>
        <div className="rounded-lg border border-dashed p-10 text-center text-sm text-[var(--text-secondary)]">
          当前没有备份记录。运行备份任务后会显示文件大小、状态和收件人。
        </div>
      </div>
    </AppShell>
  );
}
```

- [ ] **Step 7: Run checks**

Run:

```bash
cd apps/web
npm run lint
npm run build
```

Expected: PASS.

- [ ] **Step 8: Commit**

Run:

```bash
git add apps/web
git commit -m "feat: add admin pages"
```

Expected: commit succeeds.

## Milestone 4: Deployment And Verification

### Task 12: Nginx And Restore Script

**Files:**
- Create: `deploy/nginx/app.conf`
- Create: `deploy/scripts/restore-db.sh`
- Modify: `README.md`

- [ ] **Step 1: Add Nginx config**

Create `deploy/nginx/app.conf`:

```nginx
server {
    listen 80;
    server_name _;

    client_max_body_size 10m;

    location /api/ {
        proxy_pass http://api:8080/api/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /uploads/ {
        alias /uploads/;
        add_header Cache-Control "public, max-age=86400";
    }

    location / {
        proxy_pass http://web:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

- [ ] **Step 2: Add restore script**

Create `deploy/scripts/restore-db.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: deploy/scripts/restore-db.sh /path/to/backup.sql.gz"
  exit 1
fi

BACKUP_FILE="$1"
if [ ! -f "$BACKUP_FILE" ]; then
  echo "backup file not found: $BACKUP_FILE"
  exit 1
fi

gunzip -c "$BACKUP_FILE" | docker compose exec -T postgres psql -U "${POSTGRES_USER:-gaowang}" -d "${POSTGRES_DB:-gaowang}"
```

Run:

```bash
chmod +x deploy/scripts/restore-db.sh
```

- [ ] **Step 3: Document deploy and restore**

Append to `README.md`:

```markdown
## Deployment

1. Copy `.env.example` to `.env` and set production values.
2. Point DNS to the server.
3. Configure HTTPS in Nginx or a companion certificate service.
4. Run `docker compose up --build -d`.

## Restore

Run `deploy/scripts/restore-db.sh /path/to/gaowang-YYYYMMDD-HHMMSS.sql.gz` from the project root.
```

- [ ] **Step 4: Commit**

Run:

```bash
git add README.md deploy
git commit -m "chore: add deployment assets"
```

Expected: commit succeeds.

### Task 13: End-To-End Smoke Verification

**Files:**
- Modify only files required to fix failures found by this task.

- [ ] **Step 1: Run API tests**

Run:

```bash
make api-test
```

Expected: PASS.

- [ ] **Step 2: Run frontend build**

Run:

```bash
cd apps/web
npm run lint
npm run build
```

Expected: PASS.

- [ ] **Step 3: Start stack**

Run:

```bash
cp .env.example .env
make compose-up
```

Expected: `api`, `web`, and `postgres` start without crash loops.

- [ ] **Step 4: Check API health**

Run:

```bash
curl -s http://localhost:8080/api/v1/health
```

Expected:

```json
{"status":"ok"}
```

- [ ] **Step 5: Manual browser QA**

Open `http://localhost:3000` and verify:

- Sidebar renders.
- Dashboard renders.
- Products page renders.
- Inventory page renders.
- Stock movements page renders.
- Reports page renders.
- Backup page renders.
- No text overlaps at 1280px desktop width.

- [ ] **Step 6: Commit smoke fixes**

If files changed during smoke verification, run:

```bash
git add .
git commit -m "fix: pass smoke verification"
```

Expected: commit succeeds if there were fixes. If no files changed, record no commit for this step.

## Spec Coverage Checklist

- Single warehouse: Tasks 3, 6, and 11.
- No SKU: product model in Task 3 has no variant table.
- Manual sales outbound: Task 6.
- Product images: Task 5.
- Shops as sales attribution: Tasks 3, 5, 6.
- Admin and staff roles: Task 4.
- Immutable stock history: Task 6 creates append-only movements and no update/delete movement endpoints.
- Current inventory and low stock basis: Tasks 3, 6, 11.
- Basic sales/cost/gross profit reports: Task 7.
- Desktop-first modern UI: Tasks 9, 10, 11.
- Gin and GORM backend: Tasks 2, 3, 4, 5, 6, 7, 8.
- Docker Compose cloud deployment: Tasks 1 and 12.
- SQL backup and backup email: Task 8.
- Restore documentation: Task 12.
- Smoke verification: Task 13.
