package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/ingest"
	"github.com/libredmm/librefanza/backend/internal/playback"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

func registerPlaybackRoutes(api *gin.RouterGroup, provider *database.Provider) {
	service := playback.NewService()
	api.GET("/movies/:id/playbacks", func(c *gin.Context) {
		db, err := provider.Database(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		id := strings.ToUpper(c.Param("id"))
		if _, err := catalog.NewRepository(db).Movie(c.Request.Context(), id); err != nil {
			respondCatalogError(c, err)
			return
		}
		cfg, err := settings.NewService(db).Sources(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "settings_unavailable")
			return
		}
		body, referer, err := sources.FetchJablePage(c.Request.Context(), cfg, id)
		if err != nil {
			if strings.Contains(err.Error(), "HTTP 404") {
				c.JSON(http.StatusOK, gin.H{"data": []any{}})
			} else {
				respondError(c, http.StatusBadGateway, "playback_source_unavailable")
			}
			return
		}
		item, err := sources.ParsePage("jable", referer, body)
		if err != nil || item.Item.NormalizedID != id {
			respondError(c, http.StatusBadGateway, "playback_source_invalid")
			return
		}
		if err := ingest.NewService(db).Save(c.Request.Context(), item); err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		address, err := playback.ExtractJableStream(body)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"data": []any{}})
			return
		}
		token, err := service.Sign(address, referer, time.Now().Add(4*time.Hour))
		if err != nil {
			respondError(c, http.StatusBadGateway, "playback_source_invalid")
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"source": "jable", "direct_url": address, "relay_url": "/api/v1/playback/relay/" + token, "source_url": referer}}})
	})
	api.GET("/playback/relay/:token", func(c *gin.Context) {
		service.Relay(c.Writer, c.Request, c.Param("token"))
	})
}
