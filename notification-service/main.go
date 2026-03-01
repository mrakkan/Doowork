package main

import (
	"log"
	"os"
	"time"

	"notification-service/handlers"
	"notification-service/middleware"
	"notification-service/models"
	"notification-service/scheduler"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Database connection
	dsn := getEnv("DATABASE_URL", "host=notification-db user=admin password=password dbname=notification_db port=5432 sslmode=disable")

	var db *gorm.DB
	var err error

	// Retry connection
	for i := 0; i < 10; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database, retrying in 3 seconds... (%d/10)", i+1)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto migrate
	db.AutoMigrate(&models.Notification{}, &models.ScheduledNotification{}, &models.NotificationPreference{})

	// Setup scheduler for scheduled notifications
	sched := scheduler.NewScheduler(db)
	sched.Start()
	defer sched.Stop()

	// Setup handlers
	h := handlers.NewHandler(db, getEnv("USER_SERVICE_URL", "http://user-service:8081"))

	// Setup router
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Protected routes
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		// Notifications
		api.POST("/notifications", h.SendNotification)
		api.GET("/notifications", h.GetNotifications)
		api.GET("/notifications/:id", h.GetNotification)
		api.PUT("/notifications/:id/read", h.MarkAsRead)
		api.PUT("/notifications/read-all", h.MarkAllAsRead)
		api.DELETE("/notifications/:id", h.DeleteNotification)

		// Scheduled notifications
		api.POST("/notifications/schedule", h.ScheduleNotification)
		api.GET("/notifications/scheduled", h.GetScheduledNotifications)
		api.PUT("/notifications/scheduled/:id", h.UpdateScheduledNotification)
		api.DELETE("/notifications/scheduled/:id", h.CancelScheduledNotification)

		// Notification preferences
		api.GET("/notifications/preferences", h.GetPreferences)
		api.PUT("/notifications/preferences", h.UpdatePreferences)
		api.PUT("/notifications/allow", h.AllowNotifications)
	}

	port := getEnv("PORT", "8084")
	log.Printf("Notification service starting on port %s", port)
	r.Run(":" + port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
