package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AuditHandler struct {
	DB *gorm.DB
}

type auditLogResponse struct {
	ID           uuid.UUID         `json:"id"`
	ActorID      *uuid.UUID        `json:"actor_id"`
	Actor        *userResponse     `json:"actor"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	Metadata     map[string]string `json:"metadata"`
	IPAddress    string            `json:"ip_address"`
	CreatedAt    time.Time         `json:"created_at"`
}

func (h AuditHandler) List(c *gin.Context) {
	var logs []models.AuditLog
	query := h.DB.Preload("Actor").Order("created_at desc").Limit(queryLimit(c, 100, 500))
	if value := c.Query("actor_id"); value != "" {
		query = query.Where("actor_id = ?", value)
	}
	if value := c.Query("action"); value != "" {
		query = query.Where("action = ?", value)
	}
	if value := c.Query("resource_type"); value != "" {
		query = query.Where("resource_type = ?", value)
	}
	if value := c.Query("resource_id"); value != "" {
		query = query.Where("resource_id = ?", value)
	}
	if from, ok := queryDate(c, "from"); ok {
		query = query.Where("created_at >= ?", from)
	}
	if to, ok := queryDate(c, "to"); ok {
		query = query.Where("created_at < ?", to.AddDate(0, 0, 1))
	}
	if err := query.Find(&logs).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list audit logs")
		return
	}
	items := make([]auditLogResponse, 0, len(logs))
	for _, log := range logs {
		items = append(items, auditResponse(log))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func recordAudit(c *gin.Context, db *gorm.DB, action string, resourceType string, resourceID string, metadata map[string]string) {
	actorID := currentUserID(c)
	if actorID == uuid.Nil {
		recordAuditForActor(c, db, nil, action, resourceType, resourceID, metadata)
		return
	}
	recordAuditForActor(c, db, &actorID, action, resourceType, resourceID, metadata)
}

func recordAuditForActor(c *gin.Context, db *gorm.DB, actorID *uuid.UUID, action string, resourceType string, resourceID string, metadata map[string]string) {
	if db == nil {
		return
	}
	log := models.AuditLog{
		ActorID:      actorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     auditMetadata(metadata),
		IPAddress:    c.ClientIP(),
	}
	if err := db.Create(&log).Error; err != nil {
		return
	}
}

func auditMetadata(metadata map[string]string) datatypes.JSON {
	if len(metadata) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(data)
}

func auditResponse(log models.AuditLog) auditLogResponse {
	var actor *userResponse
	if log.Actor != nil {
		actor = &userResponse{ID: log.Actor.ID, Name: log.Actor.Name, Email: log.Actor.Email, Role: log.Actor.Role}
	}
	return auditLogResponse{
		ID: log.ID, ActorID: log.ActorID, Actor: actor, Action: log.Action,
		ResourceType: log.ResourceType, ResourceID: log.ResourceID, Metadata: decodeAuditMetadata(log.Metadata),
		IPAddress: log.IPAddress, CreatedAt: log.CreatedAt,
	}
}

func decodeAuditMetadata(data datatypes.JSON) map[string]string {
	if len(data) == 0 {
		return map[string]string{}
	}
	var metadata map[string]string
	if err := json.Unmarshal(data, &metadata); err != nil {
		return map[string]string{}
	}
	return metadata
}

func queryLimit(c *gin.Context, fallback int, max int) int {
	raw := c.Query("limit")
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func queryDate(c *gin.Context, key string) (time.Time, bool) {
	raw := c.Query(key)
	if raw == "" {
		return time.Time{}, false
	}
	value, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, false
	}
	return value, true
}
