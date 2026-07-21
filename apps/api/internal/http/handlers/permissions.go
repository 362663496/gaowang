package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type PermissionHandler struct {
	DB *gorm.DB
}

type updatePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

func (h PermissionHandler) Get(c *gin.Context) {
	staffPermissions, err := loadStaffAssignablePermissions(h.DB)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load permissions")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"catalog":           services.PermissionCatalog(),
		"staff_permissions": staffPermissions,
	})
}

func (h PermissionHandler) Update(c *gin.Context) {
	var req updatePermissionsRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Permissions == nil {
		req.Permissions = []string{}
	}

	var before []string
	var after []string
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		before, after, err = services.ReplaceStaffPermissions(tx, req.Permissions)
		if err != nil {
			return err
		}
		metadata, err := json.Marshal(map[string]any{
			"before": before,
			"after":  after,
		})
		if err != nil {
			return err
		}
		actorID := currentUserID(c)
		var actorPtr *uuid.UUID
		if actorID != uuid.Nil {
			actorPtr = &actorID
		}
		log := models.AuditLog{
			ActorID:      actorPtr,
			Action:       "permission.updated",
			ResourceType: "permission",
			ResourceID:   "staff",
			Metadata:     datatypes.JSON(metadata),
			IPAddress:    c.ClientIP(),
		}
		return tx.Create(&log).Error
	})
	if err != nil {
		if isPermissionValidationError(err) {
			writeError(c, http.StatusBadRequest, "VALIDATION", err.Error())
			return
		}
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to update permissions")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"catalog":           services.PermissionCatalog(),
		"staff_permissions": after,
	})
}

func loadStaffAssignablePermissions(db *gorm.DB) ([]string, error) {
	return services.EffectivePermissions(db, models.User{Role: models.RoleStaff})
}

func isPermissionValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unknown permission:") || strings.Contains(msg, "permission is admin-only:")
}
