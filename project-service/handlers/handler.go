package handlers

import (
	"net/http"
	"time"

	"project-service/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// CreateProject creates a new project
func (h *Handler) CreateProject(c *gin.Context) {
	var req models.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	project := models.Project{
		Name:        req.Name,
		Description: req.Description,
		Status:      "planning",
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Budget:      req.Budget,
		OwnerID:     userID.(uint),
	}

	if err := h.db.Create(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	// Add owner as project member
	member := models.ProjectMember{
		ProjectID: project.ID,
		UserID:    userID.(uint),
		Role:      "owner",
	}
	h.db.Create(&member)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Project created successfully",
		"project": project,
	})
}

// GetProjects returns all projects for the current user
func (h *Handler) GetProjects(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var projects []models.Project

	// Get projects where user is a member
	subQuery := h.db.Table("project_members").
		Select("project_id").
		Where("user_id = ? AND deleted_at IS NULL", userID)

	if err := h.db.Where("id IN (?)", subQuery).
		Or("owner_id = ?", userID).
		Preload("Members").
		Find(&projects).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch projects"})
		return
	}

	c.JSON(http.StatusOK, projects)
}

// GetProject returns a specific project
func (h *Handler) GetProject(c *gin.Context) {
	id := c.Param("id")

	var project models.Project
	if err := h.db.Preload("Members").First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	c.JSON(http.StatusOK, project)
}

// UpdateProject updates a project
func (h *Handler) UpdateProject(c *gin.Context) {
	id := c.Param("id")

	var project models.Project
	if err := h.db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var req models.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields
	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = req.Description
	}
	if req.Status != "" {
		project.Status = req.Status
	}
	if req.StartDate != nil {
		project.StartDate = req.StartDate
	}
	if req.EndDate != nil {
		project.EndDate = req.EndDate
	}
	if req.Budget > 0 {
		project.Budget = req.Budget
	}

	if err := h.db.Save(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Project updated successfully",
		"project": project,
	})
}

// DeleteProject deletes a project
func (h *Handler) DeleteProject(c *gin.Context) {
	id := c.Param("id")

	var project models.Project
	if err := h.db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Delete project members
	h.db.Where("project_id = ?", id).Delete(&models.ProjectMember{})

	if err := h.db.Delete(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete project"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Project deleted successfully"})
}

// GetProjectStatus returns the status of a project
func (h *Handler) GetProjectStatus(c *gin.Context) {
	id := c.Param("id")

	var project models.Project
	if err := h.db.Preload("Members").First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Calculate days left
	var daysLeft int
	if project.EndDate != nil {
		daysLeft = int(time.Until(*project.EndDate).Hours() / 24)
		if daysLeft < 0 {
			daysLeft = 0
		}
	}

	status := models.ProjectStatus{
		ProjectID:   project.ID,
		Name:        project.Name,
		Status:      project.Status,
		TotalTasks:  0, // Would need to query task-service
		Progress:    0, // Would need to calculate from tasks
		MemberCount: len(project.Members),
		StartDate:   project.StartDate,
		EndDate:     project.EndDate,
		DaysLeft:    daysLeft,
	}

	c.JSON(http.StatusOK, status)
}

// AddProjectMember adds a member to a project
func (h *Handler) AddProjectMember(c *gin.Context) {
	projectID := c.Param("id")

	var project models.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var req models.AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if already a member
	var existingMember models.ProjectMember
	if err := h.db.Where("project_id = ? AND user_id = ?", projectID, req.UserID).First(&existingMember).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User is already a member of this project"})
		return
	}

	role := req.Role
	if role == "" {
		role = "member"
	}

	member := models.ProjectMember{
		ProjectID: project.ID,
		UserID:    req.UserID,
		Role:      role,
	}

	if err := h.db.Create(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add member"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Member added successfully",
		"member":  member,
	})
}

// GetProjectMembers returns all members of a project
func (h *Handler) GetProjectMembers(c *gin.Context) {
	projectID := c.Param("id")

	var members []models.ProjectMember
	if err := h.db.Where("project_id = ?", projectID).Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch members"})
		return
	}

	c.JSON(http.StatusOK, members)
}

// RemoveProjectMember removes a member from a project
func (h *Handler) RemoveProjectMember(c *gin.Context) {
	projectID := c.Param("id")
	memberID := c.Param("member_id")

	var member models.ProjectMember
	if err := h.db.Where("project_id = ? AND id = ?", projectID, memberID).First(&member).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Member not found"})
		return
	}

	if err := h.db.Delete(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove member"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member removed successfully"})
}
