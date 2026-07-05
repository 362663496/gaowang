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

type AuthHandler struct {
	DB *gorm.DB
}

type loginRequest struct {
	Login    string `json:"login"`
	Email    string `json:"email"`
	Password string `json:"password" binding:"required,min=8"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required,min=8"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

func (h AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	identifier := strings.TrimSpace(req.Login)
	if identifier == "" {
		identifier = strings.TrimSpace(req.Email)
	}
	if identifier == "" {
		writeError(c, http.StatusBadRequest, "VALIDATION", "username or email is required")
		return
	}

	var user userResponse
	var passwordHash string
	err := h.DB.Table("users").
		Select("id, name, email, role, password_hash").
		Where("enabled = ? AND (email = ? OR name = ?)", true, identifier, identifier).
		Row().
		Scan(&user.ID, &user.Name, &user.Email, &user.Role, &passwordHash)
	if err != nil || !services.PasswordMatches(passwordHash, req.Password) {
		recordAuditForActor(c, h.DB, nil, "auth.login_failed", "auth", identifier, map[string]string{"login": identifier})
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}
	recordAuditForActor(c, h.DB, &user.ID, "auth.login_succeeded", "user", user.ID.String(), map[string]string{"login": identifier})
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	userID, err := uuid.Parse(c.GetHeader("X-Dev-User-ID"))
	if err != nil {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
		return
	}
	var user models.User
	if err := h.DB.First(&user, "id = ? AND enabled = ?", userID, true).Error; err != nil {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
		return
	}
	if !services.PasswordMatches(user.PasswordHash, req.CurrentPassword) {
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "current password is incorrect")
		return
	}
	hash, err := services.HashPassword(req.NewPassword)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to hash password")
		return
	}
	if err := h.DB.Model(&user).Update("password_hash", hash).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to change password")
		return
	}
	recordAudit(c, h.DB, "auth.password_changed", "user", user.ID.String(), nil)
	c.Status(http.StatusNoContent)
}
