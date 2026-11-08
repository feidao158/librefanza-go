package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/mailer"
	"github.com/libredmm/librefanza/backend/internal/recommendation"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"github.com/libredmm/librefanza/backend/internal/setup"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/libredmm/librefanza/backend/internal/tasks"
)

func checkGorseHealth(ctx context.Context, endpoint, apiKey string) bool {
	if endpoint == "" {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/health/ready", nil)
	if err != nil {
		return false
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		Ready bool `json:"Ready"`
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&health) == nil && health.Ready
}

func registerSettingsRoutes(api *gin.RouterGroup, provider *database.Provider) {
	service := func(c *gin.Context) *settings.Service {
		if !requireAdmin(c, provider) {
			return nil
		}
		db, err := provider.Database(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return nil
		}
		return settings.NewService(db)
	}
	api.GET("/admin/settings/sources", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		value, err := svc.Sources(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		health, err := svc.SourceHealth(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"fanza_configured": value.FanzaAPIID != "" && value.FanzaAffiliateID != "", "proxy_configured": value.ProxyURL != "", "fc2_url": value.FC2URL, "sod_url": value.SODURL, "mgstage_url": value.MGStageURL, "mgstage_series_url": value.MGStageSeriesURL, "jable_url": value.JableURL, "libredmm_url": value.LibreDMMURL, "javdb_url": value.JavDBURL, "javbus_url": value.JavBusURL, "javbooks_url": value.JavBooksURL, "javquery_url": value.JavQueryURL, "jable_cookie_configured": value.JableCookie != "", "javdb_cookie_configured": value.JavDBCookie != "", "javbus_cookie_configured": value.JavBusCookie != "", "javbooks_cookie_configured": value.JavBooksCookie != "", "javquery_cookie_configured": value.JavQueryCookie != "", "health": health}})
	})
	api.PUT("/admin/settings/sources", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var input sources.ClientSettings
		if c.ShouldBindJSON(&input) != nil || svc.UpdateSources(c.Request.Context(), input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_settings")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
	api.GET("/admin/settings/redis", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		value, err := svc.Redis(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": value.Enabled, "address": value.Address, "username": value.Username, "password_configured": value.Password != "", "database": value.Database, "tls": value.TLS}})
	})
	api.PUT("/admin/settings/redis", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var input config.Redis
		if c.ShouldBindJSON(&input) != nil || svc.UpdateRedis(c.Request.Context(), input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_settings")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
	api.POST("/admin/settings/redis-checks", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var input config.Redis
		if c.ShouldBindJSON(&input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		previous, err := svc.Redis(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		if input.ClearPassword {
			input.Password = ""
		} else if input.Password == "" {
			input.Password = previous.Password
		}
		input.Enabled = true
		if err := setup.NewDatabaseBackend().CheckRedis(c.Request.Context(), input); err != nil {
			respondError(c, http.StatusUnprocessableEntity, "redis_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"connected": true}})
	})
	api.GET("/admin/settings/schedules", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		value, err := svc.Schedules(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": value})
	})
	api.PUT("/admin/settings/schedules", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var input json.RawMessage
		previous, err := svc.Schedules(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		if c.ShouldBindJSON(&input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_settings")
			return
		}
		value, err := tasks.MergeScheduleSettings(previous, input)
		if err != nil || svc.UpdateSchedules(c.Request.Context(), value) != nil {
			respondError(c, http.StatusBadRequest, "invalid_settings")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
	api.GET("/admin/schedule-previews", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		settings, err := svc.Schedules(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		next, err := tasks.NextSchedules(settings, time.Now())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": next})
	})
	api.GET("/admin/settings/smtp", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		value, err := svc.SMTP(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": value.Enabled, "host": value.Host, "port": value.Port, "username": value.Username, "password_configured": value.Password != "", "from": value.From, "base_url": value.BaseURL, "tls_mode": value.TLSMode}})
	})
	api.PUT("/admin/settings/smtp", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var value mailer.Settings
		if c.ShouldBindJSON(&value) != nil || svc.UpdateSMTP(c.Request.Context(), value) != nil {
			respondError(c, http.StatusBadRequest, "invalid_settings")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
	api.GET("/admin/settings/gorse", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		value, err := svc.Gorse(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "settings_unavailable")
			return
		}
		data := gin.H{"entry_point": value.EntryPoint, "api_key_configured": value.APIKey != "", "enabled": value.Enabled, "min_feedback_count": value.MinFeedbackCount}
		if os.Getenv("LIBREFANZA_MANAGED_GORSE") == "true" {
			db, err := provider.Database(c.Request.Context())
			if err != nil {
				respondError(c, http.StatusServiceUnavailable, "database_unavailable")
				return
			}
			progress, err := recommendation.ManagedSyncStatus(c.Request.Context(), db)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "settings_unavailable")
				return
			}
			data["managed"] = true
			data["sync"] = progress
			healthy, err := recommendation.ManagedSyncHealthy(c.Request.Context(), db)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "settings_unavailable")
				return
			}
			data["sync_error"] = !healthy
			data["connected"] = value.Enabled && checkGorseHealth(c.Request.Context(), value.EntryPoint, value.APIKey)
		}
		c.JSON(http.StatusOK, gin.H{"data": data})
	})
	api.PUT("/admin/settings/gorse", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var value settings.GorseSettings
		if c.ShouldBindJSON(&value) != nil || svc.UpdateGorse(c.Request.Context(), value) != nil {
			respondError(c, http.StatusBadRequest, "invalid_settings")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
	api.POST("/admin/settings/gorse-checks", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var input struct {
			EntryPoint string `json:"entry_point"`
			APIKey     string `json:"api_key"`
		}
		if c.ShouldBindJSON(&input) != nil || input.EntryPoint == "" {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if input.APIKey == "" {
			previous, err := svc.Gorse(c.Request.Context())
			if err != nil {
				respondError(c, http.StatusInternalServerError, "settings_unavailable")
				return
			}
			if input.EntryPoint == previous.EntryPoint {
				input.APIKey = previous.APIKey
			}
		}
		if !checkGorseHealth(c.Request.Context(), input.EntryPoint, input.APIKey) {
			respondError(c, http.StatusBadGateway, "gorse_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"connected": true}})
	})
}
