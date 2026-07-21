package handlers_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func Test_InventoryExport_embeds_images_and_respects_low_stock_filter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Shop{}, &models.Product{}, &models.InventorySnapshot{}, &models.StockMovement{})...)
	user := models.User{Name: "Admin", Email: "export@example.com", PasswordHash: "hash", Role: models.RoleAdmin, Enabled: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	token := createSessionToken(t, db, user.ID)

	uploadDir := t.TempDir()
	imageName := "tea.png"
	if err := writeSolidPNG(filepath.Join(uploadDir, imageName), 40, 30, color.RGBA{R: 20, G: 180, B: 40, A: 255}); err != nil {
		t.Fatalf("write png: %v", err)
	}

	withImage := models.Product{
		Name: "绿茶", Code: "TEA-GREEN", ImagePath: "/uploads/" + imageName,
		LowStockThreshold: 5, Enabled: true,
	}
	lowStock := models.Product{
		Name: "红茶", Code: "TEA-RED", ImagePath: "/uploads/missing.png",
		LowStockThreshold: 10, Enabled: true,
	}
	archived := models.Product{
		Name: "归档茶", Code: "TEA-ARCH", Enabled: false,
	}
	now := time.Now()
	archived.ArchivedAt = &now
	for _, product := range []*models.Product{&withImage, &lowStock, &archived} {
		if err := db.Create(product).Error; err != nil {
			t.Fatalf("create product %s: %v", product.Code, err)
		}
	}

	snapshots := []models.InventorySnapshot{
		{ProductID: withImage.ID, Quantity: 20, MovingAverageCostCents: 150, InventoryValueCents: 3000, UpdatedAt: now.Add(-time.Hour)},
		{ProductID: lowStock.ID, Quantity: 3, MovingAverageCostCents: 200, InventoryValueCents: 600, UpdatedAt: now},
		{ProductID: archived.ID, Quantity: 1, MovingAverageCostCents: 100, InventoryValueCents: 100, UpdatedAt: now},
	}
	for _, snapshot := range snapshots {
		if err := db.Create(&snapshot).Error; err != nil {
			t.Fatalf("create snapshot: %v", err)
		}
	}

	cfg := testConfig()
	cfg.UploadDir = uploadDir
	router := apihttp.NewRouter(cfg, db)

	// When exporting all active inventory
	allResponse := doJSON(t, router, http.MethodGet, "/api/v1/inventory/export", token, nil)
	if allResponse.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200; body = %s", allResponse.Code, allResponse.Body.String())
	}
	contentType := allResponse.Header().Get("Content-Type")
	if !strings.Contains(contentType, "spreadsheetml") {
		t.Fatalf("content type = %q, want spreadsheetml", contentType)
	}
	if !strings.Contains(allResponse.Header().Get("Content-Disposition"), "inventory-") {
		t.Fatalf("content disposition = %q, want inventory- filename", allResponse.Header().Get("Content-Disposition"))
	}

	allBook, err := excelize.OpenReader(bytes.NewReader(allResponse.Body.Bytes()))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer func() { _ = allBook.Close() }()

	rows, err := allBook.GetRows("当前库存")
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("row count = %d, want 3 (header + 2 active)", len(rows))
	}
	wantHeaders := []string{"图片", "商品名称", "商品编码", "数量", "移动平均成本", "库存金额", "库存状态", "更新时间"}
	if len(rows[0]) < len(wantHeaders) {
		t.Fatalf("headers = %#v, want at least %#v", rows[0], wantHeaders)
	}
	for i, header := range wantHeaders {
		if rows[0][i] != header {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], header)
		}
	}

	// Rows ordered by updated_at desc: lowStock first, withImage second.
	if got := cellText(rows, 1, 1); got != "红茶" {
		t.Fatalf("first data name = %q, want 红茶", got)
	}
	if got := cellText(rows, 1, 2); got != "TEA-RED" {
		t.Fatalf("first data code = %q, want TEA-RED", got)
	}
	if got := cellText(rows, 1, 3); got != "3" {
		t.Fatalf("first data qty = %q, want 3", got)
	}
	if got := cellText(rows, 1, 6); got != "低库存" {
		t.Fatalf("first data status = %q, want 低库存", got)
	}
	if got := cellText(rows, 1, 0); got != "无图片" {
		t.Fatalf("missing image cell = %q, want 无图片", got)
	}

	if got := cellText(rows, 2, 1); got != "绿茶" {
		t.Fatalf("second data name = %q, want 绿茶", got)
	}
	if got := cellText(rows, 2, 6); got != "正常" {
		t.Fatalf("second data status = %q, want 正常", got)
	}
	pics, err := allBook.GetPictures("当前库存", "A3")
	if err != nil {
		t.Fatalf("get pictures: %v", err)
	}
	if len(pics) == 0 || len(pics[0].File) == 0 {
		t.Fatalf("expected embedded image bytes on A3, got %#v", pics)
	}
	if pics[0].Extension != ".png" {
		t.Fatalf("embedded image extension = %q, want .png", pics[0].Extension)
	}

	// Archived product must not appear.
	bodyText := string(allResponse.Body.Bytes())
	if strings.Contains(bodyText, "归档茶") || strings.Contains(bodyText, "TEA-ARCH") {
		t.Fatalf("archived product leaked into export")
	}

	// When exporting low stock only
	lowResponse := doJSON(t, router, http.MethodGet, "/api/v1/inventory/export?low_stock=true", token, nil)
	if lowResponse.Code != http.StatusOK {
		t.Fatalf("low stock export status = %d, want 200; body = %s", lowResponse.Code, lowResponse.Body.String())
	}
	lowBook, err := excelize.OpenReader(bytes.NewReader(lowResponse.Body.Bytes()))
	if err != nil {
		t.Fatalf("open low stock workbook: %v", err)
	}
	defer func() { _ = lowBook.Close() }()
	lowRows, err := lowBook.GetRows("当前库存")
	if err != nil {
		t.Fatalf("read low stock rows: %v", err)
	}
	if len(lowRows) != 2 {
		t.Fatalf("low stock row count = %d, want 2 (header + 1)", len(lowRows))
	}
	if got := cellText(lowRows, 1, 1); got != "红茶" {
		t.Fatalf("low stock name = %q, want 红茶", got)
	}
	if got := cellText(lowRows, 1, 2); got != "TEA-RED" {
		t.Fatalf("low stock code = %q, want TEA-RED", got)
	}
}

func writeSolidPNG(path string, width, height int, c color.RGBA) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return png.Encode(file, img)
}

func cellText(rows [][]string, row, col int) string {
	if row >= len(rows) || col >= len(rows[row]) {
		return ""
	}
	return rows[row][col]
}
