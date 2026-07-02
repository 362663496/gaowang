package handlers

import (
	"net/http"
	"strings"

	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
	if err := h.DB.Model(&models.User{}).Order("created_at desc").Find(&users).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to list users")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": users})
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
	c.JSON(http.StatusCreated, gin.H{"item": userResponse{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role}})
}
