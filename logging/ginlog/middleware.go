// Package ginlog provides Gin access logging and panic recovery without payload logging.
package ginlog

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ubiwdotspace/go-libs/logging"
	"github.com/ubiwdotspace/go-libs/logging/internal/requestlog"
)

// New logs each completed request. Install Recovery after this middleware.
// Successful /healthz and /api/v1/healthz probes log at DEBUG.
func New(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		incoming := ""
		if values := c.Request.Header.Values("X-Request-ID"); len(values) == 1 {
			incoming = values[0]
		}
		ctx := logging.WithRequest(c.Request.Context(), logger.With("transport", "http"), incoming)
		requestLogger := logging.FromContext(ctx)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", logging.RequestID(ctx))
		started := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "<unmatched>"
		}
		status := c.Writer.Status()
		attrs := []slog.Attr{slog.String("method", c.Request.Method), slog.String("route", route), slog.Int("status", status), slog.Float64("duration_ms", float64(time.Since(started).Microseconds())/1000)}
		if err := c.Errors.Last(); err != nil {
			attrs = append(attrs, slog.Any("error", err.Err))
		}
		level := requestlog.HTTPLevel(status)
		if (route == "/healthz" || route == "/api/v1/healthz") && status < 400 {
			level = slog.LevelDebug
		}
		requestLogger.LogAttrs(c.Request.Context(), level, "HTTP request completed", attrs...)
	}
}

// Recovery converts panics to HTTP 500 and logs only stack locations.
// Place it after New and before authentication and application handlers.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		logging.FromContext(c.Request.Context()).ErrorContext(c.Request.Context(), "HTTP panic recovered", "stack", requestlog.Stack())
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
