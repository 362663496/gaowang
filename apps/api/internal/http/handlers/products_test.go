package handlers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
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
	"gorm.io/gorm"
)

func Test_ProductUpdate_preserves_and_replaces_image(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newProductTestDB(t)
	user := models.User{Name: "Admin", Email: "update@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	token := createSessionToken(t, db, user.ID)
	uploadDir := t.TempDir()
	oldImage := "old.png"
	if err := os.WriteFile(filepath.Join(uploadDir, oldImage), []byte("old"), 0o644); err != nil {
		t.Fatalf("create old image: %v", err)
	}
	product := models.Product{Name: "Tea", Code: "TEA", ImagePath: "/uploads/" + oldImage, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	router := apihttp.NewRouter(config.Config{AuthSecret: testAuthSecret, UploadDir: uploadDir}, db)
	fields := map[string]string{
		"name": "Green Tea", "code": "GREEN-TEA", "note": "updated",
		"default_purchase_cents": "120", "default_sale_cents": "250", "low_stock_threshold": "4",
	}

	response := productMultipartRequest(t, router, token, http.MethodPatch, "/api/v1/products/"+product.ID.String(), fields, "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var updated models.Product
	if err := db.First(&updated, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("load updated product: %v", err)
	}
	if updated.Name != "Green Tea" || updated.Code != "GREEN-TEA" || updated.Note != "updated" || updated.DefaultPurchaseCents != 120 || updated.DefaultSaleCents != 250 || updated.LowStockThreshold != 4 || !updated.Enabled || updated.ArchivedAt != nil || updated.ImagePath != product.ImagePath {
		t.Fatalf("updated product = %+v, want fields changed and image preserved", updated)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, oldImage)); err != nil {
		t.Fatalf("preserved image missing: %v", err)
	}

	response = productMultipartRequest(t, router, token, http.MethodPatch, "/api/v1/products/"+product.ID.String(), fields, "new.jpg", []byte("new"))
	if response.Code != http.StatusOK {
		t.Fatalf("replace image status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	if err := db.First(&updated, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("reload updated product: %v", err)
	}
	if updated.ImagePath == product.ImagePath {
		t.Fatalf("image path = %q, want replacement", updated.ImagePath)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, oldImage)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old image error = %v, want removed", err)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, filepath.Base(updated.ImagePath))); err != nil {
		t.Fatalf("replacement image missing: %v", err)
	}
	replacementPath := updated.ImagePath
	conflict := models.Product{Name: "Conflict", Code: "CONFLICT", Enabled: true}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatalf("create conflicting product: %v", err)
	}
	filesBeforeFailure, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatalf("read uploads before failed update: %v", err)
	}
	badFields := map[string]string{
		"name": "Bad Update", "code": conflict.Code, "note": "must not persist",
		"default_purchase_cents": "999", "default_sale_cents": "999", "low_stock_threshold": "9",
	}
	response = productMultipartRequest(t, router, token, http.MethodPatch, "/api/v1/products/"+product.ID.String(), badFields, "leak.png", []byte("leak"))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "商品编码已存在或数据无效") {
		t.Fatalf("failed update status/body = %d %s, want stable 400 message", response.Code, response.Body.String())
	}
	filesAfterFailure, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatalf("read uploads after failed update: %v", err)
	}
	if len(filesAfterFailure) != len(filesBeforeFailure) {
		t.Fatalf("upload count after failed update = %d, want %d", len(filesAfterFailure), len(filesBeforeFailure))
	}
	if err := db.First(&updated, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("reload after failed update: %v", err)
	}
	if updated.Code != "GREEN-TEA" || updated.ImagePath != replacementPath {
		t.Fatalf("product changed after failed update: %+v", updated)
	}
	var audits int64
	if err := db.Model(&models.AuditLog{}).Where("action = ? AND resource_id = ?", "product.update", product.ID).Count(&audits).Error; err != nil {
		t.Fatalf("count update audits: %v", err)
	}
	if audits != 2 {
		t.Fatalf("update audit count = %d, want 2", audits)
	}
}

func Test_ProductLifecycle_updates_deletes_archives_and_protects_stock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newProductTestDB(t)
	user := models.User{Name: "Admin", Email: "admin@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	token := createSessionToken(t, db, user.ID)
	uploadDir := t.TempDir()
	imageName := "tea.png"
	if err := os.WriteFile(filepath.Join(uploadDir, imageName), []byte("image"), 0o644); err != nil {
		t.Fatalf("create product image: %v", err)
	}
	product := models.Product{Name: "Tea", Code: "TEA", ImagePath: "/uploads/" + imageName, Enabled: true}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	router := apihttp.NewRouter(config.Config{AuthSecret: testAuthSecret, UploadDir: uploadDir}, db)

	response := productRequest(router, token, http.MethodPatch, "/api/v1/products/"+product.ID.String()+"/enabled", `{"enabled":false}`)
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
	response = productRequest(router, token, http.MethodPatch, "/api/v1/products/"+product.ID.String()+"/enabled", `{"enabled":true}`)
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

	response = productRequest(router, token, http.MethodDelete, "/api/v1/products/"+product.ID.String(), "")
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

	archivedImageName := "coffee.png"
	if err := os.WriteFile(filepath.Join(uploadDir, archivedImageName), []byte("image"), 0o644); err != nil {
		t.Fatalf("create archived product image: %v", err)
	}
	referenced := models.Product{Name: "Coffee", Code: "COFFEE", ImagePath: "/uploads/" + archivedImageName, Enabled: true}
	if err := db.Create(&referenced).Error; err != nil {
		t.Fatalf("create referenced product: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: referenced.ID}).Error; err != nil {
		t.Fatalf("create inventory snapshot: %v", err)
	}
	response = productRequest(router, token, http.MethodDelete, "/api/v1/products/"+referenced.ID.String(), "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("referenced delete = %d/%s, want 204", response.Code, response.Body.String())
	}
	var archived models.Product
	if err := db.First(&archived, "id = ?", referenced.ID).Error; err != nil {
		t.Fatalf("load archived product: %v", err)
	}
	if archived.ArchivedAt == nil || archived.Enabled {
		t.Fatalf("archived product = ArchivedAt %v/Enabled %t, want set/false", archived.ArchivedAt, archived.Enabled)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, archivedImageName)); err != nil {
		t.Fatalf("archived product image was removed: %v", err)
	}
	assertProductAudit(t, db, "product.archive", referenced.ID.String())
	assertProductListContains(t, router, token, "/api/v1/products", referenced.ID, false)
	assertProductListContains(t, router, token, "/api/v1/products?include_archived=true", referenced.ID, true)

	inventoryResponse := productRequest(router, token, http.MethodGet, "/api/v1/inventory", "")
	if inventoryResponse.Code != http.StatusOK || strings.Contains(inventoryResponse.Body.String(), referenced.ID.String()) {
		t.Fatalf("inventory response = %d/%s, want archived product hidden", inventoryResponse.Code, inventoryResponse.Body.String())
	}

	stocked := models.Product{Name: "Milk", Code: "MILK", Enabled: true}
	if err := db.Create(&stocked).Error; err != nil {
		t.Fatalf("create stocked product: %v", err)
	}
	if err := db.Create(&models.InventorySnapshot{ProductID: stocked.ID, Quantity: 2}).Error; err != nil {
		t.Fatalf("create stocked snapshot: %v", err)
	}
	response = productRequest(router, token, http.MethodDelete, "/api/v1/products/"+stocked.ID.String(), "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "PRODUCT_HAS_STOCK") {
		t.Fatalf("stocked delete = %d/%s, want 409 PRODUCT_HAS_STOCK", response.Code, response.Body.String())
	}
	if err := db.First(&stocked, "id = ?", stocked.ID).Error; err != nil || stocked.ArchivedAt != nil {
		t.Fatalf("stocked product changed after rejected delete: product=%+v error=%v", stocked, err)
	}
}

func newProductTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return openHandlerTestDB(t, &models.User{}, &models.Session{}, &models.StaffPermission{}, &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{}, &models.AuditLog{})
}

func productRequest(router http.Handler, token string, method string, path string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	withAuth(request, token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func productMultipartRequest(t *testing.T, router http.Handler, token string, method string, path string, fields map[string]string, filename string, image []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	if filename != "" {
		part, err := writer.CreateFormFile("image", filename)
		if err != nil {
			t.Fatalf("create image part: %v", err)
		}
		if _, err := part.Write(image); err != nil {
			t.Fatalf("write image: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	withAuth(request, token)
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

func assertProductListContains(t *testing.T, router http.Handler, token string, path string, productID uuid.UUID, want bool) {
	t.Helper()
	response := productRequest(router, token, http.MethodGet, path, "")
	if response.Code != http.StatusOK {
		t.Fatalf("list products status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Items []models.Product `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode products: %v", err)
	}
	found := false
	for _, item := range body.Items {
		if item.ID == productID {
			found = true
			break
		}
	}
	if found != want {
		t.Fatalf("product %s found = %t, want %t; body = %s", productID, found, want, response.Body.String())
	}
}
