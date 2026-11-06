package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type clientLimit struct {
	limiter *rate.Limiter
	seen    time.Time
}

func rateLimit() gin.HandlerFunc {
	var mu sync.Mutex
	clients := map[string]*clientLimit{}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		protected := c.Request.Method == http.MethodPost && (strings.HasPrefix(path, "/api/v1/setup/") || path == "/api/v1/users" || path == "/api/v1/auth/sessions" || path == "/api/v1/search-jobs" || strings.HasPrefix(path, "/api/v1/auth/password-reset")) || c.Request.Method == http.MethodGet && strings.HasPrefix(path, "/api/v1/movies/") && strings.HasSuffix(path, "/playbacks")
		if !protected {
			c.Next()
			return
		}
		now := time.Now()
		mu.Lock()
		if len(clients) >= 10000 {
			for key, value := range clients {
				if now.Sub(value.seen) > 10*time.Minute {
					delete(clients, key)
				}
			}
		}
		key := c.ClientIP()
		client := clients[key]
		if client == nil && len(clients) < 10000 {
			client = &clientLimit{limiter: rate.NewLimiter(rate.Every(6*time.Second), 10)}
			clients[key] = client
		}
		allowed := client != nil && client.limiter.Allow()
		if client != nil {
			client.seen = now
		}
		mu.Unlock()
		if !allowed {
			c.Header("Retry-After", "6")
			respondError(c, http.StatusTooManyRequests, "rate_limited")
			c.Abort()
			return
		}
		c.Next()
	}
}
