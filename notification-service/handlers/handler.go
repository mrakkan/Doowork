package handlers

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"notification-service/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	db             *gorm.DB
	userServiceURL string
	httpClient     *http.Client
}

func NewHandler(db *gorm.DB, userServiceURL string) *Handler {
	if userServiceURL == "" {
		userServiceURL = getEnv("USER_SERVICE_URL", "http://user-service:8081")
	}

	return &Handler{
		db:             db,
		userServiceURL: userServiceURL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (h *Handler) userExists(userID uint) (bool, error) {
	url := fmt.Sprintf("%s/internal/users/%d", h.userServiceURL, userID)
	resp, err := h.httpClient.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("user service returned status %d", resp.StatusCode)
	}

	return true, nil
}

// SendNotification sends a notification to a user
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

	// Check user preferences
	var pref models.NotificationPreference
	if err := h.db.Where("user_id = ?", req.UserID).First(&pref).Error; err == nil {
		if !pref.AllowAll {
			c.JSON(http.StatusForbidden, gin.H{"error": "User has disabled notifications"})
			return
		}

		// Check specific type preferences
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

// GetNotifications returns all notifications for the current user
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

// GetNotification returns a specific notification
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

// MarkAsRead marks a notification as read
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

// MarkAllAsRead marks all notifications as read for the current user
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

// DeleteNotification deletes a notification
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

// ScheduleNotification schedules a notification for later
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

// GetScheduledNotifications returns all scheduled notifications for the current user
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

// UpdateScheduledNotification updates a scheduled notification
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

// CancelScheduledNotification cancels a scheduled notification
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

// GetPreferences returns notification preferences for the current user
func (h *Handler) GetPreferences(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var pref models.NotificationPreference
	if err := h.db.Where("user_id = ?", userID).First(&pref).Error; err != nil {
		// Create default preferences
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

// UpdatePreferences updates notification preferences
func (h *Handler) UpdatePreferences(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var pref models.NotificationPreference
	if err := h.db.Where("user_id = ?", userID).First(&pref).Error; err != nil {
		// Create if not exists
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

// AllowNotifications enables or disables all notifications
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
