package models

import (
	"time"

	"gorm.io/gorm"
)

type Task struct {
	ID             uint             `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	DeletedAt      gorm.DeletedAt   `gorm:"index" json:"-"`
	Title          string           `gorm:"not null" json:"title"`
	Description    string           `json:"description"`
	Status         string           `gorm:"default:todo" json:"status"`     // todo, in_progress, review, done, cancelled
	Priority       string           `gorm:"default:medium" json:"priority"` // low, medium, high, urgent
	ProjectID      uint             `gorm:"not null" json:"project_id"`
	CreatorID      uint             `gorm:"not null" json:"creator_id"`
	DueDate        *time.Time       `json:"due_date"`
	EstimatedHours float64          `json:"estimated_hours"`
	HourlyRate     float64          `json:"hourly_rate"` // Rate per hour for price calculation
	Assignments    []TaskAssignment `gorm:"foreignKey:TaskID" json:"assignments,omitempty"`
	TimeLogs       []TimeLog        `gorm:"foreignKey:TaskID" json:"time_logs,omitempty"`
}

type TaskAssignment struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	TaskID    uint           `gorm:"not null" json:"task_id"`
	UserID    uint           `gorm:"not null" json:"user_id"`
	Role      string         `gorm:"default:assignee" json:"role"` // assignee, reviewer
}

type TimeLog struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	TaskID      uint           `gorm:"not null" json:"task_id"`
	UserID      uint           `gorm:"not null" json:"user_id"`
	Hours       float64        `gorm:"not null" json:"hours"`
	Description string         `json:"description"`
	LogDate     time.Time      `gorm:"not null" json:"log_date"`
}

type CreateTaskRequest struct {
	Title          string     `json:"title" binding:"required"`
	Description    string     `json:"description"`
	ProjectID      uint       `json:"project_id" binding:"required"`
	Priority       string     `json:"priority"`
	DueDate        *time.Time `json:"due_date"`
	EstimatedHours float64    `json:"estimated_hours"`
	HourlyRate     float64    `json:"hourly_rate"`
}

type UpdateTaskRequest struct {
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	DueDate        *time.Time `json:"due_date"`
	EstimatedHours float64    `json:"estimated_hours"`
	HourlyRate     float64    `json:"hourly_rate"`
}

type AssignTaskRequest struct {
	UserID uint   `json:"user_id" binding:"required"`
	Role   string `json:"role"`
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type LogTimeRequest struct {
	Hours       float64   `json:"hours" binding:"required"`
	Description string    `json:"description"`
	LogDate     time.Time `json:"log_date"`
}

type TaskStatus struct {
	TaskID         uint       `json:"task_id"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	Priority       string     `json:"priority"`
	AssigneeCount  int        `json:"assignee_count"`
	TotalTimeSpent float64    `json:"total_time_spent"`
	EstimatedHours float64    `json:"estimated_hours"`
	Progress       float64    `json:"progress"` // percentage
	DueDate        *time.Time `json:"due_date"`
	IsOverdue      bool       `json:"is_overdue"`
}

type TimeCalculation struct {
	TaskID         uint    `json:"task_id"`
	EstimatedHours float64 `json:"estimated_hours"`
	ActualHours    float64 `json:"actual_hours"`
	RemainingHours float64 `json:"remaining_hours"`
	Variance       float64 `json:"variance"` // negative means over estimate
}

type PriceCalculation struct {
	TaskID         uint    `json:"task_id,omitempty"`
	ProjectID      uint    `json:"project_id,omitempty"`
	HourlyRate     float64 `json:"hourly_rate"`
	EstimatedHours float64 `json:"estimated_hours"`
	ActualHours    float64 `json:"actual_hours"`
	EstimatedCost  float64 `json:"estimated_cost"`
	ActualCost     float64 `json:"actual_cost"`
	Variance       float64 `json:"cost_variance"`
}
