package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func Test_InboundRequest_accepts_zero_unit_cents(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	body := `{"product_id":"2e6ecf8c-4291-4cd8-b96f-1d35bfca449f","quantity":1,"unit_cents":0}`
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
	if req.UnitCents == nil || *req.UnitCents != 0 {
		t.Fatalf("unit cents = %v, want 0", req.UnitCents)
	}
}

func Test_OutboundRequest_accepts_zero_and_camel_case_sale_unit_cents(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	body := `{"product_id":"2e6ecf8c-4291-4cd8-b96f-1d35bfca449f","shop_id":"fe6f64b5-36aa-4642-adaa-58cf20f979bc","quantity":1,"saleUnitCents":0}`
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
	if req.SaleUnitCents == nil || *req.SaleUnitCents != 0 {
		t.Fatalf("sale unit cents = %v, want 0", req.SaleUnitCents)
	}
}
