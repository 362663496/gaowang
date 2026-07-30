package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func Test_InboundRequest_accepts_request_without_price(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	body := `{"product_id":"2e6ecf8c-4291-4cd8-b96f-1d35bfca449f","quantity":1}`
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	// When
	var req inboundRequest
	err := context.ShouldBindJSON(&req)

	// Then
	if err != nil {
		t.Fatalf("bind inbound request: %v", err)
	}
	if req.ProductID == "" || req.Quantity != 1 {
		t.Fatalf("request = %+v, want product and quantity", req)
	}
}

func Test_OutboundRequest_accepts_request_without_price(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	body := `{"product_id":"2e6ecf8c-4291-4cd8-b96f-1d35bfca449f","shop_id":"fe6f64b5-36aa-4642-adaa-58cf20f979bc","quantity":1}`
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	// When
	var req outboundRequest
	err := context.ShouldBindJSON(&req)

	// Then
	if err != nil {
		t.Fatalf("bind outbound request: %v", err)
	}
	if req.ProductID == "" || req.ShopID == "" || req.Quantity != 1 {
		t.Fatalf("request = %+v, want product, shop, and quantity", req)
	}
}
