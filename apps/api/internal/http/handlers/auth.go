package handlers

import (
	"net/http"

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
	Email    string `json:"email" binding:"required,email"`
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

	var user userResponse
	var passwordHash string
	err := h.DB.Table("users").
		Select("id, name, email, role, password_hash").
		Where("email = ? AND enabled = ?", req.Email, true).
		Row().
		Scan(&user.ID, &user.Name, &user.Email, &user.Role, &passwordHash)
	if err != nil || !services.PasswordMatches(passwordHash, req.Password) {
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}
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
	c.Status(http.StatusNoContent)
}
