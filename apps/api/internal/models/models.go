package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleStaff Role = "staff"
)

type MovementType string

const (
	MovementTypeInbound       MovementType = "inbound"
	MovementTypeSalesOutbound MovementType = "sales_outbound"
	MovementTypeAdjustment    MovementType = "adjustment"
)

type BackupStatus string

const (
	BackupStatusRunning BackupStatus = "running"
	BackupStatusSuccess BackupStatus = "success"
	BackupStatusFailed  BackupStatus = "failed"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name         string    `gorm:"not null"`
	Email        string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	Role         Role      `gorm:"type:varchar(20);not null"`
	Enabled      bool      `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (u *User) BeforeCreate(_ *gorm.DB) error {
	assignUUID(&u.ID)
	return nil
}

type Shop struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name      string    `gorm:"uniqueIndex;not null"`
	Note      string
	Enabled   bool `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Shop) BeforeCreate(_ *gorm.DB) error {
	assignUUID(&s.ID)
	return nil
}

type Product struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name                 string    `gorm:"not null"`
	Code                 string    `gorm:"uniqueIndex;not null"`
	ImagePath            string
	DefaultPurchaseCents int64 `gorm:"not null;default:0"`
	DefaultSaleCents     int64 `gorm:"not null;default:0"`
	LowStockThreshold    int64 `gorm:"not null;default:0"`
	Note                 string
	Enabled              bool       `gorm:"not null;default:true"`
	ArchivedAt           *time.Time `gorm:"index"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (p *Product) BeforeCreate(_ *gorm.DB) error {
	assignUUID(&p.ID)
	return nil
}

type InventorySnapshot struct {
	ProductID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	Product                Product   `gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Quantity               int64     `gorm:"not null;default:0"`
	MovingAverageCostCents int64     `gorm:"not null;default:0"`
	InventoryValueCents    int64     `gorm:"not null;default:0"`
	UpdatedAt              time.Time
}

type StockMovement struct {
	ID                  uuid.UUID    `gorm:"type:uuid;primaryKey"`
	Type                MovementType `gorm:"type:varchar(32);not null;index"`
	ProductID           uuid.UUID    `gorm:"type:uuid;not null;index"`
	Product             Product      `gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	ShopID              *uuid.UUID   `gorm:"type:uuid;index"`
	Shop                *Shop        `gorm:"foreignKey:ShopID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	QuantityDelta       int64        `gorm:"not null"`
	PurchaseUnitCents   *int64
	SaleUnitCents       *int64
	CostUnitCents       int64 `gorm:"not null;default:0"`
	PurchaseAmountCents int64 `gorm:"not null;default:0"`
	RevenueCents        int64 `gorm:"not null;default:0"`
	CostAmountCents     int64 `gorm:"not null;default:0"`
	GrossProfitCents    int64 `gorm:"not null;default:0"`
	Reason              string
	OperatorID          uuid.UUID `gorm:"type:uuid;not null;index"`
	Operator            User      `gorm:"foreignKey:OperatorID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	CreatedAt           time.Time
}

func (m *StockMovement) BeforeCreate(_ *gorm.DB) error {
	assignUUID(&m.ID)
	return nil
}

type AuditLog struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ActorID      *uuid.UUID     `gorm:"type:uuid;index"`
	Actor        *User          `gorm:"foreignKey:ActorID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Action       string         `gorm:"not null;index"`
	ResourceType string         `gorm:"not null;index"`
	ResourceID   string         `gorm:"not null;index"`
	Metadata     datatypes.JSON `gorm:"type:jsonb"`
	IPAddress    string
	CreatedAt    time.Time
}

func (a *AuditLog) BeforeCreate(_ *gorm.DB) error {
	assignUUID(&a.ID)
	return nil
}

type BackupJob struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	StartedAt    time.Time `gorm:"not null;index"`
	FinishedAt   *time.Time
	Status       BackupStatus `gorm:"type:varchar(20);not null;index"`
	FilePath     string
	FileSize     int64 `gorm:"not null;default:0"`
	EmailStatus  string
	Recipient    string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (b *BackupJob) BeforeCreate(_ *gorm.DB) error {
	assignUUID(&b.ID)
	return nil
}

type Setting struct {
	Key       string `gorm:"primaryKey;size:128"`
	Value     string
	UpdatedAt time.Time
}

func assignUUID(id *uuid.UUID) {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
}
