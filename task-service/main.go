package main

import (
	"context"
	"log"
	"os"
	"task-service/messaging"
	"task-service/monitoring"
	"time"

	"task-service/handlers"
	"task-service/middleware"
	"task-service/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Database connection
	dsn := getEnv("DATABASE_URL", "host=task-db user=admin password=password dbname=task_db port=5432 sslmode=disable")

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
	db.AutoMigrate(&models.Task{}, &models.TaskAssignment{}, &models.TimeLog{}, &models.OutboxEvent{})

	// Setup handlers
	amqpURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/")
	publisher, err := messaging.ConnectWithRetry(amqpURL, 10, 3*time.Second)
	if err != nil {
		log.Printf("RabbitMQ unavailable, continuing without event publish: %v", err)
	}
	defer func() {
		if publisher != nil {
			publisher.Close()
		}
	}()

	h := handlers.NewHandler(
		db,
		getEnv("USER_SERVICE_URL", "http://user-service:8081"),
		getEnv("PROJECT_SERVICE_URL", "http://project-service:8082"),
	)

	if publisher != nil {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		processor := messaging.NewOutboxProcessor(db, publisher, "task-service")
		go processor.Start(ctx, 2*time.Second)
	}

	// Setup router
	r := gin.Default()
	monitoring.SetupMetrics(r, "task-service")

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Protected routes
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		// Task CRUD
		api.POST("/tasks", h.CreateTask)
		api.GET("/tasks", h.GetTasks)
		api.GET("/tasks/:id", h.GetTask)
		api.PUT("/tasks/:id", h.UpdateTask)
		api.DELETE("/tasks/:id", h.DeleteTask)

		// Task assignment
		api.POST("/tasks/:id/assign", h.AssignTask)
		api.DELETE("/tasks/:id/assign/:user_id", h.UnassignTask)

		// Task status
		api.GET("/tasks/:id/status", h.GetTaskStatus)
		api.PUT("/tasks/:id/status", h.UpdateTaskStatus)

		// Time tracking
		api.POST("/tasks/:id/time", h.LogTime)
		api.GET("/tasks/:id/time", h.GetTimeLogs)
		api.GET("/tasks/:id/calculate-time", h.CalculateTime)

		// Price calculation
		api.GET("/tasks/:id/calculate-price", h.CalculatePrice)
		api.GET("/projects/:project_id/calculate-price", h.CalculateProjectPrice)
	}

	port := getEnv("PORT", "8083")
	log.Printf("Task service starting on port %s", port)
	r.Run(":" + port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
