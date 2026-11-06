package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func TestMovieAPIHidesHiddenRecords(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	cfg := config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}
	if err := store.Save(config.Config{Installed: true, Database: cfg}); err != nil {
		t.Fatal(err)
	}
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("HTTP-%d", time.Now().UnixNano())
	movie := catalog.Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("normalized_id = ?", id).Delete(&catalog.Movie{})
	item := catalog.SourceItem{Source: "fanza", ExternalID: id, NormalizedID: id, Title: "测试影片", MetadataJSON: `{"genres":["剧情"]}`}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("normalized_id = ?", id).Delete(&catalog.SourceItem{})
	sod := catalog.SourceItem{Source: "sod", ExternalID: id, NormalizedID: id, Title: "SOD", SourceURL: "https://ec.sod.co.jp/prime/videos/?id=" + id, CoverImageURL: "https://dy43ylo5q3vt8.cloudfront.net/cover.jpg"}
	if err := db.Create(&sod).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouter(setup.NewService(store, setupBackend{}), provider)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/movies/"+id, nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("测试影片")) {
		t.Fatalf("movie: %d %s", response.Code, response.Body.String())
	}
	version := httptest.NewRecorder()
	router.ServeHTTP(version, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/source-items/%d", item.ID), nil))
	if version.Code != http.StatusOK || !bytes.Contains(version.Body.Bytes(), []byte("测试影片")) || !bytes.Contains(version.Body.Bytes(), []byte(`"metadata":{"genres":["剧情"]}`)) {
		t.Fatalf("source version: %d %s", version.Code, version.Body.String())
	}
	sodVersion := httptest.NewRecorder()
	router.ServeHTTP(sodVersion, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/source-items/%d", sod.ID), nil))
	if sodVersion.Code != http.StatusOK || !bytes.Contains(sodVersion.Body.Bytes(), []byte(fmt.Sprintf("/api/v1/source-items/%d/images/cover", sod.ID))) {
		t.Fatalf("SOD image proxy: %d %s", sodVersion.Code, sodVersion.Body.String())
	}
	if err := db.Model(&movie).Update("is_hidden", true).Error; err != nil {
		t.Fatal(err)
	}
	hidden := httptest.NewRecorder()
	router.ServeHTTP(hidden, httptest.NewRequest(http.MethodGet, "/api/v1/movies/"+id, nil))
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("hidden movie: %d %s", hidden.Code, hidden.Body.String())
	}
	hiddenVersion := httptest.NewRecorder()
	router.ServeHTTP(hiddenVersion, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/source-items/%d", item.ID), nil))
	if hiddenVersion.Code != http.StatusNotFound {
		t.Fatalf("hidden source version: %d", hiddenVersion.Code)
	}
	hiddenImage := httptest.NewRecorder()
	router.ServeHTTP(hiddenImage, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/source-items/%d/images/cover", sod.ID), nil))
	if hiddenImage.Code != http.StatusNotFound {
		t.Fatalf("hidden image: %d", hiddenImage.Code)
	}
}

func TestProductIDNormalizationAPI(t *testing.T) {
	store := config.NewStore(t.TempDir())
	service := setup.NewService(store, setupBackend{})
	defer service.Close()
	router := NewRouter(service, database.NewProvider(store))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/product-ids/normalize?q=abp00123", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"normalized":"ABP-123"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"valid":true`)) {
		t.Fatalf("normalization: %d %s", response.Code, response.Body.String())
	}
}
