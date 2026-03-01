package models

import (
	"time"

	"gorm.io/gorm"
)

type Notification struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	UserID    uint           `gorm:"not null;index" json:"user_id"`
	Title     string         `gorm:"not null" json:"title"`
	Message   string         `gorm:"not null" json:"message"`
	Type      string         `gorm:"default:info" json:"type"` // info, warning, error, success, task, project
	Read      bool           `gorm:"default:false" json:"read"`
	ReadAt    *time.Time     `json:"read_at"`
	Data      string         `json:"data"` // JSON string for additional data
}

type ScheduledNotification struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	UserID       uint           `gorm:"not null;index" json:"user_id"`
	Title        string         `gorm:"not null" json:"title"`
	Message      string         `gorm:"not null" json:"message"`
	Type         string         `gorm:"default:info" json:"type"`
	ScheduledAt  time.Time      `gorm:"not null" json:"scheduled_at"`
	Recurring    bool           `gorm:"default:false" json:"recurring"`
	CronSchedule string         `json:"cron_schedule"`                 // For recurring notifications
	Status       string         `gorm:"default:pending" json:"status"` // pending, sent, cancelled
	SentAt       *time.Time     `json:"sent_at"`
	Data         string         `json:"data"`
}

type NotificationPreference struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	UserID          uint           `gorm:"uniqueIndex;not null" json:"user_id"`
	AllowAll        bool           `gorm:"default:true" json:"allow_all"`
	AllowTask       bool           `gorm:"default:true" json:"allow_task"`
	AllowProject    bool           `gorm:"default:true" json:"allow_project"`
	AllowSystem     bool           `gorm:"default:true" json:"allow_system"`
	AllowReminder   bool           `gorm:"default:true" json:"allow_reminder"`
	EmailEnabled    bool           `gorm:"default:false" json:"email_enabled"`
	PushEnabled     bool           `gorm:"default:true" json:"push_enabled"`
	QuietHoursStart *int           `json:"quiet_hours_start"` // Hour of day (0-23)
	QuietHoursEnd   *int           `json:"quiet_hours_end"`
}

// Request structs
type SendNotificationRequest struct {
	UserID  uint   `json:"user_id" binding:"required"`
	Title   string `json:"title" binding:"required"`
	Message string `json:"message" binding:"required"`
	Type    string `json:"type"`
	Data    string `json:"data"`
}

type ScheduleNotificationRequest struct {
	UserID       uint      `json:"user_id" binding:"required"`
	Title        string    `json:"title" binding:"required"`
	Message      string    `json:"message" binding:"required"`
	Type         string    `json:"type"`
	ScheduledAt  time.Time `json:"scheduled_at" binding:"required"`
	Recurring    bool      `json:"recurring"`
	CronSchedule string    `json:"cron_schedule"`
	Data         string    `json:"data"`
}

type UpdateScheduledRequest struct {
	Title        string     `json:"title"`
	Message      string     `json:"message"`
	Type         string     `json:"type"`
	ScheduledAt  *time.Time `json:"scheduled_at"`
	Recurring    bool       `json:"recurring"`
	CronSchedule string     `json:"cron_schedule"`
	Data         string     `json:"data"`
}

type UpdatePreferencesRequest struct {
	AllowAll        *bool `json:"allow_all"`
	AllowTask       *bool `json:"allow_task"`
	AllowProject    *bool `json:"allow_project"`
	AllowSystem     *bool `json:"allow_system"`
	AllowReminder   *bool `json:"allow_reminder"`
	EmailEnabled    *bool `json:"email_enabled"`
	PushEnabled     *bool `json:"push_enabled"`
	QuietHoursStart *int  `json:"quiet_hours_start"`
	QuietHoursEnd   *int  `json:"quiet_hours_end"`
}

type AllowNotificationsRequest struct {
	Allow bool `json:"allow" binding:"required"`
}
