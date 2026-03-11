package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RateLimit(client *redis.Client, maxRequests int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxRequests <= 0 {
			c.Next()
			return
		}
		key := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), c.ClientIP())
		pipe := client.TxPipeline()
		countCmd := pipe.Incr(c.Request.Context(), key)
		pipe.Expire(c.Request.Context(), key, window)
		if _, err := pipe.Exec(c.Request.Context()); err != nil {
			c.Next()
			return
		}
		if countCmd.Val() > int64(maxRequests) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		c.Next()
	}
}
