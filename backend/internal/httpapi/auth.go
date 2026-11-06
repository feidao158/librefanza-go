package httpapi

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/auth"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/mailer"
	"github.com/libredmm/librefanza/backend/internal/settings"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Client   string `json:"client"`
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
		if c.ShouldBindJSON(&request) != nil || (request.Client != "" && request.Client != "web" && request.Client != "mobile") {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		if authService := service(c); authService != nil {
			tokens, err := authService.Login(c.Request.Context(), request.Email, request.Password)
			if err != nil {
				respondAuthError(c, err)
				return
			}
			respondTokens(c, http.StatusCreated, tokens, request.Client == "mobile")
		}
	})
	api.POST("/auth/refresh", func(c *gin.Context) {
		var request struct {
			RefreshToken string `json:"refresh_token"`
		}
		if c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&request); err != nil {
				respondError(c, http.StatusBadRequest, "invalid_request")
				return
			}
		}
		mobile := request.RefreshToken != ""
		if !mobile {
			if origin := c.GetHeader("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(parsed.Host, c.Request.Host) {
					respondError(c, http.StatusForbidden, "invalid_origin")
					return
				}
			}
			request.RefreshToken, _ = c.Cookie("lf_refresh")
		}
		if authService := service(c); authService != nil {
			tokens, err := authService.Refresh(c.Request.Context(), request.RefreshToken)
			if err != nil {
				respondAuthError(c, err)
				return
			}
			respondTokens(c, http.StatusOK, tokens, mobile)
		}
	})
	api.GET("/auth/me", func(c *gin.Context) {
		if authService := service(c); authService != nil {
			user, err := authService.Authenticate(c.Request.Context(), bearer(c))
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
			writeRefreshCookie(c, "", -1)
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

func respondTokens(c *gin.Context, status int, tokens auth.Tokens, mobile bool) {
	if !mobile {
		writeRefreshCookie(c, tokens.RefreshToken, int((30 * 24 * time.Hour).Seconds()))
		tokens.RefreshToken = ""
	}
	c.JSON(status, gin.H{"data": tokens})
}

func writeRefreshCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "lf_refresh", Value: value, Path: "/api/v1/auth", HttpOnly: true, Secure: os.Getenv("LIBREFANZA_INSECURE_COOKIES") != "true", SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}

func respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		respondError(c, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, auth.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, auth.ErrConflict):
		respondError(c, http.StatusConflict, "account_exists")
	default:
		respondError(c, http.StatusInternalServerError, "internal_error")
	}
}
