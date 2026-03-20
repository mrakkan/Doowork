package models

import (
	"time"

	"gorm.io/gorm"
)

type ProcessedEvent struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	EventID     string         `gorm:"uniqueIndex;not null" json:"event_id"`
	EventType   string         `gorm:"index;not null" json:"event_type"`
	Source      string         `gorm:"index" json:"source"`
	ProcessedAt time.Time      `gorm:"not null" json:"processed_at"`
}
