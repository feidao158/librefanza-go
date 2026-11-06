package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/auth"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/libredmm/librefanza/backend/internal/tasks"
)

type TaskService interface {
	Enqueue(context.Context, string, bool) (string, error)
	Get(context.Context, string) (tasks.Status, error)
	EnqueueMaintenance(context.Context, string) (string, error)
}

func RegisterTaskRoutes(router *gin.Engine, provider *database.Provider, taskService func(context.Context) (TaskService, error)) {
	api := router.Group("/api/v1")
	api.POST("/search-jobs", func(c *gin.Context) {
		var request struct {
			ID    string `json:"id"`
			Force bool   `json:"force"`
		}
		if c.ShouldBindJSON(&request) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if request.Force && !requireAdmin(c, provider) {
			return
		}
		service, err := taskService(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "tasks_unavailable")
			return
		}
		id, err := service.Enqueue(c.Request.Context(), request.ID, request.Force)
		if err != nil {
			if errors.Is(err, tasks.ErrInvalidID) {
				respondError(c, http.StatusBadRequest, "invalid_product_id")
			} else {
				respondError(c, http.StatusInternalServerError, "task_create_failed")
			}
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"id": id}})
	})
	api.GET("/search-jobs/:id", func(c *gin.Context) {
		service, err := taskService(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "tasks_unavailable")
			return
		}
		status, err := service.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			if errors.Is(err, tasks.ErrJobNotFound) {
				respondError(c, http.StatusNotFound, "not_found")
			} else {
				respondError(c, http.StatusServiceUnavailable, "tasks_unavailable")
			}
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": status})
	})
	api.POST("/admin/maintenance-jobs", func(c *gin.Context) {
		if !requireAdmin(c, provider) {
			return
		}
		var input struct {
			Type string `json:"type"`
		}
		if c.ShouldBindJSON(&input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		svc, err := taskService(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "tasks_unavailable")
			return
		}
		id, err := svc.EnqueueMaintenance(c.Request.Context(), input.Type)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"id": id}})
	})
	api.POST("/admin/crawl-jobs", func(c *gin.Context) {
		if !requireAdmin(c, provider) {
			return
		}
		var request sources.CrawlRequest
		if c.ShouldBindJSON(&request) != nil || request.Validate() != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		service, err := taskService(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "tasks_unavailable")
			return
		}
		crawler, ok := service.(interface {
			EnqueueCrawl(context.Context, sources.CrawlRequest) (string, error)
		})
		if !ok {
			respondError(c, http.StatusServiceUnavailable, "tasks_unavailable")
			return
		}
		id, err := crawler.EnqueueCrawl(c.Request.Context(), request)
		if errors.Is(err, tasks.ErrCrawlInProgress) {
			respondError(c, http.StatusConflict, "crawl_in_progress")
			return
		}
		if err != nil {
			respondError(c, http.StatusInternalServerError, "task_create_failed")
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"id": id}})
	})
}

func requireAdmin(c *gin.Context, provider *database.Provider) bool {
	if provider == nil || bearer(c) == "" {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return false
	}
	db, err := provider.Database(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, "database_unavailable")
		return false
	}
	user, err := auth.NewService(db).Authenticate(c.Request.Context(), bearer(c))
	if err != nil {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if !user.IsAdmin {
		respondError(c, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}
