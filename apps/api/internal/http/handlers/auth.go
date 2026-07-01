package handlers

import (
	"net/http"

	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB *gorm.DB
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
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
