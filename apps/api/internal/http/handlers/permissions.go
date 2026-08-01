package handlers

import (
	"encoding/json"
	"errors"
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
	UserID      string   `json:"user_id" binding:"required"`
	Permissions []string `json:"permissions"`
}

type permissionUserResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Permissions []string  `json:"permissions"`
}

func (h PermissionHandler) Get(c *gin.Context) {
	catalog := services.PermissionCatalog()
	var staff []models.User
	if err := h.DB.Where("role = ? AND deleted_at IS NULL", models.RoleStaff).Order("name asc").Order("email asc").Order("id asc").Find(&staff).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load permissions")
		return
	}

	grantsByUser := make(map[uuid.UUID][]string, len(staff))
	if len(staff) > 0 {
		ids := make([]uuid.UUID, 0, len(staff))
		for _, user := range staff {
			ids = append(ids, user.ID)
		}
		var grants []models.UserPermission
		if err := h.DB.Where("user_id IN ?", ids).Order("permission asc").Find(&grants).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load permissions")
			return
		}
		assignable := make(map[string]struct{})
		for _, def := range catalog {
			if def.StaffAssignable {
				assignable[def.Key] = struct{}{}
			}
		}
		for _, grant := range grants {
			if _, ok := assignable[grant.Permission]; ok {
				grantsByUser[grant.UserID] = append(grantsByUser[grant.UserID], grant.Permission)
			}
		}
	}

	users := make([]permissionUserResponse, 0, len(staff))
	for _, user := range staff {
		permissions := grantsByUser[user.ID]
		if permissions == nil {
			permissions = []string{}
		}
		users = append(users, permissionUserResponse{ID: user.ID, Name: user.Name, Email: user.Email, Permissions: permissions})
	}
	c.JSON(http.StatusOK, gin.H{
		"catalog": catalog,
		"users":   users,
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
	userID, ok := parseUUID(c, req.UserID, "user_id")
	if !ok {
		return
	}
	var user models.User
	if err := h.DB.Where("deleted_at IS NULL").First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load user")
		return
	}
	if user.Role != models.RoleStaff {
		writeError(c, http.StatusBadRequest, "VALIDATION", "permissions can only be assigned to staff users")
		return
	}

	var before []string
	var after []string
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		before, after, err = services.ReplaceUserPermissions(tx, userID, req.Permissions)
		if err != nil {
			return err
		}
		metadata, err := json.Marshal(map[string]any{
			"user_id": userID.String(),
			"before":  before,
			"after":   after,
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
			ResourceID:   userID.String(),
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
		"permissions": after,
	})
}

func isPermissionValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unknown permission:") || strings.Contains(msg, "permission is admin-only:")
}
