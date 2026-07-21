package handlers

import (
	"net/http"
	"strings"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB  *gorm.DB
	Cfg config.Config
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

type authUserResponse struct {
	ID    uuid.UUID   `json:"id"`
	Name  string      `json:"name"`
	Email string      `json:"email"`
	Role  models.Role `json:"role"`
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

	var user models.User
	err := h.DB.Where("enabled = ? AND (email = ? OR name = ?)", true, identifier, identifier).First(&user).Error
	if err != nil || !services.PasswordMatches(user.PasswordHash, req.Password) {
		recordAuditForActor(c, h.DB, nil, "auth.login_failed", "auth", identifier, map[string]string{"login": identifier})
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}

	sessions := services.SessionService{DB: h.DB, Secret: h.Cfg.AuthSecret}
	rawToken, _, err := sessions.Create(user.ID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to create session")
		return
	}
	permissions, err := services.EffectivePermissions(h.DB, user)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load permissions")
		return
	}

	setAuthCookie(c, h.Cfg, rawToken)
	recordAuditForActor(c, h.DB, &user.ID, "auth.login_succeeded", "user", user.ID.String(), map[string]string{"login": identifier})
	c.JSON(http.StatusOK, gin.H{
		"user":        authUserResponse{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role},
		"permissions": permissions,
	})
}

func (h AuthHandler) Me(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
		return
	}
	permissions, err := services.EffectivePermissions(h.DB, user)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to load permissions")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user":        authUserResponse{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role},
		"permissions": permissions,
	})
}

func (h AuthHandler) Logout(c *gin.Context) {
	rawToken, _ := c.Get("session_raw_token")
	token, _ := rawToken.(string)
	sessions := services.SessionService{DB: h.DB, Secret: h.Cfg.AuthSecret}
	_ = sessions.DeleteByToken(token)
	clearAuthCookie(c, h.Cfg)
	c.Status(http.StatusNoContent)
}

func (h AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
		return
	}
	if !services.PasswordMatches(user.PasswordHash, req.CurrentPassword) {
		// Session is still valid; do not clear cookie.
		writeError(c, http.StatusBadRequest, "INVALID_CREDENTIALS", "current password is incorrect")
		return
	}
	hash, err := services.HashPassword(req.NewPassword)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to hash password")
		return
	}
	sessions := services.SessionService{DB: h.DB, Secret: h.Cfg.AuthSecret}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", user.ID).Update("password_hash", hash).Error; err != nil {
			return err
		}
		return sessions.DeleteAllForUserTx(tx, user.ID)
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL", "failed to change password")
		return
	}
	clearAuthCookie(c, h.Cfg)
	recordAudit(c, h.DB, "auth.password_changed", "user", user.ID.String(), nil)
	c.Status(http.StatusNoContent)
}

func currentUser(c *gin.Context) (models.User, bool) {
	value, ok := c.Get("current_user")
	if !ok {
		return models.User{}, false
	}
	user, ok := value.(models.User)
	return user, ok
}

func setAuthCookie(c *gin.Context, cfg config.Config, rawToken string) {
	c.SetSameSite(http.SameSiteStrictMode)
	maxAge := int(services.SessionTTL.Seconds())
	c.SetCookie(services.SessionCookieName, rawToken, maxAge, services.SessionCookiePath, "", cookieSecure(c, cfg), true)
}

func clearAuthCookie(c *gin.Context, cfg config.Config) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(services.SessionCookieName, "", -1, services.SessionCookiePath, "", cookieSecure(c, cfg), true)
}

func cookieSecure(c *gin.Context, cfg config.Config) bool {
	if cfg.SessionCookieSecure {
		return true
	}
	if c.Request.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]))
	return proto == "https"
}
