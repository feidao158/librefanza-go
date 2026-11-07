package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"gorm.io/gorm"
)

func TestSampleImagesUseRestrictedProxy(t *testing.T) {
	item := SourceItem{ID: 42, Source: "sod", MetadataJSON: `{"sample_image_urls":["https://dy43ylo5q3vt8.cloudfront.net/image.jpg","https://evil.example/image.jpg","javascript:alert(1)"]}`}
	body, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "/api/v1/source-items/42/images/samples/0") || strings.Contains(string(body), "/images/samples/1") || strings.Contains(string(body), "javascript:") {
		t.Fatalf("unexpected sample addresses: %s", body)
	}
}

func TestManualCoverPreservesOriginalSourceVersion(t *testing.T) {
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
	id := fmt.Sprintf("COVER-%d", time.Now().UnixNano())
	movie := Movie{NormalizedID: id, CompressedID: id, CoverImageURL: "https://example.com/manual.jpg"}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	item := SourceItem{Source: "sod", ExternalID: id, NormalizedID: id, Title: "title", CoverImageURL: "https://dy43ylo5q3vt8.cloudfront.net/original.jpg"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&item)
	repo := NewRepository(db)
	result, err := repo.Movie(context.Background(), id)
	if err != nil || result.PreferredItem.CoverImageURL != movie.CoverImageURL || result.Versions[0].CoverImageURL != item.CoverImageURL {
		t.Fatal("manual cover replaced source version")
	}
	original, err := repo.SourceItem(context.Background(), item.ID)
	if err != nil || original.CoverImageURL != item.CoverImageURL {
		t.Fatal("source image endpoint lost original cover")
	}
}

func TestMovieRepositorySearchAndPreferredSource(t *testing.T) {
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
	id := fmt.Sprintf("TEST-%d", time.Now().UnixNano())
	movie := Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("normalized_id = ?", id).Delete(&Movie{})
	defer db.Where("normalized_id = ?", id).Delete(&SourceItem{})
	for _, item := range []SourceItem{
		{Source: "mgstage", ExternalID: id, NormalizedID: id, Title: "MGStage"},
		{Source: "sod", ExternalID: id, NormalizedID: id, Title: "SOD"},
		{Source: "fanza", ExternalID: id + "a", NormalizedID: id, Title: "Fanza low", Priority: 0},
		{Source: "fanza", ExternalID: id + "b", NormalizedID: id, Title: "Fanza preferred", Priority: 2},
	} {
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(db)
	items, total, err := repo.Movies(context.Background(), MovieQuery{Query: id, Match: "prefix", Page: 1, PageSize: 30})
	if err != nil || total != 1 || len(items) != 1 || items[0].PreferredItem == nil || items[0].PreferredItem.Title != "Fanza preferred" {
		t.Fatalf("search: items=%+v total=%d err=%v", items, total, err)
	}
	if err := db.Model(&movie).Update("is_hidden", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Movie(context.Background(), id); err == nil {
		t.Fatal("hidden movie was returned")
	}
}

func TestActressFilmographyUsesSingleActorFilter(t *testing.T) {
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
	name := fmt.Sprintf("测试演员%d", time.Now().UnixNano())
	actress := Actress{Name: name, SearchName: name}
	if err := db.Create(&actress).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&actress)
	id := fmt.Sprintf("FILM-%d", time.Now().UnixNano())
	movie := Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	if err := db.Table("movie_cast").Create(map[string]any{"movie_id": movie.ID, "name": name}).Error; err != nil {
		t.Fatal(err)
	}
	items, total, err := NewRepository(db).Filmography(context.Background(), actress.ID, true, 1, 30)
	if err != nil || total != 1 || len(items) != 1 || items[0].NormalizedID != id {
		t.Fatalf("filmography=%+v total=%d err=%v", items, total, err)
	}
}

func TestSortSourceItemsPutsMissingFanzaDateLast(t *testing.T) {
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []SourceItem{{ID: 1, Source: "fanza", Priority: 1}, {ID: 2, Source: "fanza", Priority: 1, ReleaseDate: &date}}
	SortSourceItems(items)
	if items[0].ID != 2 {
		t.Fatalf("unexpected priority order: %+v", items)
	}
}

func TestMovieLookupNormalizesInput(t *testing.T) {
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
	id := fmt.Sprintf("NORMAL-%d", time.Now().UnixNano())
	input := "normal000" + strings.TrimPrefix(id, "NORMAL-")
	movie := Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	item := SourceItem{Source: "fanza", ExternalID: id, NormalizedID: id, Title: "Example"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&item)
	got, err := NewRepository(db).Movie(context.Background(), input)
	if err != nil || got.NormalizedID != id {
		t.Fatalf("movie=%+v err=%v", got, err)
	}
}

func TestMovieMetadataExcludesHiddenActresses(t *testing.T) {
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
	id := fmt.Sprintf("PRIVATE-%d", time.Now().UnixNano())
	hiddenName := "隐藏演员" + id
	actress := Actress{Name: hiddenName, SearchName: hiddenName, IsHidden: true}
	if err := db.Create(&actress).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&actress)
	movie := Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	item := SourceItem{Source: "fanza", ExternalID: id, NormalizedID: id, Title: "Example", MetadataJSON: fmt.Sprintf(`{"cast":[{"name":%q},{"name":"公开演员"}]}`, hiddenName)}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&item)
	repo := NewRepository(db)
	result, err := repo.Movie(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.PreferredItem.MetadataJSON, hiddenName) || !strings.Contains(result.PreferredItem.MetadataJSON, "公开演员") {
		t.Fatalf("filtered metadata: %s", result.PreferredItem.MetadataJSON)
	}
	version, err := repo.SourceItem(context.Background(), item.ID)
	if err != nil || strings.Contains(version.MetadataJSON, hiddenName) {
		t.Fatalf("source metadata: %s err=%v", version.MetadataJSON, err)
	}
}

func TestUnknownActressNameStillHasFilmography(t *testing.T) {
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
	name := fmt.Sprintf("未收录演员%d", time.Now().UnixNano())
	id := fmt.Sprintf("CAST-%d", time.Now().UnixNano())
	movie := Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	if err := db.Table("movie_cast").Create(map[string]any{"movie_id": movie.ID, "name": name}).Error; err != nil {
		t.Fatal(err)
	}
	actress, movies, total, err := NewRepository(db).FilmographyForName(context.Background(), name, false, 1, 30)
	if err != nil || actress.Name != name || total != 1 || len(movies) != 1 {
		t.Fatalf("actress=%+v movies=%+v total=%d err=%v", actress, movies, total, err)
	}
}

func TestHiddenActressNameDoesNotLeakThroughVisibleNamesake(t *testing.T) {
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
	name := fmt.Sprintf("同名演员%d", time.Now().UnixNano())
	visibleID, hiddenID := time.Now().UnixNano()/1000, time.Now().UnixNano()/1000+1
	visible := Actress{FanzaID: &visibleID, Name: name, SearchName: name}
	hidden := Actress{FanzaID: &hiddenID, Name: name, SearchName: name, IsHidden: true}
	if err := db.Create(&visible).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&visible)
	if err := db.Create(&hidden).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&hidden)
	repo := NewRepository(db)
	if _, _, _, err := repo.FilmographyForName(context.Background(), name, false, 1, 30); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("hidden namesake was exposed by name: %v", err)
	}
	if _, _, err := repo.Filmography(context.Background(), visible.ID, false, 1, 30); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("hidden namesake was exposed by id: %v", err)
	}
}

func TestMultilingualActressPagination(t *testing.T) {
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
	prefix := fmt.Sprintf("测试日本語%d", time.Now().UnixNano())
	defer db.Where("search_name LIKE ?", prefix+"%").Delete(&Actress{})
	for _, suffix := range []string{" 中文", " 日本語", " かな"} {
		name := prefix + suffix
		if err := db.Create(&Actress{Name: name, SearchName: name}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(db)
	first, total, err := repo.Actresses(context.Background(), prefix, "name", 1, 2)
	if err != nil || total != 3 || len(first) != 2 {
		t.Fatalf("first=%+v total=%d err=%v", first, total, err)
	}
	second, total, err := repo.Actresses(context.Background(), prefix, "name", 2, 2)
	if err != nil || total != 3 || len(second) != 1 || second[0].ID == first[0].ID || second[0].ID == first[1].ID {
		t.Fatalf("second=%+v total=%d err=%v", second, total, err)
	}
}
