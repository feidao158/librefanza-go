package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/mailer"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"github.com/libredmm/librefanza/backend/internal/setup"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/libredmm/librefanza/backend/internal/tasks"
)

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
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"fanza_configured": value.FanzaAPIID != "" && value.FanzaAffiliateID != "", "proxy_configured": value.ProxyURL != "", "fc2_url": value.FC2URL, "sod_url": value.SODURL, "mgstage_url": value.MGStageURL, "mgstage_series_url": value.MGStageSeriesURL, "jable_url": value.JableURL, "avmoo_url": value.AVMOOURL, "javlibrary_url": value.JavLibraryURL, "libredmm_url": value.LibreDMMURL, "jable_cookie_configured": value.JableCookie != "", "javlibrary_cookie_configured": value.JavLibraryCookie != ""}})
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
		var value tasks.ScheduleSettings
		if c.ShouldBindJSON(&value) != nil || svc.UpdateSchedules(c.Request.Context(), value) != nil {
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
}
