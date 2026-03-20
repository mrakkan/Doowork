package models

import (
	"time"

	"gorm.io/gorm"
)

type OutboxEvent struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	EventID     string         `gorm:"uniqueIndex;not null" json:"event_id"`
	EventType   string         `gorm:"index;not null" json:"event_type"`
	RoutingKey  string         `gorm:"index;not null" json:"routing_key"`
	Payload     string         `gorm:"type:text;not null" json:"payload"`
	Status      string         `gorm:"index;default:pending" json:"status"`
	RetryCount  int            `gorm:"default:0" json:"retry_count"`
	LastError   string         `gorm:"type:text" json:"last_error"`
	PublishedAt *time.Time     `json:"published_at"`
}
