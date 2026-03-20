package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"project-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sony/gobreaker"
	"gorm.io/gorm"
)

type Handler struct {
	db             *gorm.DB
	userServiceURL string
	httpClient     *http.Client
	userBreaker    *gobreaker.CircuitBreaker
}

func NewHandler(db *gorm.DB, userServiceURL string) *Handler {
	if userServiceURL == "" {
		userServiceURL = getEnv("USER_SERVICE_URL", "http://user-service:8081")
	}

	userBreaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "project-user-service",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     15 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	})

	return &Handler{
		db:             db,
		userServiceURL: userServiceURL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		userBreaker:    userBreaker,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (h *Handler) fetchUserByID(userID uint) (*models.UserSummary, bool, error) {
	result, err := h.userBreaker.Execute(func() (interface{}, error) {
		url := fmt.Sprintf("%s/internal/users/%d", h.userServiceURL, userID)
		resp, err := h.httpClient.Get(url)
		if err != nil {
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		return nil, false, err
	}

	resp := result.(*http.Response)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("user service returned status %d", resp.StatusCode)
	}

	var user models.UserSummary
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, false, err
	}

	return &user, true, nil
}

func (h *Handler) enqueueOutboxEvent(tx *gorm.DB, eventType, routingKey string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	outbox := models.OutboxEvent{
		EventID:    uuid.NewString(),
		EventType:  eventType,
		RoutingKey: routingKey,
		Payload:    string(body),
		Status:     "pending",
	}

	return tx.Create(&outbox).Error
}

func (h *Handler) enrichMembers(members []models.ProjectMember) []models.ProjectMember {
	for index := range members {
		user, exists, err := h.fetchUserByID(members[index].UserID)
		if err != nil || !exists {
			continue
		}
		members[index].User = user
	}
	return members
}

// CreateProject creates a new project
func (h *Handler) CreateProject(c *gin.Context) {
	var req models.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	ownerID := userID.(uint)

	if _, exists, err := h.fetchUserByID(ownerID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to validate owner with user service"})
		return
	} else if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Owner user not found"})
		return
	}

	project := models.Project{
		Name:        req.Name,
		Description: req.Description,
		Status:      "planning",
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Budget:      req.Budget,
		OwnerID:     ownerID,
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		member := models.ProjectMember{
			ProjectID: project.ID,
			UserID:    ownerID,
			Role:      "owner",
		}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}

		return h.enqueueOutboxEvent(tx, "project.created", "project.created", map[string]interface{}{
			"project_id": project.ID,
			"name":       project.Name,
			"owner_id":   project.OwnerID,
		})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

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

	for i := range projects {
		projects[i].Members = h.enrichMembers(projects[i].Members)
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

	project.Members = h.enrichMembers(project.Members)

	c.JSON(http.StatusOK, project)
}

// GetProjectInternal returns minimal project data for internal service-to-service calls
func (h *Handler) GetProjectInternal(c *gin.Context) {
	id := c.Param("id")

	var project models.Project
	if err := h.db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       project.ID,
		"name":     project.Name,
		"status":   project.Status,
		"owner_id": project.OwnerID,
	})
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

	user, exists, err := h.fetchUserByID(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to validate user with user service"})
		return
	}
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found in user service"})
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

	err = h.db.Transaction(func(tx *gorm.DB) error {
		var existingMember models.ProjectMember
		checkErr := tx.Where("project_id = ? AND user_id = ?", projectID, req.UserID).First(&existingMember).Error
		if checkErr == nil {
			return fmt.Errorf("member_already_exists")
		}
		if checkErr != nil && !errors.Is(checkErr, gorm.ErrRecordNotFound) {
			return checkErr
		}

		if err := tx.Create(&member).Error; err != nil {
			return err
		}

		return h.enqueueOutboxEvent(tx, "project.member_added", "project.member_added", map[string]interface{}{
			"project_id": project.ID,
			"project":    project.Name,
			"user_id":    req.UserID,
			"role":       role,
		})
	})

	if err != nil {
		if err.Error() == "member_already_exists" {
			c.JSON(http.StatusConflict, gin.H{"error": "User is already a member of this project"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add member"})
		return
	}

	member.User = user

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

	members = h.enrichMembers(members)

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
