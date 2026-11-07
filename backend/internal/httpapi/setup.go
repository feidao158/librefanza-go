package httpapi

import (
	_ "embed"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/cache"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

//go:embed openapi.yaml
var openAPIDocument []byte

func NewRouter(service *setup.Service, providers ...*database.Provider) *gin.Engine {
	router := gin.New()
	var proxies []string
	for _, value := range strings.Split(os.Getenv("LIBREFANZA_TRUSTED_PROXIES"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			if value == "0.0.0.0/0" || value == "::/0" {
				panic("trusted proxy range is too broad")
			}
			proxies = append(proxies, value)
		}
	}
	if err := router.SetTrustedProxies(proxies); err != nil {
		panic("invalid trusted proxy configuration")
	}
	router.Use(gin.Recovery())
	router.Use(rateLimit())
	router.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
		c.Next()
	})
	api := router.Group("/api/v1")
	api.GET("/openapi.yaml", func(c *gin.Context) { c.Data(http.StatusOK, "application/yaml; charset=utf-8", openAPIDocument) })
	api.GET("/setup", func(c *gin.Context) {
		installed, err := service.Installed()
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "configuration_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"installed": installed}})
	})
	api.POST("/setup/database-checks", func(c *gin.Context) {
		var request config.Database
		if err := c.ShouldBindJSON(&request); err != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := service.CheckDatabase(c.Request.Context(), request); err != nil {
			respondSetupError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"connected": true}})
	})
	api.POST("/setup/redis-checks", func(c *gin.Context) {
		var request setup.Redis
		if err := c.ShouldBindJSON(&request); err != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := service.CheckRedis(c.Request.Context(), request); err != nil {
			respondSetupError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"connected": true}})
	})
	api.POST("/setup/installations", func(c *gin.Context) {
		var request setup.Request
		if err := c.ShouldBindJSON(&request); err != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := service.Initialize(c.Request.Context(), request); err != nil {
			respondSetupError(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": gin.H{"installed": true}})
	})
	if len(providers) > 0 {
		registerAuthRoutes(api, providers[0])
		registerCatalogRoutes(api, providers[0], cache.NewManager(providers[0]))
		registerPlaybackRoutes(api, providers[0])
		registerSettingsRoutes(api, providers[0])
		registerAdminRoutes(api, providers[0])
		registerRecommendationRoutes(api, providers[0])
	}
	return router
}

func respondSetupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, setup.ErrAlreadyInstalled), errors.Is(err, setup.ErrPendingMismatch):
		respondError(c, http.StatusConflict, "setup_conflict")
	case errors.Is(err, setup.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "invalid_setup")
	default:
		respondError(c, http.StatusUnprocessableEntity, "setup_failed")
	}
}

func respondError(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code}})
}
