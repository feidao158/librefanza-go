package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/auth"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/recommendation"
	"github.com/libredmm/librefanza/backend/internal/settings"
)

func registerRecommendationRoutes(api *gin.RouterGroup, provider *database.Provider) {
	service := func(c *gin.Context) *recommendation.Service {
		db, err := provider.Database(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return nil
		}
		return recommendation.NewService(db, func(ctx context.Context) (settings.GorseSettings, error) {
			return settings.NewService(db).Gorse(ctx)
		})
	}

	resolveID := func(c *gin.Context) string {
		return resolveUserID(c, provider)
	}

	// GET /api/v1/annotations/next 随机获取下一部待人工标注的番号
	api.GET("/annotations/next", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		userID := resolveID(c)
		exclude := c.Query("exclude")
		movie, stats, err := svc.GetNextAnnotation(c.Request.Context(), userID, exclude)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"movie": movie,
				"stats": stats,
			},
		})
	})

	// POST /api/v1/annotations/feedback 提交人工标注选项（喜欢/不喜欢/不确定）
	api.POST("/annotations/feedback", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var input recommendation.FeedbackInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if input.NormalizedID == "" || input.Action == "" {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		userID := resolveID(c)
		stats, nextMovie, err := svc.SaveFeedback(c.Request.Context(), userID, input)
		if err != nil {
			if errors.Is(err, recommendation.ErrInvalidAction) {
				respondError(c, http.StatusBadRequest, "invalid_action")
				return
			}
			if errors.Is(err, recommendation.ErrMovieNotFound) {
				respondError(c, http.StatusNotFound, "movie_not_found")
				return
			}
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		svc.RecordFeedback(c.Request.Context(), "annotation", input.Action, stats)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"success":    true,
				"stats":      stats,
				"next_movie": nextMovie,
			},
		})
	})

	// GET /api/v1/annotations/stats 获取用户标注统计数据
	api.GET("/annotations/stats", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		userID := resolveID(c)
		stats, err := svc.GetStats(c.Request.Context(), userID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data": stats,
		})
	})

	// 读取当前用户喜欢的作品。
	api.GET("/annotations/likes", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		page, size := pageParams(c)
		movies, total, err := svc.GetLikedMovies(c.Request.Context(), resolveID(c), page, size)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": movies, "meta": gin.H{"page": page, "page_size": size, "total": total}})
	})

	// GET /api/v1/recommendations 猜你喜欢：获取个性化推荐结果
	api.GET("/recommendations", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		page, size := pageParams(c)
		userID := resolveID(c)
		movies, stats, total, err := svc.GetRecommendations(c.Request.Context(), userID, page, size)
		if err != nil {
			if errors.Is(err, recommendation.ErrGorseUnavailable) {
				respondError(c, http.StatusServiceUnavailable, "recommendation_unavailable")
				return
			}
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"ready":  stats.Ready,
				"stats":  stats,
				"movies": movies,
			},
			"meta": gin.H{
				"page":      page,
				"page_size": size,
				"total":     total,
			},
		})
	})

	// POST /api/v1/recommendations/feedback 猜你喜欢页面交互反馈
	api.POST("/recommendations/feedback", func(c *gin.Context) {
		svc := service(c)
		if svc == nil {
			return
		}
		var input recommendation.FeedbackInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if input.NormalizedID == "" || input.Action == "" {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		userID := resolveID(c)
		stats, nextMovie, err := svc.SaveFeedback(c.Request.Context(), userID, input)
		if err != nil {
			if errors.Is(err, recommendation.ErrInvalidAction) {
				respondError(c, http.StatusBadRequest, "invalid_action")
				return
			}
			if errors.Is(err, recommendation.ErrMovieNotFound) {
				respondError(c, http.StatusNotFound, "movie_not_found")
				return
			}
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		svc.RecordFeedback(c.Request.Context(), "recommendation", input.Action, stats)
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"success":    true,
				"stats":      stats,
				"next_movie": nextMovie,
			},
		})
	})
}

// resolveUserID 解析请求对应的用户标识（优先已登录账号，未登录使用客户端标识）
func resolveUserID(c *gin.Context, provider *database.Provider) string {
	if b := bearer(c); b != "" && provider != nil {
		if db, err := provider.Database(c.Request.Context()); err == nil {
			if user, err := auth.NewService(db).Authenticate(c.Request.Context(), b); err == nil {
				return fmt.Sprintf("user_%d", user.ID)
			}
		}
	}
	clientID := strings.TrimSpace(c.GetHeader("X-Client-ID"))
	if clientID == "" {
		clientID = strings.TrimSpace(c.Query("client_id"))
	}
	if clientID != "" && len(clientID) <= 64 {
		return "anon_" + clientID
	}
	return "anon_default"
}
