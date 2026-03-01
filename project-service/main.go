package main

import (
	"log"
	"os"
	"time"

	"project-service/handlers"
	"project-service/middleware"
	"project-service/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Database connection
	dsn := getEnv("DATABASE_URL", "host=project-db user=admin password=password dbname=project_db port=5432 sslmode=disable")

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
	db.AutoMigrate(&models.Project{}, &models.ProjectMember{})

	// Setup handlers
	h := handlers.NewHandler(db, getEnv("USER_SERVICE_URL", "http://user-service:8081"))

	// Setup router
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/internal/projects/:id", h.GetProjectInternal)

	// Protected routes
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		// Project CRUD
		api.POST("/projects", h.CreateProject)
		api.GET("/projects", h.GetProjects)
		api.GET("/projects/:id", h.GetProject)
		api.PUT("/projects/:id", h.UpdateProject)
		api.DELETE("/projects/:id", h.DeleteProject)

		// Project status
		api.GET("/projects/:id/status", h.GetProjectStatus)

		// Project members
		api.POST("/projects/:id/members", h.AddProjectMember)
		api.GET("/projects/:id/members", h.GetProjectMembers)
		api.DELETE("/projects/:id/members/:member_id", h.RemoveProjectMember)
	}

	port := getEnv("PORT", "8082")
	log.Printf("Project service starting on port %s", port)
	r.Run(":" + port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
