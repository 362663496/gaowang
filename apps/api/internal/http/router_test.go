package apihttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gaowang/apps/api/internal/config"

	"github.com/gin-gonic/gin"
)

func Test_NewRouter_returns_health_ok_when_requested(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	router := NewRouter(config.Config{}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %q, want ok", body["status"])
	}
}

func Test_NewRouter_serves_upload_files_from_configured_directory(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	uploadDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(uploadDir, "product.txt"), []byte("image-data"), 0644); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	router := NewRouter(config.Config{UploadDir: uploadDir}, nil)
	request := httptest.NewRequest(http.MethodGet, "/uploads/product.txt", nil)
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != "image-data" {
		t.Fatalf("body = %q, want image-data", response.Body.String())
	}
}
