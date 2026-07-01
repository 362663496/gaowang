package apihttp

import (
	"net/http"

	"gaowang/apps/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const userIDKey = "current_user_id"
const roleKey = "current_role"

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := uuid.Parse(c.GetHeader("X-Dev-User-ID"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "login required"}})
			c.Abort()
			return
		}
		role := models.Role(c.GetHeader("X-Dev-Role"))
		if role != models.RoleAdmin && role != models.RoleStaff {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "login required"}})
			c.Abort()
			return
		}
		c.Set(userIDKey, userID)
		c.Set(roleKey, role)
		c.Next()
	}
}

func RequireRole(roles ...models.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		current, _ := c.Get(roleKey)
		for _, role := range roles {
			if current == role {
				c.Next()
				return
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "FORBIDDEN", "message": "permission denied"}})
		c.Abort()
	}
}
