package httpapi

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterSPA(router *gin.Engine, dir string) {
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			respondError(c, http.StatusNotFound, "not_found")
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}
		relative := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		file := filepath.Join(dir, filepath.FromSlash(relative))
		info, err := os.Stat(file)
		if err == nil && !info.IsDir() {
			c.File(file)
			return
		}
		if path.Ext(relative) != "" {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(filepath.Join(dir, "index.html"))
	})
}
