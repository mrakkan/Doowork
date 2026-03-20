package monitoring

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "doowork_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"service", "method", "path", "status"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "doowork_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method", "path"},
	)
)

func SetupMetrics(r *gin.Engine, service string) {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration)

	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		httpRequestsTotal.WithLabelValues(service, c.Request.Method, path, strconv.Itoa(c.Writer.Status())).Inc()
		httpRequestDuration.WithLabelValues(service, c.Request.Method, path).Observe(time.Since(start).Seconds())
	})

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}
