package main

import (
	"log"
	"os"
	"time"

	"user-service/handlers"
	"user-service/middleware"
	"user-service/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Database connection
	dsn := getEnv("DATABASE_URL", "host=user-db user=admin password=password dbname=user_db port=5432 sslmode=disable")

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
	db.AutoMigrate(&models.User{}, &models.Member{}, &models.Session{})

	// Setup handlers
	h := handlers.NewHandler(db)

	// Setup router
	r := gin.Default()

	// Public routes
	r.POST("/api/auth/register", h.Register)
	r.POST("/api/auth/login", h.Login)
	r.GET("/internal/users/:id", h.GetUserByID)

	// Protected routes
	auth := r.Group("/api")
	auth.Use(middleware.AuthMiddleware())
	{
		auth.POST("/auth/logout", h.Logout)
		auth.GET("/users/me", h.GetCurrentUser)

		// Member management
		auth.POST("/members", h.AddMember)
		auth.GET("/members", h.GetMembers)
		auth.GET("/members/:id", h.GetMember)
		auth.PUT("/members/:id", h.EditMember)
		auth.DELETE("/members/:id", h.DeleteMember)
	}

	port := getEnv("PORT", "8081")
	log.Printf("User service starting on port %s", port)
	r.Run(":" + port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
