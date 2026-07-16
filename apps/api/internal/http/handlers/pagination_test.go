package handlers

import (
	"net/http/httptest"
	"testing"

	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Test_Paginate_clamps_pages_and_allows_explicit_all(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Shop{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, name := range []string{"A", "B", "C"} {
		if err := db.Create(&models.Shop{Name: name, Enabled: true}).Error; err != nil {
			t.Fatalf("create shop: %v", err)
		}
	}

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/?page=99&page_size=2", nil)
	query, meta, err := paginate(context, db.Model(&models.Shop{}))
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	var page []models.Shop
	if err := query.Order("name").Find(&page).Error; err != nil {
		t.Fatalf("find page: %v", err)
	}
	if meta.Page != 2 || meta.PageSize != 2 || meta.Total != 3 || meta.TotalPages != 2 || len(page) != 1 {
		t.Fatalf("page/meta = %d %+v, want 1 item on page 2/2", len(page), meta)
	}

	context, _ = gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/?all=true", nil)
	query, meta, err = paginate(context, db.Model(&models.Shop{}))
	if err != nil {
		t.Fatalf("paginate all: %v", err)
	}
	page = nil
	if err := query.Find(&page).Error; err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(page) != 3 || meta.TotalPages != 1 || meta.PageSize != 3 {
		t.Fatalf("all/meta = %d %+v, want all 3", len(page), meta)
	}
}
