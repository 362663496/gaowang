package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	apihttp "gaowang/apps/api/internal/http"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
)

func Test_BackupSchedule_reads_and_updates_daily_shanghai_time(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openHandlerTestDB(t, append(authModels(), &models.Setting{}, &models.BackupJob{})...)
	admin := createTestUser(t, db, "Admin", "backup-admin@example.com", "password123", models.RoleAdmin)
	reader := createTestUser(t, db, "Reader", "backup-reader@example.com", "password123", models.RoleStaff)
	setUserPermissions(t, db, reader.ID, services.PermBackupRead)
	adminToken := createSessionToken(t, db, admin.ID)
	readerToken := createSessionToken(t, db, reader.ID)
	apiToken := createAPIToken(t, db, admin.ID)
	router := apihttp.NewRouter(testConfig(), db)

	got := backupSchedule(t, router, http.MethodGet, adminToken, nil)
	if got.Enabled || got.Time != "02:00" || got.Timezone != "Asia/Shanghai" || got.Due {
		t.Fatalf("default schedule = %+v", got)
	}

	denied := doJSON(t, router, http.MethodPut, "/api/v1/backups/schedule", readerToken, map[string]any{"enabled": true, "time": "03:30"})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("reader update = %d, want 403; body = %s", denied.Code, denied.Body.String())
	}
	tokenDenied := doBearerJSON(t, router, http.MethodGet, "/api/v1/backups/schedule", apiToken, nil)
	if tokenDenied.Code != http.StatusForbidden {
		t.Fatalf("token schedule = %d, want 403; body = %s", tokenDenied.Code, tokenDenied.Body.String())
	}

	invalid := doJSON(t, router, http.MethodPut, "/api/v1/backups/schedule", adminToken, map[string]any{"enabled": true, "time": "25:00"})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid time = %d, want 400; body = %s", invalid.Code, invalid.Body.String())
	}
	saved := backupSchedule(t, router, http.MethodPut, adminToken, map[string]any{"enabled": false, "time": "03:30"})
	if saved.Enabled || saved.Time != "03:30" || saved.Timezone != "Asia/Shanghai" {
		t.Fatalf("saved schedule = %+v", saved)
	}
	reloaded := backupSchedule(t, router, http.MethodGet, readerToken, nil)
	if reloaded.Enabled || reloaded.Time != "03:30" {
		t.Fatalf("reloaded schedule = %+v", reloaded)
	}
	var audit models.AuditLog
	if err := db.First(&audit, "action = ?", "backup.schedule_updated").Error; err != nil {
		t.Fatalf("load schedule audit: %v", err)
	}
}

type backupScheduleBody struct {
	Enabled  bool   `json:"enabled"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
	Due      bool   `json:"due"`
}

func backupSchedule(t *testing.T, router http.Handler, method string, token string, payload any) backupScheduleBody {
	t.Helper()
	response := doJSON(t, router, method, "/api/v1/backups/schedule", token, payload)
	want := http.StatusOK
	if method == http.MethodPut {
		want = http.StatusOK
	}
	if response.Code != want {
		t.Fatalf("%s schedule = %d, want %d; body = %s", method, response.Code, want, response.Body.String())
	}
	var body struct {
		Schedule backupScheduleBody `json:"schedule"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	return body.Schedule
}
