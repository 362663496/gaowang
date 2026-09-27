package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func Test_InventoryList_filters_quantity_range_and_sorts_before_pagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Product{}, &models.InventorySnapshot{})...)
	user := models.User{Name: "Admin", Email: "inventory-filter@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	token := createSessionToken(t, db, user.ID)
	archivedAt := time.Now()
	products := []models.Product{
		{Name: "Alpha", Code: "A", DefaultPurchaseCents: 100, Enabled: true},
		{Name: "Alpha", Code: "B", DefaultPurchaseCents: 100, Enabled: true},
		{Name: "Beta", Code: "C", DefaultPurchaseCents: 100, Enabled: true},
		{Name: "Hidden", Code: "Z", DefaultPurchaseCents: 100, Enabled: false, ArchivedAt: &archivedAt},
	}
	for index := range products {
		if err := db.Create(&products[index]).Error; err != nil {
			t.Fatalf("create product: %v", err)
		}
	}
	snapshots := []models.InventorySnapshot{
		{ProductID: products[0].ID, Quantity: 4},
		{ProductID: products[1].ID, Quantity: 2},
		{ProductID: products[2].ID, Quantity: 9},
		{ProductID: products[3].ID, Quantity: 3},
	}
	for _, snapshot := range snapshots {
		if err := db.Create(&snapshot).Error; err != nil {
			t.Fatalf("create snapshot: %v", err)
		}
	}
	router := apihttp.NewRouter(testConfig(), db)

	first := inventoryCodes(t, router, token, "/api/v1/inventory?min_quantity=2&max_quantity=4&sort=quantity&order=desc&page_size=1&page=1")
	if len(first.Items) != 1 || first.Items[0] != "A" || first.Total != 2 {
		t.Fatalf("first page = %+v, want code A of 2", first)
	}
	second := inventoryCodes(t, router, token, "/api/v1/inventory?min_quantity=2&max_quantity=4&sort=quantity&order=desc&page_size=1&page=2")
	if len(second.Items) != 1 || second.Items[0] != "B" || second.Total != 2 {
		t.Fatalf("second page = %+v, want code B of 2", second)
	}
	byCode := inventoryCodes(t, router, token, "/api/v1/inventory?min_quantity=0&sort=code&order=desc")
	if len(byCode.Items) != 3 || byCode.Items[0] != "C" || byCode.Items[2] != "A" {
		t.Fatalf("code sort = %+v, want C, B, A and archived hidden", byCode)
	}

	for _, path := range []string{
		"/api/v1/inventory?min_quantity=nope",
		"/api/v1/inventory?min_quantity=4&max_quantity=1",
		"/api/v1/inventory?sort=price",
		"/api/v1/inventory?order=up",
	} {
		response := doJSON(t, router, http.MethodGet, path, token, nil)
		if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"VALIDATION"`)) {
			t.Fatalf("%s status = %d body = %s, want 400 VALIDATION", path, response.Code, response.Body.String())
		}
	}

	export := doJSON(t, router, http.MethodGet, "/api/v1/inventory/export?min_quantity=2&max_quantity=4&sort=quantity&order=asc", token, nil)
	if export.Code != http.StatusOK {
		t.Fatalf("export status = %d body = %s", export.Code, export.Body.String())
	}
	book, err := excelize.OpenReader(bytes.NewReader(export.Body.Bytes()))
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	t.Cleanup(func() { _ = book.Close() })
	rows, err := book.GetRows("当前库存")
	if err != nil {
		t.Fatalf("read export rows: %v", err)
	}
	if len(rows) != 3 || rows[1][2] != "B" || rows[2][2] != "A" {
		t.Fatalf("export rows = %+v, want codes B then A", rows)
	}
}

type inventoryCodePage struct {
	Items []string
	Total int64
}

func inventoryCodes(t *testing.T, router http.Handler, token string, path string) inventoryCodePage {
	t.Helper()
	response := doJSON(t, router, http.MethodGet, path, token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("%s status = %d body = %s", path, response.Code, response.Body.String())
	}
	var body struct {
		Items []struct {
			Product struct {
				Code string
			}
		} `json:"items"`
		Pagination struct {
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	codes := make([]string, 0, len(body.Items))
	for _, item := range body.Items {
		codes = append(codes, item.Product.Code)
	}
	return inventoryCodePage{Items: codes, Total: body.Pagination.Total}
}
