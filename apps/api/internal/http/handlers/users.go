package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errUserDeleteSelf = errors.New("users cannot delete themselves")
	errLastAdmin      = errors.New("cannot delete the last active admin")
)

type UserHandler struct {
	DB *gorm.DB
}

type userResponse struct {
	ID    uuid.UUID   `json:"id"`
	Name  string      `json:"name"`
	Email string      `json:"email"`
	Role  models.Role `json:"role"`
}

type createUserRequest struct {
	Name     string      `json:"name" binding:"required,min=1,max=80"`
	Email    string      `json:"email" binding:"required,email"`
	Password string      `json:"password" binding:"required,min=8"`
	Role     models.Role `json:"role" binding:"required"`
}

func (h UserHandler) List(c *gin.Context) {
	var users []userResponse
	query, meta, err := paginate(c, h.DB.Model(&models.User{}).Where("deleted_at IS NULL"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to count users")
		return
	}
	if err := query.Order("created_at desc").Find(&users).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list users")
		return
	}
	writePage(c, users, meta)
}

func (h UserHandler) Delete(c *gin.Context) {
	targetID, ok := parseUUID(c, c.Param("id"), "id")
	if !ok {
		return
	}
	actorID := currentUserID(c)
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var target models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL").First(&target, "id = ?", targetID).Error; err != nil {
			return err
		}
		if target.ID == actorID {
			return errUserDeleteSelf
		}
		if target.Role == models.RoleAdmin && target.Enabled {
			var admins []models.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role = ? AND enabled = ? AND deleted_at IS NULL", models.RoleAdmin, true).Order("id asc").Find(&admins).Error; err != nil {
				return err
			}
			if len(admins) <= 1 {
				return errLastAdmin
			}
		}

		now := time.Now().UTC()
		if err := tx.Model(&models.User{}).Where("id = ? AND deleted_at IS NULL", target.ID).Updates(map[string]any{"enabled": false, "deleted_at": now}).Error; err != nil {
			return err
		}
		if err := (services.SessionService{}).DeleteAllForUserTx(tx, target.ID); err != nil {
			return err
		}
		if err := (services.APITokenService{}).DeleteForUserTx(tx, target.ID); err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", target.ID).Delete(&models.UserPermission{}).Error; err != nil {
			return err
		}
		return tx.Create(&models.AuditLog{
			ActorID:      &actorID,
			Action:       "user.delete",
			ResourceType: "user",
			ResourceID:   target.ID.String(),
			Metadata: auditMetadata(map[string]string{
				"name": target.Name, "email": target.Email, "role": string(target.Role),
			}),
			IPAddress: c.ClientIP(),
		}).Error
	})
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在")
	case errors.Is(err, errUserDeleteSelf):
		writeError(c, http.StatusConflict, "USER_DELETE_SELF", "不能删除当前登录用户")
	case errors.Is(err, errLastAdmin):
		writeError(c, http.StatusConflict, "LAST_ADMIN", "至少保留一个启用的管理员")
	case err != nil:
		writeError(c, http.StatusInternalServerError, "USER_DELETE_FAILED", "删除用户失败")
	default:
		c.Status(http.StatusNoContent)
	}
}

func (h UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Role != models.RoleAdmin && req.Role != models.RoleStaff {
		writeError(c, http.StatusBadRequest, "VALIDATION", "role must be admin or staff")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	if req.Name == "" {
		writeError(c, http.StatusBadRequest, "VALIDATION", "username is required")
		return
	}
	var existing int64
	if err := h.DB.Model(&models.User{}).Where("name = ?", req.Name).Count(&existing).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to check username")
		return
	}
	if existing > 0 {
		writeError(c, http.StatusBadRequest, "USER_CREATE_FAILED", "username already exists")
		return
	}
	hash, err := services.HashPassword(req.Password)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to hash password")
		return
	}
	user := models.User{Name: req.Name, Email: req.Email, PasswordHash: hash, Role: req.Role, Enabled: true}
	if err := h.DB.Create(&user).Error; err != nil {
		writeError(c, http.StatusBadRequest, "USER_CREATE_FAILED", err.Error())
		return
	}
	recordAudit(c, h.DB, "user.create", "user", user.ID.String(), map[string]string{"email": user.Email, "role": string(user.Role)})
	c.JSON(http.StatusCreated, gin.H{"item": userResponse{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role}})
}
