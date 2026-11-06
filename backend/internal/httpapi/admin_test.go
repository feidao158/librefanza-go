package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/auth"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func TestAdminCanImportJavlibraryPage(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	_ = store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}})
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	user, err := auth.NewService(db).Register(context.Background(), fmt.Sprintf("admin-import-%d@example.com", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&user)
	if err := db.Model(&user).Update("is_admin", true).Error; err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewService(db).Login(context.Background(), user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("MANUAL-%d", time.Now().UnixNano())
	defer db.Where("normalized_id = ?", id).Delete(&catalog.Movie{})
	defer db.Where("normalized_id = ?", id).Delete(&catalog.SourceItem{})
	body, _ := json.Marshal(map[string]string{"source": "javlibrary", "url": "https://www.javlibrary.com/ja/?v=" + id, "raw_html": `<div id="video_id"><table><tr><td class="text">` + id + `</td></tr></table></div><div id="video_title"><h3>` + id + ` Test title</h3></div>`})
	router := NewRouter(setup.NewService(store, setupBackend{}), provider)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/source-pages", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", response.Code, response.Body.String())
	}
	public := httptest.NewRecorder()
	router.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/v1/movies/"+id, nil))
	if public.Code != http.StatusOK || !bytes.Contains(public.Body.Bytes(), []byte("Test title")) {
		t.Fatalf("movie: %d %s", public.Code, public.Body.String())
	}
}

func TestAdminJobsRequireAuthentication(t *testing.T) {
	store := config.NewStore(t.TempDir())
	router := NewRouter(setup.NewService(store, setupBackend{}), database.NewProvider(store))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/jobs", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("jobs: %d", response.Code)
	}
}

func TestImportedSourcePageURLDoesNotLeakCredentials(t *testing.T) {
	for _, value := range []string{"https://user:password@example.com/page", "https://example.com/page?api_key=secret", "http://127.0.0.1/private"} {
		if validPublicSourceURL(value) {
			t.Fatalf("accepted sensitive source URL %q", value)
		}
	}
	if !validPublicSourceURL("https://www.javlibrary.com/ja/?v=example") {
		t.Fatal("valid source URL rejected")
	}
}

func TestNonAdminCannotModifyManagementSettings(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	_ = store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}})
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.NewService(db).Register(context.Background(), fmt.Sprintf("regular-%d@example.com", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&user)
	tokens, err := auth.NewService(db).Login(context.Background(), user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	service := setup.NewService(store, setupBackend{})
	defer service.Close()
	router := NewRouter(service, provider)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/redis", bytes.NewBufferString(`{"enabled":false}`))
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin update: %d", response.Code)
	}
}
