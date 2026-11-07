package httpapi

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/cache"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/ingest"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/libredmm/librefanza/backend/internal/tasks"
	"gorm.io/gorm"
)

type adminPage struct {
	ID        uint64    `json:"id"`
	Source    string    `json:"source"`
	URL       string    `json:"url"`
	RawHTML   string    `json:"raw_html,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func registerAdminRoutes(api *gin.RouterGroup, provider *database.Provider) {
	databaseForAdmin := func(c *gin.Context) *gorm.DB {
		if !requireAdmin(c, provider) {
			return nil
		}
		db, err := provider.Database(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return nil
		}
		return db.WithContext(c.Request.Context())
	}
	api.GET("/admin/logs", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		page, size := pageParams(c)
		entries, total, err := logbook.List(c.Request.Context(), db, logbook.Filter{Level: c.Query("level"), Source: c.Query("source"), TaskID: c.Query("task_id"), Page: page, PageSize: size})
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": entries, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
	api.GET("/admin/jobs", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		page, size := pageParams(c)
		query := db.Model(&jobs.Job{})
		if c.Query("kind") == "collection" {
			query = query.Where("type IN ?", []string{"source_crawl", "actress_crawl", "fanza_crawl", "mgstage_crawl", "metadata_backfill"})
		} else if c.Query("kind") == "metadata" {
			query = query.Where("type IN ?", []string{"metadata_enrich", "metadata_backfill"})
		}
		if status := c.Query("status"); status != "" {
			query = query.Where("status = ?", status)
		}
		query = query.Where("type NOT IN ?", []string{"maintenance_tick", "metadata_backfill_tick"})
		var total int64
		var entries []jobs.Job
		if query.Count(&total).Error != nil || query.Order("created_at DESC").Offset((page-1)*size).Limit(size).Find(&entries).Error != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		result := make([]gin.H, 0, len(entries))
		ids := make([]string, 0, len(entries))
		metadataIDs := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.Type == "source_crawl" {
				ids = append(ids, entry.ID.String())
			}
			if entry.Type == "metadata_enrich" {
				metadataIDs = append(metadataIDs, entry.ID.String())
			}
		}
		metadataFailures := make(map[string][]tasks.MetadataSourceProgress)
		if len(metadataIDs) > 0 {
			var rows []tasks.MetadataSourceProgress
			if err := db.Where("job_id IN ? AND status = ?", metadataIDs, "failed").Find(&rows).Error; err != nil {
				respondError(c, http.StatusInternalServerError, "internal_error")
				return
			}
			for _, row := range rows {
				metadataFailures[row.JobID] = append(metadataFailures[row.JobID], row)
			}
		}
		progress := make(map[string]tasks.CrawlProgress, len(ids))
		if len(ids) > 0 {
			var rows []tasks.CrawlProgress
			if err := db.Where("job_id IN ?", ids).Find(&rows).Error; err != nil {
				respondError(c, http.StatusInternalServerError, "internal_error")
				return
			}
			for _, row := range rows {
				progress[row.JobID] = row
			}
		}
		for _, entry := range entries {
			value := gin.H{"id": entry.ID.String(), "type": entry.Type, "status": entry.Status, "attempt": entry.Attempt, "created_at": entry.CreatedAt, "started_at": entry.StartedAt, "completed_at": entry.CompletedAt}
			if entry.Type == "metadata_enrich" {
				value["metadata_failures"] = metadataFailures[entry.ID.String()]
				var request tasks.SearchArgs
				if json.Unmarshal(entry.Args, &request) == nil {
					value["metadata_movie_id"] = request.ID
				}
			}
			if entry.Type == "source_crawl" {
				var request sources.CrawlRequest
				if json.Unmarshal(entry.Args, &request) == nil {
					value["crawl"] = request
				}
				if row, ok := progress[entry.ID.String()]; ok {
					value["progress"] = row
				}
				if entry.LastError != "" {
					value["error_code"] = crawlErrorCode(entry.LastError)
				}
			}
			result = append(result, value)
		}
		c.JSON(http.StatusOK, gin.H{"data": result, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
	api.GET("/admin/metadata-backfill", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		var progress tasks.MetadataBackfillProgress
		if err := db.First(&progress, 1).Error; err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		var failed int64
		if err := db.Model(&jobs.Job{}).Where("type = ? AND status = ?", "metadata_enrich", "failed").Count(&failed).Error; err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"progress": progress, "failed_jobs": failed}})
	})
	api.POST("/admin/jobs/:id/retries", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		id, err := jobs.ParseUUID(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		var existing jobs.Job
		if err := db.WithContext(c.Request.Context()).Where("id = ?", id).First(&existing).Error; err != nil {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		if existing.Type == "source_crawl" {
			var request sources.CrawlRequest
			if json.Unmarshal(existing.Args, &request) == nil && errors.Is(request.Validate(), sources.ErrSourceRetired) {
				respondError(c, http.StatusConflict, "source_retired")
				return
			}
		}
		if _, err := jobs.NewGormStorage(db).RetryJob(c.Request.Context(), id); err != nil {
			respondError(c, http.StatusConflict, "retry_failed")
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"id": id.String()}})
	})
	api.GET("/admin/source-items", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		page, size := pageParams(c)
		query := db.Model(&catalog.SourceItem{})
		if source := c.Query("source"); source != "" {
			query = query.Where("source = ?", source)
		}
		var total int64
		var items []catalog.SourceItem
		if query.Count(&total).Error != nil || query.Order("id DESC").Offset((page-1)*size).Limit(size).Find(&items).Error != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
	api.GET("/admin/source-pages", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		page, size := pageParams(c)
		query := db.Table("source_pages")
		if source := c.Query("source"); source != "" {
			query = query.Where("source = ?", source)
		}
		var total int64
		var pages []adminPage
		if query.Count(&total).Error != nil || query.Select("id", "source", "url", "created_at", "updated_at").Order("id DESC").Offset((page-1)*size).Limit(size).Scan(&pages).Error != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": pages, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
	api.PATCH("/admin/actresses/:id", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		var input struct {
			IsHidden *bool `json:"is_hidden"`
		}
		if c.ShouldBindJSON(&input) != nil || input.IsHidden == nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			var actress catalog.Actress
			if err := tx.First(&actress, c.Param("id")).Error; err != nil {
				return err
			}
			if err := tx.Model(&actress).Update("is_hidden", *input.IsHidden).Error; err != nil {
				return err
			}
			return cache.BumpVersion(tx)
		})
		if err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
	api.GET("/admin/source-pages/:id", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		var entry adminPage
		if err := db.Table("source_pages").Where("id = ?", c.Param("id")).First(&entry).Error; err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": entry})
	})
	api.POST("/admin/source-pages", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		var input struct {
			Source  string `json:"source"`
			URL     string `json:"url"`
			RawHTML string `json:"raw_html"`
		}
		if c.ShouldBindJSON(&input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if !validPublicSourceURL(input.URL) {
			respondError(c, http.StatusBadRequest, "invalid_url")
			return
		}
		parsed, err := sources.ParsePage(input.Source, input.URL, []byte(input.RawHTML))
		if err != nil {
			respondError(c, http.StatusUnprocessableEntity, "invalid_source_page")
			return
		}
		if err := ingest.NewService(db).Save(c.Request.Context(), parsed); err != nil {
			respondError(c, http.StatusInternalServerError, "import_failed")
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": gin.H{"normalized_id": parsed.Item.NormalizedID}})
	})
	api.PATCH("/admin/source-items/:id", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		var input struct {
			Priority *int `json:"priority"`
		}
		if err != nil || c.ShouldBindJSON(&input) != nil || input.Priority == nil || *input.Priority < -10000 || *input.Priority > 10000 {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := ingest.NewService(db).SetPriority(c.Request.Context(), id, *input.Priority); err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
	api.POST("/admin/source-items/:id/rederivations", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		var item catalog.SourceItem
		if err := db.First(&item, c.Param("id")).Error; err != nil {
			respondCatalogError(c, err)
			return
		}
		var parsed sources.Parsed
		var err error
		if item.Source == "fanza" {
			parsed, err = sources.ParseFanza([]byte(item.RawJSON))
		} else if item.Source == "avmoo" || item.Source == "libredmm" {
			parsed, err = sources.ParseRemoteJSON(item.Source, item.SourceURL, []byte(item.RawJSON))
		} else if item.Source == "jable" {
			cfg, loadErr := settings.NewService(db).Sources(c.Request.Context())
			if loadErr != nil {
				err = loadErr
			} else {
				var body []byte
				var address string
				body, address, err = sources.FetchJablePage(c.Request.Context(), cfg, item.NormalizedID)
				if err == nil {
					parsed, err = sources.ParsePage("jable", address, body)
				}
			}
		} else if item.SourcePageID != nil {
			var entry adminPage
			if err = db.Table("source_pages").First(&entry, *item.SourcePageID).Error; err == nil {
				parsed, err = sources.ParsePage(item.Source, entry.URL, []byte(entry.RawHTML))
			}
		} else {
			err = errors.New("source page missing")
		}
		if err != nil {
			respondError(c, http.StatusUnprocessableEntity, "invalid_source_page")
			return
		}
		if err := ingest.NewService(db).Save(c.Request.Context(), parsed); err != nil {
			respondError(c, http.StatusInternalServerError, "rederivation_failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"normalized_id": parsed.Item.NormalizedID}})
	})
	api.GET("/admin/source-items/:id/raw", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		var item catalog.SourceItem
		if err := db.First(&item, c.Param("id")).Error; err != nil {
			respondCatalogError(c, err)
			return
		}
		var value map[string]any
		if item.Source != "fanza" || json.Unmarshal([]byte(item.RawJSON), &value) != nil {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		delete(value, "affiliateURL")
		delete(value, "affiliateURLsp")
		c.JSON(http.StatusOK, gin.H{"data": value})
	})
	api.PATCH("/admin/movies/:id", func(c *gin.Context) {
		db := databaseForAdmin(c)
		if db == nil {
			return
		}
		var input struct {
			CoverImageURL *string `json:"cover_image_url"`
			IsHidden      *bool   `json:"is_hidden"`
		}
		if c.ShouldBindJSON(&input) != nil || input.CoverImageURL == nil && input.IsHidden == nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		changes := map[string]any{}
		if input.CoverImageURL != nil {
			value := strings.TrimSpace(*input.CoverImageURL)
			if value != "" {
				parsed, err := url.Parse(value)
				if err != nil || len(value) > 2048 || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
					respondError(c, http.StatusBadRequest, "invalid_url")
					return
				}
			}
			changes["cover_image_url"] = value
		}
		if input.IsHidden != nil {
			changes["is_hidden"] = *input.IsHidden
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&catalog.Movie{}).Where("normalized_id = ?", strings.ToUpper(c.Param("id"))).Updates(changes)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			return cache.BumpVersion(tx)
		})
		if err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": true}})
	})
}

func crawlErrorCode(value string) string {
	switch {
	case strings.Contains(value, "source retired"):
		return "source_retired"
	case strings.Contains(value, "source returned HTTP 403"), strings.Contains(value, "source requires verification"):
		return "source_access_denied"
	case strings.Contains(value, "source request failed"):
		return "source_unavailable"
	case strings.Contains(value, "invalid source"):
		return "source_response_invalid"
	default:
		return "crawl_failed"
	}
}

func validPublicSourceURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil {
		return false
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "v", "id":
		default:
			return false
		}
	}
	return true
}
