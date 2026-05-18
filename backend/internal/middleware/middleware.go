// backend/internal/middleware/middleware.go
package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Options struct {
	AuthToken   string
	CORSOrigins string
}

func SetupMiddleware(r *gin.Engine, opts ...Options) {
	options := Options{}
	if len(opts) > 0 {
		options = opts[0]
	}

	origins := parseOrigins(options.CORSOrigins)
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
		if options.AuthToken == "" {
			origins = []string{"*"}
		}
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: options.AuthToken != "",
		MaxAge:           12 * time.Hour,
	}))

	r.Use(gin.Recovery())
	if options.AuthToken != "" {
		r.Use(requireBearerToken(options.AuthToken))
	}

	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s %d %v", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
	})
}

func requireBearerToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions || c.Request.URL.Path == "/health" || strings.HasPrefix(c.Request.URL.Path, "/ws/") {
			c.Next()
			return
		}
		if c.GetHeader("Authorization") != "Bearer "+token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func parseOrigins(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
