package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func TestSPAFallbackDoesNotHandleUnknownAPI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("frontend"), 0600); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(setup.NewService(config.NewStore(t.TempDir()), setupBackend{}))
	RegisterSPA(router, dir)
	page := httptest.NewRecorder()
	router.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/movies/ABC-123", nil))
	if page.Code != http.StatusOK || page.Body.String() != "frontend" {
		t.Fatalf("SPA response: %d %s", page.Code, page.Body.String())
	}
	api := httptest.NewRecorder()
	router.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil))
	if api.Code != http.StatusNotFound {
		t.Fatalf("unknown API response: %d", api.Code)
	}
}
