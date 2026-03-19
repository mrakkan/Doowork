package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

type Gateway struct {
	userProxy         *httputil.ReverseProxy
	projectProxy      *httputil.ReverseProxy
	taskProxy         *httputil.ReverseProxy
	notificationProxy *httputil.ReverseProxy
}

func newReverseProxy(rawURL string) *httputil.ReverseProxy {
	targetURL, err := url.Parse(rawURL)
	if err != nil {
		log.Fatalf("invalid target url %s: %v", rawURL, err)
	}
	return httputil.NewSingleHostReverseProxy(targetURL)
}

func NewGateway() *Gateway {
	userURL := getEnv("USER_SERVICE_URL", "http://user-service:8081")
	projectURL := getEnv("PROJECT_SERVICE_URL", "http://project-service:8082")
	taskURL := getEnv("TASK_SERVICE_URL", "http://task-service:8083")
	notificationURL := getEnv("NOTIFICATION_SERVICE_URL", "http://notification-service:8084")

	return &Gateway{
		userProxy:         newReverseProxy(userURL),
		projectProxy:      newReverseProxy(projectURL),
		taskProxy:         newReverseProxy(taskURL),
		notificationProxy: newReverseProxy(notificationURL),
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasPrefix(path, "/api/auth"):
		g.userProxy.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/api/users"):
		g.userProxy.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/api/members"):
		g.userProxy.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/internal/users"):
		g.userProxy.ServeHTTP(w, r)

	case strings.HasPrefix(path, "/api/notifications"):
		g.notificationProxy.ServeHTTP(w, r)

	case strings.HasPrefix(path, "/api/tasks"):
		g.taskProxy.ServeHTTP(w, r)

	case strings.HasPrefix(path, "/api/projects") && strings.HasSuffix(path, "/calculate-price"):
		g.taskProxy.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/api/projects"):
		g.projectProxy.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/internal/projects"):
		g.projectProxy.ServeHTTP(w, r)

	case path == "/health":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"gateway"}`))
	case path == "/":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Doowork API Gateway","port":8000}`))
	default:
		http.NotFound(w, r)
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	port := getEnv("PORT", "8000")
	gateway := NewGateway()

	server := &http.Server{
		Addr:    ":" + port,
		Handler: gateway,
	}

	log.Printf("Gateway service starting on port %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("gateway failed to start: %v", err)
	}
}
