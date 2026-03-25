package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"notification-service/messaging"
	"notification-service/models"

	"github.com/gin-gonic/gin"
	"github.com/sony/gobreaker"
	"gorm.io/gorm"
)

type Handler struct {
	db             *gorm.DB
	userServiceURL string
	httpClient     *http.Client
	consumer       *messaging.Consumer
	userBreaker    *gobreaker.CircuitBreaker
}

func NewHandler(db *gorm.DB, userServiceURL string, consumer *messaging.Consumer) *Handler {
	if userServiceURL == "" {
		userServiceURL = getEnv("USER_SERVICE_URL", "http://user-service:8081")
	}

	userBreaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "notification-user-service",
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
		consumer:       consumer,
		userBreaker:    userBreaker,
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

type projectCreatedPayload struct {
	ProjectID uint   `json:"project_id"`
	Name      string `json:"name"`
	OwnerID   uint   `json:"owner_id"`
}

type projectMemberAddedPayload struct {
	ProjectID uint   `json:"project_id"`
	Project   string `json:"project"`
	UserID    uint   `json:"user_id"`
	Role      string `json:"role"`
}

type taskCreatedPayload struct {
	TaskID    uint   `json:"task_id"`
	Title     string `json:"title"`
	ProjectID uint   `json:"project_id"`
	CreatorID uint   `json:"creator_id"`
}

type taskAssignedPayload struct {
	TaskID uint   `json:"task_id"`
	Title  string `json:"title"`
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`
}

func (h *Handler) StartEventConsumer() {
	if h.consumer == nil {
		return
	}

	msgs, err := h.consumer.Consume()
	if err != nil {
		log.Printf("failed to consume rabbitmq messages: %v", err)
		return
	}

	go func() {
		for msg := range msgs {
			if err := h.handleEvent(msg.Body); err != nil {
				log.Printf("failed to handle event: %v", err)
				msg.Nack(false, true)
				continue
			}

			msg.Ack(false)
		}
	}()
}

func (h *Handler) handleEvent(body []byte) error {
	var event messaging.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}

	eventID := event.EventID
	if eventID == "" {
		hash := sha256.Sum256(body)
		eventID = hex.EncodeToString(hash[:])
	}

	return h.db.Transaction(func(tx *gorm.DB) error {
		var existing models.ProcessedEvent
		err := tx.Where("event_id = ?", eventID).First(&existing).Error
		if err == nil {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := h.applyEvent(tx, event, body); err != nil {
			return err
		}

		processed := models.ProcessedEvent{
			EventID:     eventID,
			EventType:   event.Type,
			Source:      event.Source,
			ProcessedAt: time.Now().UTC(),
		}
		return tx.Create(&processed).Error
	})
}

func (h *Handler) applyEvent(tx *gorm.DB, event messaging.Event, body []byte) error {
	switch event.Type {
	case "project.created":
		var payload projectCreatedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return h.createEventNotification(tx, payload.OwnerID, "Project Created", fmt.Sprintf("Project '%s' has been created", payload.Name), "project", string(body))
	case "project.member_added":
		var payload projectMemberAddedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return h.createEventNotification(tx, payload.UserID, "Added to Project", fmt.Sprintf("You were added to project '%s' as %s", payload.Project, payload.Role), "project", string(body))
	case "task.created":
		var payload taskCreatedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return h.createEventNotification(tx, payload.CreatorID, "Task Created", fmt.Sprintf("Task '%s' has been created", payload.Title), "task", string(body))
	case "task.assigned":
		var payload taskAssignedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return h.createEventNotification(tx, payload.UserID, "Task Assigned", fmt.Sprintf("You were assigned to task '%s'", payload.Title), "task", string(body))
	default:
		return nil
	}
}

func (h *Handler) createEventNotification(tx *gorm.DB, userID uint, title, message, notifType, data string) error {
	notification := models.Notification{
		UserID:  userID,
		Title:   title,
		Message: message,
		Type:    notifType,
		Read:    false,
		Data:    data,
	}

	return tx.Create(&notification).Error
}

func (h *Handler) SendNotification(c *gin.Context) {
	var req models.SendNotificationRequest
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

	var pref models.NotificationPreference
	if err := h.db.Where("user_id = ?", req.UserID).First(&pref).Error; err == nil {
		if !pref.AllowAll {
			c.JSON(http.StatusForbidden, gin.H{"error": "User has disabled notifications"})
			return
		}

		switch req.Type {
		case "task":
			if !pref.AllowTask {
				c.JSON(http.StatusForbidden, gin.H{"error": "User has disabled task notifications"})
				return
			}
		case "project":
			if !pref.AllowProject {
				c.JSON(http.StatusForbidden, gin.H{"error": "User has disabled project notifications"})
				return
			}
		}
	}

	notifType := req.Type
	if notifType == "" {
		notifType = "info"
	}

	notification := models.Notification{
		UserID:  req.UserID,
		Title:   req.Title,
		Message: req.Message,
		Type:    notifType,
		Read:    false,
		Data:    req.Data,
	}

	if err := h.db.Create(&notification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send notification"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":      "Notification sent successfully",
		"notification": notification,
	})
}

func (h *Handler) GetNotifications(c *gin.Context) {
	userID, _ := c.Get("user_id")
	unreadOnly := c.Query("unread") == "true"

	query := h.db.Where("user_id = ?", userID).Order("created_at desc")

	if unreadOnly {
		query = query.Where("read = false")
	}

	var notifications []models.Notification
	if err := query.Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications"})
		return
	}

	c.JSON(http.StatusOK, notifications)
}

func (h *Handler) GetNotification(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")

	var notification models.Notification
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&notification).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	c.JSON(http.StatusOK, notification)
}

func (h *Handler) MarkAsRead(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")

	var notification models.Notification
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&notification).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	now := time.Now()
	notification.Read = true
	notification.ReadAt = &now

	if err := h.db.Save(&notification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Marked as read",
		"notification": notification,
	})
}

func (h *Handler) MarkAllAsRead(c *gin.Context) {
	userID, _ := c.Get("user_id")

	now := time.Now()
	if err := h.db.Model(&models.Notification{}).
		Where("user_id = ? AND read = false", userID).
		Updates(map[string]interface{}{"read": true, "read_at": now}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark all as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All notifications marked as read"})
}

func (h *Handler) DeleteNotification(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")

	var notification models.Notification
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&notification).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	if err := h.db.Delete(&notification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete notification"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification deleted successfully"})
}

func (h *Handler) ScheduleNotification(c *gin.Context) {
	var req models.ScheduleNotificationRequest
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

	notifType := req.Type
	if notifType == "" {
		notifType = "info"
	}

	scheduled := models.ScheduledNotification{
		UserID:       req.UserID,
		Title:        req.Title,
		Message:      req.Message,
		Type:         notifType,
		ScheduledAt:  req.ScheduledAt,
		Recurring:    req.Recurring,
		CronSchedule: req.CronSchedule,
		Status:       "pending",
		Data:         req.Data,
	}

	if err := h.db.Create(&scheduled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to schedule notification"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":                "Notification scheduled successfully",
		"scheduled_notification": scheduled,
	})
}

func (h *Handler) GetScheduledNotifications(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var scheduled []models.ScheduledNotification
	if err := h.db.Where("user_id = ? AND status = 'pending'", userID).
		Order("scheduled_at asc").
		Find(&scheduled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch scheduled notifications"})
		return
	}

	c.JSON(http.StatusOK, scheduled)
}

func (h *Handler) UpdateScheduledNotification(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")

	var scheduled models.ScheduledNotification
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&scheduled).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Scheduled notification not found"})
		return
	}

	if scheduled.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot update non-pending notification"})
		return
	}

	var req models.UpdateScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title != "" {
		scheduled.Title = req.Title
	}
	if req.Message != "" {
		scheduled.Message = req.Message
	}
	if req.Type != "" {
		scheduled.Type = req.Type
	}
	if req.ScheduledAt != nil {
		scheduled.ScheduledAt = *req.ScheduledAt
	}
	scheduled.Recurring = req.Recurring
	if req.CronSchedule != "" {
		scheduled.CronSchedule = req.CronSchedule
	}
	if req.Data != "" {
		scheduled.Data = req.Data
	}

	if err := h.db.Save(&scheduled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update scheduled notification"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":                "Scheduled notification updated successfully",
		"scheduled_notification": scheduled,
	})
}

func (h *Handler) CancelScheduledNotification(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")

	var scheduled models.ScheduledNotification
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&scheduled).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Scheduled notification not found"})
		return
	}

	scheduled.Status = "cancelled"
	if err := h.db.Save(&scheduled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel scheduled notification"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Scheduled notification cancelled successfully"})
}

func (h *Handler) GetPreferences(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var pref models.NotificationPreference
	if err := h.db.Where("user_id = ?", userID).First(&pref).Error; err != nil {
		pref = models.NotificationPreference{
			UserID:        userID.(uint),
			AllowAll:      true,
			AllowTask:     true,
			AllowProject:  true,
			AllowSystem:   true,
			AllowReminder: true,
			EmailEnabled:  false,
			PushEnabled:   true,
		}
		h.db.Create(&pref)
	}

	c.JSON(http.StatusOK, pref)
}

func (h *Handler) UpdatePreferences(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var pref models.NotificationPreference
	if err := h.db.Where("user_id = ?", userID).First(&pref).Error; err != nil {
		pref = models.NotificationPreference{
			UserID:        userID.(uint),
			AllowAll:      true,
			AllowTask:     true,
			AllowProject:  true,
			AllowSystem:   true,
			AllowReminder: true,
			EmailEnabled:  false,
			PushEnabled:   true,
		}
		h.db.Create(&pref)
	}

	var req models.UpdatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.AllowAll != nil {
		pref.AllowAll = *req.AllowAll
	}
	if req.AllowTask != nil {
		pref.AllowTask = *req.AllowTask
	}
	if req.AllowProject != nil {
		pref.AllowProject = *req.AllowProject
	}
	if req.AllowSystem != nil {
		pref.AllowSystem = *req.AllowSystem
	}
	if req.AllowReminder != nil {
		pref.AllowReminder = *req.AllowReminder
	}
	if req.EmailEnabled != nil {
		pref.EmailEnabled = *req.EmailEnabled
	}
	if req.PushEnabled != nil {
		pref.PushEnabled = *req.PushEnabled
	}
	if req.QuietHoursStart != nil {
		pref.QuietHoursStart = req.QuietHoursStart
	}
	if req.QuietHoursEnd != nil {
		pref.QuietHoursEnd = req.QuietHoursEnd
	}

	if err := h.db.Save(&pref).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update preferences"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Preferences updated successfully",
		"preferences": pref,
	})
}

func (h *Handler) AllowNotifications(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req models.AllowNotificationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pref models.NotificationPreference
	if err := h.db.Where("user_id = ?", userID).First(&pref).Error; err != nil {
		pref = models.NotificationPreference{
			UserID:        userID.(uint),
			AllowAll:      req.Allow,
			AllowTask:     true,
			AllowProject:  true,
			AllowSystem:   true,
			AllowReminder: true,
			EmailEnabled:  false,
			PushEnabled:   true,
		}
		h.db.Create(&pref)
	} else {
		pref.AllowAll = req.Allow
		h.db.Save(&pref)
	}

	status := "enabled"
	if !req.Allow {
		status = "disabled"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Notifications " + status,
		"allow":   req.Allow,
	})
}
