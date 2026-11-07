package httpapi

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/auth"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/mailer"
	"github.com/libredmm/librefanza/backend/internal/settings"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func registerAuthRoutes(api *gin.RouterGroup, provider *database.Provider) {
	service := func(c *gin.Context) *auth.Service {
		db, err := provider.Database(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return nil
		}
		return auth.NewService(db)
	}
	api.POST("/users", func(c *gin.Context) {
		var request credentials
		if c.ShouldBindJSON(&request) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if authService := service(c); authService != nil {
			user, err := authService.Register(c.Request.Context(), request.Email, request.Password)
			if err != nil {
				respondAuthError(c, err)
				return
			}
			c.JSON(http.StatusCreated, gin.H{"data": user})
		}
	})
	api.POST("/auth/sessions", func(c *gin.Context) {
		var request credentials
		if c.ShouldBindJSON(&request) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if authService := service(c); authService != nil {
			tokens, err := authService.Login(c.Request.Context(), request.Email, request.Password)
			if err != nil {
				respondAuthError(c, err)
				return
			}
			c.JSON(http.StatusCreated, gin.H{"data": tokens})
		}
	})
	api.GET("/auth/me", func(c *gin.Context) {
		if authService := service(c); authService != nil {
			user, err := authService.AuthenticateWithActivity(c.Request.Context(), bearer(c), c.GetHeader("X-Session-Activity") != "passive")
			if err != nil {
				respondAuthError(c, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": user})
		}
	})
	api.DELETE("/auth/sessions/current", func(c *gin.Context) {
		if authService := service(c); authService != nil {
			if _, err := authService.Authenticate(c.Request.Context(), bearer(c)); err != nil {
				respondAuthError(c, err)
				return
			}
			if err := authService.Logout(c.Request.Context(), bearer(c)); err != nil {
				respondAuthError(c, err)
				return
			}
			c.Status(http.StatusNoContent)
		}
	})
	api.POST("/auth/password-reset-requests", func(c *gin.Context) {
		var input struct {
			Email string `json:"email"`
		}
		if c.ShouldBindJSON(&input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		db, err := provider.Database(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		cfg, err := settings.NewService(db).SMTP(c.Request.Context())
		if err != nil || !cfg.Enabled {
			respondError(c, http.StatusServiceUnavailable, "password_reset_unavailable")
			return
		}
		token, err := auth.NewService(db).CreatePasswordReset(c.Request.Context(), input.Email)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal_error")
			return
		}
		if token != "" {
			if err := mailer.SendReset(c.Request.Context(), cfg, input.Email, token); err != nil {
				log.Print("密码重置邮件发送失败")
			}
		}
		c.Status(http.StatusAccepted)
	})
	api.POST("/auth/password-resets", func(c *gin.Context) {
		var input struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if c.ShouldBindJSON(&input) != nil {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if authService := service(c); authService != nil {
			if err := authService.ResetPassword(c.Request.Context(), input.Token, input.Password); err != nil {
				respondAuthError(c, err)
				return
			}
			c.Status(http.StatusNoContent)
		}
	})
}

func bearer(c *gin.Context) string {
	value := c.GetHeader("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(value, "Bearer ")
}

func respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		respondError(c, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, auth.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, auth.ErrConflict):
		respondError(c, http.StatusConflict, "account_exists")
	case errors.Is(err, auth.ErrDatabase):
		respondError(c, http.StatusServiceUnavailable, "database_unavailable")
	default:
		respondError(c, http.StatusInternalServerError, "internal_error")
	}
}
