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

func SaveProductImage(uploadDir string, file multipart.File, header *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return "", fmt.Errorf("image type must be jpg, png, or webp")
	}
	if header.Size > 5*1024*1024 {
		return "", fmt.Errorf("image must be 5MB or smaller")
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}
	name := uuid.NewString() + ext
	dst, err := os.Create(filepath.Join(uploadDir, name))
	if err != nil {
		return "", fmt.Errorf("create upload file: %w", err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, file); err != nil {
		return "", fmt.Errorf("save upload file: %w", err)
	}
	return "/uploads/" + name, nil
}
