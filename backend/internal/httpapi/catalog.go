package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/cache"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/images"
	"gorm.io/gorm"
)

func registerCatalogRoutes(api *gin.RouterGroup, provider *database.Provider, cacheManager *cache.Manager) {
	api.GET("/product-ids/normalize", func(c *gin.Context) {
		value := strings.TrimSpace(c.Query("q"))
		valid := len(value) <= 128 && catalog.IsProductID(value)
		if !valid {
			c.JSON(http.StatusOK, gin.H{"data": gin.H{"valid": false, "normalized": value}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"valid": true, "normalized": catalog.NormalizeProductID(value)}})
	})
	repository := func(c *gin.Context) *catalog.Repository {
		db, err := provider.Database(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return nil
		}
		return catalog.NewRepository(db)
	}
	api.GET("/movies", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		page, size := pageParams(c)
		movies, total, err := repo.Movies(c.Request.Context(), catalog.MovieQuery{Query: c.Query("q"), Match: c.Query("match"), Order: c.Query("order"), Page: page, PageSize: size})
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": movies, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
	api.GET("/movies/:id", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		key := "movie:" + strings.ToUpper(c.Param("id"))
		cached, handle := cacheManager.Get(c.Request.Context(), key)
		if cached != nil {
			c.Data(http.StatusOK, "application/json; charset=utf-8", cached)
			return
		}
		movie, err := repo.Movie(c.Request.Context(), c.Param("id"))
		if err != nil {
			respondCatalogError(c, err)
			return
		}
		if handle != nil {
			if body, err := json.Marshal(gin.H{"data": movie}); err == nil {
				handle.Set(c.Request.Context(), body)
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": movie})
	})
	api.GET("/source-items/:id", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		item, err := repo.SourceItem(c.Request.Context(), id)
		if err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": item})
	})
	api.GET("/source-items/:id/images/:variant", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		item, err := repo.SourceItem(c.Request.Context(), id)
		if err != nil || item.Source != "sod" {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		imageURL := item.CoverImageURL
		if c.Param("variant") == "thumbnail" {
			imageURL = item.ThumbnailImageURL
		} else if c.Param("variant") != "cover" {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		body, contentType, err := images.FetchSOD(c.Request.Context(), nil, imageURL, item.SourceURL)
		if err != nil {
			respondError(c, http.StatusBadGateway, "image_unavailable")
			return
		}
		c.Header("Cache-Control", "private, max-age=60")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, contentType, body)
	})
	api.GET("/actresses", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		page, size := pageParams(c)
		actresses, total, err := repo.Actresses(c.Request.Context(), c.Query("q"), c.Query("order"), page, size)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": actresses, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
	api.GET("/actress-filmographies", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		page, size := pageParams(c)
		actress, movies, total, err := repo.FilmographyForName(c.Request.Context(), c.Query("name"), c.Query("solo") == "true", page, size)
		if err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"actress": actress, "movies": movies}, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
	api.GET("/actresses/:id", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		actress, err := repo.Actress(c.Request.Context(), id)
		if err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": actress})
	})
	api.GET("/actresses/:id/movies", func(c *gin.Context) {
		repo := repository(c)
		if repo == nil {
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		page, size := pageParams(c)
		movies, total, err := repo.Filmography(c.Request.Context(), id, c.Query("solo") == "true", page, size)
		if err != nil {
			respondCatalogError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": movies, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})
}

func pageParams(c *gin.Context) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1000000 {
		page = 1
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	if err != nil || size < 1 {
		size = 30
	}
	return page, min(size, 100)
}

func respondCatalogError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		respondError(c, http.StatusNotFound, "not_found")
		return
	}
	respondError(c, http.StatusInternalServerError, "internal_error")
}
