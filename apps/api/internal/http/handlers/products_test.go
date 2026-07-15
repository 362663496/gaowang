package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gaowang/apps/api/internal/config"
	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_ProductLifecycle_updates_deletes_and_protects_history(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newProductTestDB(t)
	user := models.User{Name: "Admin", Email: "admin@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	uploadDir := t.TempDir()
	imageName := "tea.png"
	if err := os.WriteFile(filepath.Join(uploadDir, imageName), []byte("image"), 0o644); err != nil {
		t.Fatalf("create product image: %v", err)
	}
	product := models.Product{Name: "Tea", Code: "TEA", ImagePath: "/uploads/" + imageName, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	router := apihttp.NewRouter(config.Config{UploadDir: uploadDir}, db)

	response := productRequest(router, user.ID, http.MethodPatch, "/api/v1/products/"+product.ID.String()+"/enabled", `{"enabled":false}`)
	if response.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var updated models.Product
	if err := db.First(&updated, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("load updated product: %v", err)
	}
	if updated.Enabled {
		t.Fatal("product remained enabled after explicit false update")
	}
	assertProductAudit(t, db, "product.disable", product.ID.String())
	response = productRequest(router, user.ID, http.MethodPatch, "/api/v1/products/"+product.ID.String()+"/enabled", `{"enabled":true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if err := db.First(&updated, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("load enabled product: %v", err)
	}
	if !updated.Enabled {
		t.Fatal("product remained disabled after true update")
	}
	assertProductAudit(t, db, "product.enable", product.ID.String())

	response = productRequest(router, user.ID, http.MethodDelete, "/api/v1/products/"+product.ID.String(), "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d; body = %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if err := db.First(&models.Product{}, "id = ?", product.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted product lookup error = %v, want record not found", err)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, imageName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted product image error = %v, want not found", err)
	}
	assertProductAudit(t, db, "product.delete", product.ID.String())

	referenced := models.Product{Name: "Coffee", Code: "COFFEE", Enabled: true}
	if err := db.Create(&referenced).Error; err != nil {
		t.Fatalf("create referenced product: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: referenced.ID}).Error; err != nil {
		t.Fatalf("create inventory snapshot: %v", err)
	}
	response = productRequest(router, user.ID, http.MethodDelete, "/api/v1/products/"+referenced.ID.String(), "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "PRODUCT_IN_USE") {
		t.Fatalf("referenced delete = %d/%s, want 409 PRODUCT_IN_USE", response.Code, response.Body.String())
	}
	if err := db.First(&models.Product{}, "id = ?", referenced.ID).Error; err != nil {
		t.Fatalf("referenced product was deleted: %v", err)
	}
}

func newProductTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func productRequest(router http.Handler, userID uuid.UUID, method string, path string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Dev-User-ID", userID.String())
	request.Header.Set("X-Dev-Role", string(models.RoleAdmin))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertProductAudit(t *testing.T, db *gorm.DB, action string, productID string) {
	t.Helper()
	var count int64
	if err := db.Model(&models.AuditLog{}).Where("action = ? AND resource_id = ?", action, productID).Count(&count).Error; err != nil {
		t.Fatalf("count %s audit: %v", action, err)
	}
	if count != 1 {
		t.Fatalf("%s audit count = %d, want 1", action, count)
	}
}
