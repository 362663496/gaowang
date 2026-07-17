package services

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"

	"github.com/xuri/excelize/v2"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	inventoryExportSheet     = "当前库存"
	inventoryThumbMaxSide    = 96
	inventoryExportRowHeight = 72
)

// BuildInventoryWorkbook creates an XLSX workbook for the given inventory rows.
// Missing or unreadable product images are replaced with the text "无图片" and never fail the export.
func BuildInventoryWorkbook(items []models.InventorySnapshot, uploadDir string) ([]byte, error) {
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()

	defaultSheet := book.GetSheetName(0)
	if err := book.SetSheetName(defaultSheet, inventoryExportSheet); err != nil {
		return nil, fmt.Errorf("rename sheet: %w", err)
	}

	headers := []string{"图片", "商品名称", "商品编码", "数量", "移动平均成本", "库存金额", "库存状态", "更新时间"}
	headerStyle, err := book.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, fmt.Errorf("create header style: %w", err)
	}
	moneyStyle, err := book.NewStyle(&excelize.Style{
		NumFmt:    2, // 0.00
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, fmt.Errorf("create money style: %w", err)
	}
	centerStyle, err := book.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, fmt.Errorf("create cell style: %w", err)
	}

	for i, header := range headers {
		cell, cellErr := excelize.CoordinatesToCellName(i+1, 1)
		if cellErr != nil {
			return nil, cellErr
		}
		if err := book.SetCellValue(inventoryExportSheet, cell, header); err != nil {
			return nil, fmt.Errorf("set header %s: %w", header, err)
		}
		if err := book.SetCellStyle(inventoryExportSheet, cell, cell, headerStyle); err != nil {
			return nil, fmt.Errorf("style header %s: %w", header, err)
		}
	}
	_ = book.SetRowHeight(inventoryExportSheet, 1, 22)
	_ = book.SetColWidth(inventoryExportSheet, "A", "A", 12)
	_ = book.SetColWidth(inventoryExportSheet, "B", "B", 22)
	_ = book.SetColWidth(inventoryExportSheet, "C", "C", 16)
	_ = book.SetColWidth(inventoryExportSheet, "D", "D", 10)
	_ = book.SetColWidth(inventoryExportSheet, "E", "F", 14)
	_ = book.SetColWidth(inventoryExportSheet, "G", "G", 10)
	_ = book.SetColWidth(inventoryExportSheet, "H", "H", 18)

	for index, item := range items {
		row := index + 2
		_ = book.SetRowHeight(inventoryExportSheet, row, inventoryExportRowHeight)

		name := item.Product.Name
		code := item.Product.Code
		threshold := item.Product.LowStockThreshold
		imagePath := item.Product.ImagePath

		values := map[string]any{
			fmt.Sprintf("B%d", row): name,
			fmt.Sprintf("C%d", row): code,
			fmt.Sprintf("D%d", row): item.Quantity,
			fmt.Sprintf("E%d", row): float64(item.MovingAverageCostCents) / 100,
			fmt.Sprintf("F%d", row): float64(item.InventoryValueCents) / 100,
			fmt.Sprintf("G%d", row): stockStatusLabel(item.Quantity, threshold),
			fmt.Sprintf("H%d", row): formatExportTime(item.UpdatedAt),
		}
		for cell, value := range values {
			if err := book.SetCellValue(inventoryExportSheet, cell, value); err != nil {
				return nil, fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
		_ = book.SetCellStyle(inventoryExportSheet, fmt.Sprintf("B%d", row), fmt.Sprintf("D%d", row), centerStyle)
		_ = book.SetCellStyle(inventoryExportSheet, fmt.Sprintf("E%d", row), fmt.Sprintf("F%d", row), moneyStyle)
		_ = book.SetCellStyle(inventoryExportSheet, fmt.Sprintf("G%d", row), fmt.Sprintf("H%d", row), centerStyle)

		imageCell := fmt.Sprintf("A%d", row)
		thumb, thumbErr := productImageThumbnailPNG(uploadDir, imagePath)
		if thumbErr != nil || len(thumb) == 0 {
			if err := book.SetCellValue(inventoryExportSheet, imageCell, "无图片"); err != nil {
				return nil, fmt.Errorf("set missing image label: %w", err)
			}
			_ = book.SetCellStyle(inventoryExportSheet, imageCell, imageCell, centerStyle)
			continue
		}
		if err := book.AddPictureFromBytes(inventoryExportSheet, imageCell, &excelize.Picture{
			Extension: ".png",
			File:      thumb,
			Format: &excelize.GraphicOptions{
				AltText:         name,
				LockAspectRatio: true,
				AutoFit:         true,
			},
		}); err != nil {
			if setErr := book.SetCellValue(inventoryExportSheet, imageCell, "无图片"); setErr != nil {
				return nil, fmt.Errorf("set image fallback after embed failure: %w", setErr)
			}
			_ = book.SetCellStyle(inventoryExportSheet, imageCell, imageCell, centerStyle)
		}
	}

	var buf bytes.Buffer
	if err := book.Write(&buf); err != nil {
		return nil, fmt.Errorf("write workbook: %w", err)
	}
	return buf.Bytes(), nil
}

func stockStatusLabel(quantity int64, threshold int64) string {
	if quantity <= 0 {
		return "无库存"
	}
	if threshold > 0 && quantity <= threshold {
		return "低库存"
	}
	return "正常"
}

func formatExportTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(time.Local).Format("2006-01-02 15:04")
}

func productImageThumbnailPNG(uploadDir, imagePath string) ([]byte, error) {
	if uploadDir == "" || strings.TrimSpace(imagePath) == "" {
		return nil, fmt.Errorf("no image")
	}
	name := filepath.Base(imagePath)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return nil, fmt.Errorf("invalid image path")
	}
	// Reject path components that try to escape the upload directory.
	if name != filepath.Clean(name) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid image name")
	}
	fullPath := filepath.Join(uploadDir, name)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	src, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("empty image")
	}

	maxSide := inventoryThumbMaxSide
	scale := 1.0
	if width > maxSide || height > maxSide {
		if width >= height {
			scale = float64(maxSide) / float64(width)
		} else {
			scale = float64(maxSide) / float64(height)
		}
	}
	newW := int(float64(width) * scale)
	newH := int(float64(height) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
