package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"task-service/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sony/gobreaker"
	"gorm.io/gorm"
)

type Handler struct {
	db                *gorm.DB
	userServiceURL    string
	projectServiceURL string
	httpClient        *http.Client
	userBreaker       *gobreaker.CircuitBreaker
	projectBreaker    *gobreaker.CircuitBreaker
}

func NewHandler(db *gorm.DB, userServiceURL, projectServiceURL string) *Handler {
	if userServiceURL == "" {
		userServiceURL = getEnv("USER_SERVICE_URL", "http://user-service:8081")
	}
	if projectServiceURL == "" {
		projectServiceURL = getEnv("PROJECT_SERVICE_URL", "http://project-service:8082")
	}

	userBreaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "task-user-service",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     15 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	})

	projectBreaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "task-project-service",
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     15 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	})

	return &Handler{
		db:                db,
		userServiceURL:    userServiceURL,
		projectServiceURL: projectServiceURL,
		httpClient:        &http.Client{Timeout: 5 * time.Second},
		userBreaker:       userBreaker,
		projectBreaker:    projectBreaker,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (h *Handler) userExists(userID uint) (bool, error) {
	result, err := h.userBreaker.Execute(func() (interface{}, error) {
		url := fmt.Sprintf("%s/internal/users/%d", h.userServiceURL, userID)
		resp, err := h.httpClient.Get(url)
		if err != nil {
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		return false, err
	}

	resp := result.(*http.Response)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("user service returned status %d", resp.StatusCode)
	}

	return true, nil
}

func (h *Handler) projectExists(projectID uint) (bool, error) {
	result, err := h.projectBreaker.Execute(func() (interface{}, error) {
		url := fmt.Sprintf("%s/internal/projects/%d", h.projectServiceURL, projectID)
		resp, err := h.httpClient.Get(url)
		if err != nil {
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		return false, err
	}

	resp := result.(*http.Response)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("project service returned status %d", resp.StatusCode)
	}

	return true, nil
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

// CreateTask creates a new task
func (h *Handler) CreateTask(c *gin.Context) {
	var req models.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	creatorID := userID.(uint)

	projectExists, err := h.projectExists(req.ProjectID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to validate project with project service"})
		return
	}
	if !projectExists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Project not found"})
		return
	}

	userExists, err := h.userExists(creatorID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to validate user with user service"})
		return
	}
	if !userExists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Creator user not found"})
		return
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	task := models.Task{
		Title:          req.Title,
		Description:    req.Description,
		Status:         "todo",
		Priority:       priority,
		ProjectID:      req.ProjectID,
		CreatorID:      creatorID,
		DueDate:        req.DueDate,
		EstimatedHours: req.EstimatedHours,
		HourlyRate:     req.HourlyRate,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}

		return h.enqueueOutboxEvent(tx, "task.created", "task.created", map[string]interface{}{
			"task_id":     task.ID,
			"title":       task.Title,
			"project_id":  task.ProjectID,
			"creator_id":  task.CreatorID,
			"priority":    task.Priority,
			"status":      task.Status,
			"description": task.Description,
		})
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Task created successfully",
		"task":    task,
	})
}

// GetTasks returns all tasks (with optional filtering)
func (h *Handler) GetTasks(c *gin.Context) {
	projectID := c.Query("project_id")
	status := c.Query("status")
	assignee := c.Query("assignee_id")

	query := h.db.Preload("Assignments")

	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if assignee != "" {
		subQuery := h.db.Table("task_assignments").
			Select("task_id").
			Where("user_id = ? AND deleted_at IS NULL", assignee)
		query = query.Where("id IN (?)", subQuery)
	}

	var tasks []models.Task
	if err := query.Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

// GetTask returns a specific task
func (h *Handler) GetTask(c *gin.Context) {
	id := c.Param("id")

	var task models.Task
	if err := h.db.Preload("Assignments").Preload("TimeLogs").First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	c.JSON(http.StatusOK, task)
}

// UpdateTask updates a task
func (h *Handler) UpdateTask(c *gin.Context) {
	id := c.Param("id")

	var task models.Task
	if err := h.db.First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var req models.UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update fields
	if req.Title != "" {
		task.Title = req.Title
	}
	if req.Description != "" {
		task.Description = req.Description
	}
	if req.Status != "" {
		task.Status = req.Status
	}
	if req.Priority != "" {
		task.Priority = req.Priority
	}
	if req.DueDate != nil {
		task.DueDate = req.DueDate
	}
	if req.EstimatedHours > 0 {
		task.EstimatedHours = req.EstimatedHours
	}
	if req.HourlyRate > 0 {
		task.HourlyRate = req.HourlyRate
	}

	if err := h.db.Save(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Task updated successfully",
		"task":    task,
	})
}

// DeleteTask deletes a task
func (h *Handler) DeleteTask(c *gin.Context) {
	id := c.Param("id")

	var task models.Task
	if err := h.db.First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// Delete assignments and time logs
	h.db.Where("task_id = ?", id).Delete(&models.TaskAssignment{})
	h.db.Where("task_id = ?", id).Delete(&models.TimeLog{})

	if err := h.db.Delete(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task deleted successfully"})
}

// AssignTask assigns a user to a task
func (h *Handler) AssignTask(c *gin.Context) {
	taskID := c.Param("id")

	var task models.Task
	if err := h.db.First(&task, taskID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var req models.AssignTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userExists, err := h.userExists(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to validate user with user service"})
		return
	}
	if !userExists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}

	role := req.Role
	if role == "" {
		role = "assignee"
	}

	assignment := models.TaskAssignment{
		TaskID: task.ID,
		UserID: req.UserID,
		Role:   role,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		var existingAssignment models.TaskAssignment
		checkErr := tx.Where("task_id = ? AND user_id = ?", taskID, req.UserID).First(&existingAssignment).Error
		if checkErr == nil {
			return fmt.Errorf("assignment_already_exists")
		}
		if checkErr != nil && !errors.Is(checkErr, gorm.ErrRecordNotFound) {
			return checkErr
		}

		if err := tx.Create(&assignment).Error; err != nil {
			return err
		}

		return h.enqueueOutboxEvent(tx, "task.assigned", "task.assigned", map[string]interface{}{
			"task_id": task.ID,
			"title":   task.Title,
			"user_id": req.UserID,
			"role":    role,
		})
	})

	if err != nil {
		if err.Error() == "assignment_already_exists" {
			c.JSON(http.StatusConflict, gin.H{"error": "User is already assigned to this task"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign task"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Task assigned successfully",
		"assignment": assignment,
	})
}

// UnassignTask removes a user from a task
func (h *Handler) UnassignTask(c *gin.Context) {
	taskID := c.Param("id")
	userID := c.Param("user_id")

	var assignment models.TaskAssignment
	if err := h.db.Where("task_id = ? AND user_id = ?", taskID, userID).First(&assignment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
		return
	}

	if err := h.db.Delete(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unassign task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task unassigned successfully"})
}

// GetTaskStatus returns the status of a task
func (h *Handler) GetTaskStatus(c *gin.Context) {
	id := c.Param("id")

	var task models.Task
	if err := h.db.Preload("Assignments").Preload("TimeLogs").First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// Calculate total time spent
	var totalTime float64
	for _, log := range task.TimeLogs {
		totalTime += log.Hours
	}

	// Calculate progress
	var progress float64
	if task.EstimatedHours > 0 {
		progress = (totalTime / task.EstimatedHours) * 100
		if progress > 100 {
			progress = 100
		}
	}

	// Check if overdue
	var isOverdue bool
	if task.DueDate != nil && task.Status != "done" && task.Status != "cancelled" {
		isOverdue = time.Now().After(*task.DueDate)
	}

	status := models.TaskStatus{
		TaskID:         task.ID,
		Title:          task.Title,
		Status:         task.Status,
		Priority:       task.Priority,
		AssigneeCount:  len(task.Assignments),
		TotalTimeSpent: totalTime,
		EstimatedHours: task.EstimatedHours,
		Progress:       progress,
		DueDate:        task.DueDate,
		IsOverdue:      isOverdue,
	}

	c.JSON(http.StatusOK, status)
}

// UpdateTaskStatus updates only the status of a task
func (h *Handler) UpdateTaskStatus(c *gin.Context) {
	id := c.Param("id")

	var task models.Task
	if err := h.db.First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var req models.UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"todo":        true,
		"in_progress": true,
		"review":      true,
		"done":        true,
		"cancelled":   true,
	}
	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}

	task.Status = req.Status
	if err := h.db.Save(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Status updated successfully",
		"task":    task,
	})
}

// LogTime logs time spent on a task
func (h *Handler) LogTime(c *gin.Context) {
	taskID := c.Param("id")

	var task models.Task
	if err := h.db.First(&task, taskID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var req models.LogTimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}

	timeLog := models.TimeLog{
		TaskID:      task.ID,
		UserID:      userID.(uint),
		Hours:       req.Hours,
		Description: req.Description,
		LogDate:     logDate,
	}

	if err := h.db.Create(&timeLog).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log time"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Time logged successfully",
		"time_log": timeLog,
	})
}

// GetTimeLogs returns all time logs for a task
func (h *Handler) GetTimeLogs(c *gin.Context) {
	taskID := c.Param("id")

	var timeLogs []models.TimeLog
	if err := h.db.Where("task_id = ?", taskID).Order("log_date desc").Find(&timeLogs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch time logs"})
		return
	}

	c.JSON(http.StatusOK, timeLogs)
}

// CalculateTime calculates time metrics for a task
func (h *Handler) CalculateTime(c *gin.Context) {
	id := c.Param("id")

	var task models.Task
	if err := h.db.Preload("TimeLogs").First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// Calculate actual hours
	var actualHours float64
	for _, log := range task.TimeLogs {
		actualHours += log.Hours
	}

	// Calculate remaining and variance
	remainingHours := task.EstimatedHours - actualHours
	if remainingHours < 0 {
		remainingHours = 0
	}

	variance := task.EstimatedHours - actualHours // negative means over estimate

	calculation := models.TimeCalculation{
		TaskID:         task.ID,
		EstimatedHours: task.EstimatedHours,
		ActualHours:    actualHours,
		RemainingHours: remainingHours,
		Variance:       variance,
	}

	c.JSON(http.StatusOK, calculation)
}

// CalculatePrice calculates price metrics for a task
func (h *Handler) CalculatePrice(c *gin.Context) {
	id := c.Param("id")

	var task models.Task
	if err := h.db.Preload("TimeLogs").First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// Calculate actual hours
	var actualHours float64
	for _, log := range task.TimeLogs {
		actualHours += log.Hours
	}

	estimatedCost := task.EstimatedHours * task.HourlyRate
	actualCost := actualHours * task.HourlyRate
	variance := estimatedCost - actualCost // negative means over budget

	calculation := models.PriceCalculation{
		TaskID:         task.ID,
		HourlyRate:     task.HourlyRate,
		EstimatedHours: task.EstimatedHours,
		ActualHours:    actualHours,
		EstimatedCost:  estimatedCost,
		ActualCost:     actualCost,
		Variance:       variance,
	}

	c.JSON(http.StatusOK, calculation)
}

// CalculateProjectPrice calculates total price for all tasks in a project
func (h *Handler) CalculateProjectPrice(c *gin.Context) {
	projectID := c.Param("project_id")
	projectIDUint, _ := strconv.ParseUint(projectID, 10, 32)

	var tasks []models.Task
	if err := h.db.Preload("TimeLogs").Where("project_id = ?", projectID).Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
		return
	}

	var totalEstimatedHours, totalActualHours, totalEstimatedCost, totalActualCost float64
	var avgHourlyRate float64

	for _, task := range tasks {
		totalEstimatedHours += task.EstimatedHours
		totalEstimatedCost += task.EstimatedHours * task.HourlyRate
		avgHourlyRate += task.HourlyRate

		for _, log := range task.TimeLogs {
			totalActualHours += log.Hours
		}
		totalActualCost += totalActualHours * task.HourlyRate
	}

	if len(tasks) > 0 {
		avgHourlyRate /= float64(len(tasks))
	}

	calculation := models.PriceCalculation{
		ProjectID:      uint(projectIDUint),
		HourlyRate:     avgHourlyRate,
		EstimatedHours: totalEstimatedHours,
		ActualHours:    totalActualHours,
		EstimatedCost:  totalEstimatedCost,
		ActualCost:     totalActualCost,
		Variance:       totalEstimatedCost - totalActualCost,
	}

	c.JSON(http.StatusOK, calculation)
}
