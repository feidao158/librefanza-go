package tasks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
)

func TestMetadataEnrichmentContinuesAndResumesFailedSources(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	jobID := fmt.Sprintf("resume-%d", time.Now().UnixNano())
	movieID := fmt.Sprintf("RETRY-%d", time.Now().UnixNano())
	defer db.Where("job_id = ?", jobID).Delete(&MetadataSourceProgress{})
	calls := map[string]int{}
	run := func(_ context.Context, _, source string) (bool, error) {
		calls[source]++
		if source == "fanza" && calls[source] == 1 {
			return false, errors.New("source unavailable")
		}
		return true, nil
	}
	firstErr := manager.runEnrichment(context.Background(), jobID, movieID, []string{"fanza", "sod"}, run)
	if firstErr == nil {
		t.Fatal("source failure was not reported")
	}
	if calls["sod"] != 1 {
		t.Fatalf("another source was not collected: %v", firstErr)
	}
	if err := manager.runEnrichment(context.Background(), jobID, movieID, []string{"fanza", "sod"}, run); err != nil {
		t.Fatal(err)
	}
	if calls["fanza"] != 2 || calls["sod"] != 1 {
		t.Fatalf("unexpected retry calls: %v", calls)
	}
}

func TestMetadataJobRunsAndDeduplicates(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	if err := manager.ConfigureMetadata(func(context.Context, string, string) (bool, error) { calls.Add(1); return true, nil }, func(context.Context, catalog.SourceItem) error { return nil }); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("META-%d", time.Now().UnixNano())
	movie := catalog.Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	job, err := manager.EnqueueEnrichment(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := manager.EnqueueEnrichment(context.Background(), id)
	if err != nil || job != duplicate {
		t.Fatalf("deduplication failed: %v", err)
	}
	defer db.Where("job_id = ?", job).Delete(&MetadataSourceProgress{})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		status, err := manager.EnrichmentStatus(context.Background(), id)
		if err == nil && status.Status == "completed" {
			if status.CompletedSources != int64(len(metadataSources)) || calls.Load() != int32(len(metadataSources)) {
				t.Fatalf("status=%+v calls=%d", status, calls.Load())
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("metadata job did not complete")
}

func TestMetadataRecentSourceIsSkippedAcrossJobs(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("RECENT-%d", time.Now().UnixNano())
	defer db.Where("movie_id = ?", id).Delete(&MetadataSourceProgress{})
	item := catalog.SourceItem{Source: "sod", ExternalID: id, NormalizedID: id, Title: "title"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&item)
	calls := 0
	collect := func(context.Context, string, string) (bool, error) { calls++; return true, nil }
	for _, job := range []string{"first", "second"} {
		if err := manager.runEnrichment(context.Background(), job+id, id, []string{"sod"}, collect); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("recent source was requested %d times", calls)
	}
}

func TestEnrichmentStatusIncludesHistoricalSourceBackfill(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ConfigureMetadata(func(context.Context, string, string) (bool, error) { return false, nil }, func(context.Context, catalog.SourceItem) error { return nil }); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("HISTORYSTATUS-%d", time.Now().UnixNano())
	movie := catalog.Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	jobID, err := manager.EnqueueEnrichment(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Where("unique_key = ?", "metadata_enrich:"+id).Delete(&jobs.Job{})
	defer db.Where("movie_id = ?", id).Delete(&MetadataSourceProgress{})
	for _, source := range metadataSources {
		progressID := jobID
		for _, name := range newHistorySources {
			if source == name {
				progressID = fmt.Sprintf("history:%s:%d", source, movie.ID)
			}
		}
		if err := db.Create(&MetadataSourceProgress{JobID: progressID, MovieID: id, Source: source, Status: "completed", UpdatedAt: time.Now().UTC()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	status, err := manager.EnrichmentStatus(context.Background(), id)
	if err != nil || status.CollectionStatus != "complete" || status.CompletedSources != int64(len(metadataSources)) {
		t.Fatalf("历史补采未计入影片状态: %+v %v", status, err)
	}
}

func TestMetadataBackfillResumesAfterCancellation(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ConfigureMetadata(func(context.Context, string, string) (bool, error) { return true, nil }, func(context.Context, catalog.SourceItem) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var original MetadataBackfillProgress
	if err := db.First(&original, 1).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Model(&MetadataBackfillProgress{}).Where("id = ?", 1).Select("phase", "cursor_id", "movie_cursor_id", "raw_limit_id", "movie_limit_id", "processed", "failed", "completed", "updated_at").Updates(&original)
	var maxItemID, maxMovieID uint64
	if err := db.Model(&catalog.SourceItem{}).Select("COALESCE(MAX(id), 0)").Scan(&maxItemID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalog.Movie{}).Select("COALESCE(MAX(id), 0)").Scan(&maxMovieID).Error; err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("BACKFILL-%d", time.Now().UnixNano())
	movie := catalog.Movie{NormalizedID: id, CompressedID: id}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	item := catalog.SourceItem{Source: "fanza", ExternalID: id, NormalizedID: id, Title: "title"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&item)
	if err := db.Model(&MetadataBackfillProgress{}).Where("id = ?", 1).Updates(map[string]any{"phase": "raw", "cursor_id": maxItemID, "movie_cursor_id": maxMovieID, "raw_limit_id": item.ID, "movie_limit_id": maxMovieID, "processed": 0, "failed": 0, "completed": false}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	var started atomic.Bool
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if ctx.Err() != nil {
				return
			}
			var progress MetadataBackfillProgress
			if db.First(&progress, 1).Error == nil && progress.CursorID == item.ID {
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	if err := manager.runBackfill(ctx, func(context.Context, catalog.SourceItem) error { calls++; started.Store(true); return nil }); err != nil && !errors.Is(err, context.Canceled) {
		if ctx.Err() == nil {
			t.Fatal(err)
		}
	}
	cancel()
	if err := db.Model(&MetadataBackfillProgress{}).Where("id = ?", 1).Update("movie_limit_id", movie.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.runBackfill(context.Background(), func(context.Context, catalog.SourceItem) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	var progress MetadataBackfillProgress
	if err := db.First(&progress, 1).Error; err != nil || !progress.Completed || !started.Load() || calls != 1 {
		t.Fatalf("resume failed: %+v %v", progress, err)
	}
	var job jobs.Job
	if err := db.Where("type = ? AND unique_key = ?", "metadata_enrich", "metadata_enrich:"+id).First(&job).Error; err != nil {
		t.Fatal(err)
	}
	defer manager.storage.DeleteJob(context.Background(), job.ID)
}
