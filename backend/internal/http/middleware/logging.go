package middleware

import (
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func StructuredLogger() gin.HandlerFunc {
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "wifi-man-api").Logger()
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info().
			Str("request_id", GetRequestID(c)).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("client_ip", c.ClientIP()).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(start)).
			Msg("http_request")
	}
}
