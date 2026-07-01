package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const currentUserIDKey = "current_user_id"

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func currentUserID(c *gin.Context) uuid.UUID {
	value, ok := c.Get(currentUserIDKey)
	if !ok {
		return uuid.Nil
	}
	id, ok := value.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		writeError(c, http.StatusBadRequest, "VALIDATION", err.Error())
		return false
	}
	return true
}
