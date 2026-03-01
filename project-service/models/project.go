package models

import (
	"time"

	"gorm.io/gorm"
)

type Project struct {
	ID          uint            `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   gorm.DeletedAt  `gorm:"index" json:"-"`
	Name        string          `gorm:"not null" json:"name"`
	Description string          `json:"description"`
	Status      string          `gorm:"default:planning" json:"status"` // planning, in_progress, completed, on_hold, cancelled
	StartDate   *time.Time      `json:"start_date"`
	EndDate     *time.Time      `json:"end_date"`
	Budget      float64         `json:"budget"`
	OwnerID     uint            `gorm:"not null" json:"owner_id"`
	Members     []ProjectMember `gorm:"foreignKey:ProjectID" json:"members,omitempty"`
}

type ProjectMember struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	ProjectID uint           `gorm:"not null" json:"project_id"`
	UserID    uint           `gorm:"not null" json:"user_id"`
	Role      string         `gorm:"default:member" json:"role"` // owner, admin, member
}

// Request/Response structs
type CreateProjectRequest struct {
	Name        string     `json:"name" binding:"required"`
	Description string     `json:"description"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
	Budget      float64    `json:"budget"`
}

type UpdateProjectRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
	Budget      float64    `json:"budget"`
}

type AddMemberRequest struct {
	UserID uint   `json:"user_id" binding:"required"`
	Role   string `json:"role"`
}

type ProjectStatus struct {
	ProjectID   uint       `json:"project_id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	TotalTasks  int        `json:"total_tasks"`
	Progress    float64    `json:"progress"`
	MemberCount int        `json:"member_count"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
	DaysLeft    int        `json:"days_left"`
}
