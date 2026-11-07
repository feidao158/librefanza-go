package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

func TestSaveActressPreservesHiddenFlag(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	id := time.Now().UnixNano()
	defer db.Where("fanza_id = ?", id).Delete(&catalog.Actress{})
	service := NewService(db)
	if err := service.SaveActress(context.Background(), sources.ActressRecord{ID: id, Name: "演员甲", RawJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalog.Actress{}).Where("fanza_id = ?", id).Update("is_hidden", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.SaveActress(context.Background(), sources.ActressRecord{ID: id, Name: "演员乙", RawJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	var actress catalog.Actress
	if err := db.Where("fanza_id = ?", id).First(&actress).Error; err != nil || !actress.IsHidden || actress.Name != "演员乙" {
		t.Fatalf("actress=%+v err=%v", actress, err)
	}
}

func TestSaveUpdatesPreferredMovieAndIsIdempotent(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("INGEST-%d", time.Now().UnixNano())
	defer db.Where("normalized_id = ?", id).Delete(&catalog.Movie{})
	defer db.Where("normalized_id = ?", id).Delete(&catalog.SourceItem{})
	service := NewService(db)
	sod := sources.Parsed{Item: catalog.SourceItem{Source: "sod", ExternalID: id, NormalizedID: id, Title: "SOD", MetadataJSON: `{"cast":[{"name":"演员甲"}]}`}}
	fanza := sources.Parsed{Item: catalog.SourceItem{Source: "fanza", ExternalID: id, NormalizedID: id, Title: "Fanza", Description: "作品简介", CoverImageURL: "https://example.com/cover.jpg", MetadataJSON: `{"cast":[{"name":"演员乙"}],"sample_image_urls":["https://example.com/sample.jpg"]}`}}
	updated := fanza
	updated.Item.Description = ""
	updated.Item.CoverImageURL = ""
	updated.Item.MetadataJSON = `{"cast":[],"sample_image_urls":[]}`
	for _, item := range []sources.Parsed{sod, fanza, updated} {
		if err := service.Save(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	movie, err := catalog.NewRepository(db).Movie(context.Background(), id)
	if err != nil || movie.PreferredItem.Title != "Fanza" || movie.PreferredItem.Description != "作品简介" || len(movie.Versions) != 2 {
		t.Fatalf("movie=%+v err=%v", movie, err)
	}
	if movie.PreferredItem.CoverImageURL == "" || !strings.Contains(movie.PreferredItem.MetadataJSON, "sample.jpg") {
		t.Fatal("partial source response erased existing metadata")
	}
	var names []string
	if err := db.Table("movie_cast").Where("movie_id = ?", movie.ID).Pluck("name", &names).Error; err != nil || len(names) != 1 || names[0] != "演员乙" {
		t.Fatalf("cast=%v err=%v", names, err)
	}
}

func TestRebuildRecomputesMovieData(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	id := fmt.Sprintf("REBUILD-%d", time.Now().UnixNano())
	defer db.Where("normalized_id = ?", id).Delete(&catalog.Movie{})
	defer db.Where("normalized_id = ?", id).Delete(&catalog.SourceItem{})
	service := NewService(db)
	item := sources.Parsed{Item: catalog.SourceItem{Source: "fanza", ExternalID: id, NormalizedID: id, Title: "title", MetadataJSON: `{"cast":[{"name":"演员"}]}`}}
	if err := service.Save(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalog.Movie{}).Where("normalized_id = ?", id).Update("compressed_id", "broken").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RebuildMovies(context.Background()); err != nil {
		t.Fatal(err)
	}
	var result catalog.Movie
	if err := db.Where("normalized_id = ?", id).First(&result).Error; err != nil || result.CompressedID != catalog.CompressProductID(id) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestParsedSourceFixturesPersistAcrossDatabases(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	tests := []struct{ source, url, file string }{
		{"javlibrary", "https://www.javlibrary.com/ja/?v=javlio354y", "javlibrary.html"},
		{"mgstage", "https://www.mgstage.com/product/product_detail/261ARA-168/", "mgstage.html"},
		{"sod", "https://ec.sod.co.jp/prime/videos/?id=STARS-455", "sod.html"},
		{"fc2", "https://adult.contents.fc2.com/article/3069309/", "fc2.html"},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "sources", "testdata", test.file))
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := sources.ParsePage(test.source, test.url, body)
			if err != nil {
				t.Fatal(err)
			}
			id := fmt.Sprintf("FIXTURE-%d", time.Now().UnixNano())
			parsed.Item.ExternalID, parsed.Item.NormalizedID = id, id
			parsed.Item.SourceURL = "https://example.com/fixture/" + id
			defer db.Where("url = ?", parsed.Item.SourceURL).Delete(&page{})
			defer db.Where("normalized_id = ?", id).Delete(&catalog.Movie{})
			defer db.Where("normalized_id = ?", id).Delete(&catalog.SourceItem{})
			service := NewService(db)
			for i := 0; i < 2; i++ {
				if err := service.Save(context.Background(), parsed); err != nil {
					t.Fatal(err)
				}
			}
			movie, err := catalog.NewRepository(db).Movie(context.Background(), id)
			if err != nil || movie.PreferredItem == nil || movie.PreferredItem.Title != parsed.Item.Title || len(movie.Versions) != 1 {
				t.Fatalf("movie=%+v err=%v", movie, err)
			}
		})
	}
}
