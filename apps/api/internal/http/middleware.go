package apihttp

import (
	"net/http"
	"net/url"
	"strings"

	"gaowang/apps/api/internal/config"
	"gaowang/apps/api/internal/models"
	"gaowang/apps/api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	userIDKey       = "current_user_id"
	roleKey         = "current_role"
	userKey         = "current_user"
	permissionsKey  = "current_permissions"
	sessionTokenKey = "session_raw_token"
)

func RequireSameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" {
			writeMiddlewareError(c, http.StatusForbidden, "FORBIDDEN", "origin required")
			return
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			writeMiddlewareError(c, http.StatusForbidden, "FORBIDDEN", "invalid origin")
			return
		}
		expectedHost := c.Request.Host
		if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
			// Prefer the first value when proxies send a list.
			expectedHost = strings.TrimSpace(strings.Split(forwardedHost, ",")[0])
		}
		expectedScheme := "http"
		if c.Request.TLS != nil {
			expectedScheme = "https"
		}
		if forwardedProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
			expectedScheme = strings.ToLower(strings.TrimSpace(strings.Split(forwardedProto, ",")[0]))
		}
		if !strings.EqualFold(parsed.Scheme, expectedScheme) || !strings.EqualFold(parsed.Host, expectedHost) {
			writeMiddlewareError(c, http.StatusForbidden, "FORBIDDEN", "cross-origin request rejected")
			return
		}
		c.Next()
	}
}

func RequireAuth(db *gorm.DB, cfg config.Config) gin.HandlerFunc {
	sessions := services.SessionService{DB: db, Secret: cfg.AuthSecret}
	return func(c *gin.Context) {
		rawToken, err := c.Cookie(services.SessionCookieName)
		if err != nil || rawToken == "" {
			// Intentionally ignore legacy X-Dev-* headers.
			clearSessionCookie(c, cfg)
			writeMiddlewareError(c, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
			return
		}
		user, _, lookupErr := sessions.LookupActiveUser(rawToken)
		if lookupErr != nil {
			clearSessionCookie(c, cfg)
			writeMiddlewareError(c, http.StatusUnauthorized, "UNAUTHORIZED", "login required")
			return
		}
		permissions, permErr := services.EffectivePermissions(db, user)
		if permErr != nil {
			writeMiddlewareError(c, http.StatusInternalServerError, "INTERNAL", "failed to load permissions")
			return
		}
		c.Set(userIDKey, user.ID)
		c.Set(roleKey, user.Role)
		c.Set(userKey, user)
		c.Set(permissionsKey, services.PermissionSet(permissions))
		c.Set(sessionTokenKey, rawToken)
		c.Next()
	}
}

func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleValue, _ := c.Get(roleKey)
		if role, ok := roleValue.(models.Role); ok && role == models.RoleAdmin {
			c.Next()
			return
		}
		setValue, ok := c.Get(permissionsKey)
		if !ok {
			writeMiddlewareError(c, http.StatusForbidden, "FORBIDDEN", "permission denied")
			return
		}
		set, ok := setValue.(map[string]struct{})
		if !ok || !services.HasPermission(set, permission) {
			writeMiddlewareError(c, http.StatusForbidden, "FORBIDDEN", "permission denied")
			return
		}
		c.Next()
	}
}

func writeMiddlewareError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
	c.Abort()
}

func clearSessionCookie(c *gin.Context, cfg config.Config) {
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

// CurrentUserID is exported for handlers that share the middleware package key.
func CurrentUserID(c *gin.Context) uuid.UUID {
	value, ok := c.Get(userIDKey)
	if !ok {
		return uuid.Nil
	}
	id, ok := value.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}
