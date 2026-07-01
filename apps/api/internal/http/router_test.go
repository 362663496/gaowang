package apihttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
