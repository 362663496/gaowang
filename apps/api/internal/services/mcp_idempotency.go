package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gaowang/apps/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MCPRequestRetention = 7 * 24 * time.Hour

var (
	ErrMCPRequestConflict  = errors.New("request_id was used with different arguments")
	ErrMCPRequestIDInvalid = errors.New("invalid request_id")
)

func NormalizeMCPRequestID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", nil
	}
	if len(id) > 128 || strings.ContainsAny(id, "\r\n\x00") {
		return "", fmt.Errorf("%w: request_id 须为 1 到 128 个字符", ErrMCPRequestIDInvalid)
	}
	return id, nil
}

func MCPRequestFingerprint(tool string, payload any) (string, error) {
	raw, err := json.Marshal(struct {
		Tool    string `json:"tool"`
		Payload any    `json:"payload"`
	}{Tool: tool, Payload: payload})
	if err != nil {
		return "", fmt.Errorf("fingerprint mcp request: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// LookupMCPRequest returns a stored successful response. Expired keys are treated as missing.
func LookupMCPRequest(db *gorm.DB, actorID uuid.UUID, requestID string, tool string, fingerprint string) (map[string]any, bool, error) {
	if requestID == "" {
		return nil, false, nil
	}
	if err := purgeExpiredMCPRequests(db); err != nil {
		return nil, false, err
	}
	existing, found, err := findMCPRequest(db, actorID, requestID)
	if err != nil || !found {
		return nil, false, err
	}
	if existing.CreatedAt.Before(time.Now().Add(-MCPRequestRetention)) {
		if err := db.Delete(&existing).Error; err != nil {
			return nil, false, fmt.Errorf("delete expired mcp request: %w", err)
		}
		return nil, false, nil
	}
	payload, err := decodeMCPRequest(existing, tool, fingerprint)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

// CommitMCPRequest runs write inside the stored-key transaction. An empty requestID skips storage.
// A replay returns the first successful payload and duplicate=true without calling write.
func CommitMCPRequest(db *gorm.DB, actorID uuid.UUID, requestID string, tool string, fingerprint string, write func(tx *gorm.DB) (map[string]any, error)) (map[string]any, bool, error) {
	if requestID == "" {
		payload, err := write(db)
		return payload, false, err
	}
	if err := purgeExpiredMCPRequests(db); err != nil {
		return nil, false, err
	}
	var payload map[string]any
	var duplicate bool
	err := db.Transaction(func(tx *gorm.DB) error {
		existing, found, err := lockMCPRequest(tx, actorID, requestID)
		if err != nil {
			return err
		}
		if found && existing.CreatedAt.Before(time.Now().Add(-MCPRequestRetention)) {
			if err := tx.Delete(&existing).Error; err != nil {
				return fmt.Errorf("delete expired mcp request: %w", err)
			}
			found = false
		}
		if found {
			decoded, err := decodeMCPRequest(existing, tool, fingerprint)
			if err != nil {
				return err
			}
			payload = decoded
			duplicate = true
			return nil
		}
		body, err := write(tx)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode mcp response: %w", err)
		}
		row := models.MCPIdempotencyKey{
			ActorID:     actorID,
			RequestID:   requestID,
			Tool:        tool,
			Fingerprint: fingerprint,
			Response:    datatypes.JSON(encoded),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		payload = body
		return nil
	})
	if err != nil && isDuplicateKey(err) {
		return readCommittedMCPRequest(db, actorID, requestID, tool, fingerprint)
	}
	return payload, duplicate, err
}

func purgeExpiredMCPRequests(db *gorm.DB) error {
	cutoff := time.Now().Add(-MCPRequestRetention)
	if err := db.Where("created_at < ?", cutoff).Delete(&models.MCPIdempotencyKey{}).Error; err != nil {
		return fmt.Errorf("purge expired mcp requests: %w", err)
	}
	return nil
}

func findMCPRequest(db *gorm.DB, actorID uuid.UUID, requestID string) (models.MCPIdempotencyKey, bool, error) {
	var existing models.MCPIdempotencyKey
	err := db.Where("actor_id = ? AND request_id = ?", actorID, requestID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.MCPIdempotencyKey{}, false, nil
	}
	if err != nil {
		return models.MCPIdempotencyKey{}, false, fmt.Errorf("load mcp request: %w", err)
	}
	return existing, true, nil
}

func lockMCPRequest(tx *gorm.DB, actorID uuid.UUID, requestID string) (models.MCPIdempotencyKey, bool, error) {
	var existing models.MCPIdempotencyKey
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("actor_id = ? AND request_id = ?", actorID, requestID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.MCPIdempotencyKey{}, false, nil
	}
	if err != nil {
		return models.MCPIdempotencyKey{}, false, fmt.Errorf("lock mcp request: %w", err)
	}
	return existing, true, nil
}

func decodeMCPRequest(existing models.MCPIdempotencyKey, tool string, fingerprint string) (map[string]any, error) {
	if existing.Tool != tool || existing.Fingerprint != fingerprint {
		return nil, ErrMCPRequestConflict
	}
	var payload map[string]any
	if err := json.Unmarshal(existing.Response, &payload); err != nil {
		return nil, fmt.Errorf("decode stored mcp response: %w", err)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func readCommittedMCPRequest(db *gorm.DB, actorID uuid.UUID, requestID string, tool string, fingerprint string) (map[string]any, bool, error) {
	existing, found, err := findMCPRequest(db, actorID, requestID)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, gorm.ErrRecordNotFound
	}
	payload, err := decodeMCPRequest(existing, tool, fingerprint)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}
