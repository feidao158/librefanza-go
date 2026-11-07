package tasks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/sourcehealth"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

func TestSuspiciousCoverCandidates(t *testing.T) {
	for _, test := range []struct {
		item catalog.SourceItem
		want bool
	}{
		{catalog.SourceItem{}, true},
		{catalog.SourceItem{CoverImageURL: "https://example.com/small.jpg", ThumbnailImageURL: "https://example.com/small.jpg"}, true},
		{catalog.SourceItem{CoverImageURL: "https://example.com/w240/cover.jpg", ThumbnailImageURL: "https://example.com/small.jpg"}, true},
		{catalog.SourceItem{CoverImageURL: "https://example.com/large.jpg", ThumbnailImageURL: "https://example.com/small.jpg"}, false},
	} {
		if got := suspiciousCover(test.item); got != test.want {
			t.Fatalf("封面=%s 缩略图=%s 结果=%v", test.item.CoverImageURL, test.item.ThumbnailImageURL, got)
		}
	}
}

func TestSourceHistoryPersistsDeferredAndResumes(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要专用测试数据库")
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
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	movie := catalog.Movie{NormalizedID: fmt.Sprintf("HISTORY-%d", time.Now().UnixNano()), CompressedID: fmt.Sprintf("HISTORY-%d", time.Now().UnixNano())}
	if err := tx.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("source = ?", "javbooks").Delete(&sourceHistoryProgress{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("source = ?", "javbooks").Delete(&sourcehealth.State{}).Error; err != nil {
		t.Fatal(err)
	}
	progress := sourceHistoryProgress{Source: "javbooks", CursorID: movie.ID - 1, LimitID: movie.ID, UpdatedAt: time.Now().UTC()}
	if err := tx.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{db: tx, logs: logbook.New(tx, io.Discard)}
	calls := 0
	collect := func(context.Context, string, string) (bool, error) {
		calls++
		if calls == 1 {
			return false, errors.New("source unavailable")
		}
		return false, nil
	}
	if err := manager.runSourceHistoryBackfill(context.Background(), "javbooks", collect); err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&progress, "source = ?", "javbooks").Error; err != nil || progress.CursorID != movie.ID || progress.Failed != 1 {
		t.Fatalf("补采游标未提交: %+v, %v", progress, err)
	}
	jobID := fmt.Sprintf("history:javbooks:%d", movie.ID)
	var deferred MetadataSourceProgress
	if err := tx.Where("job_id = ? AND source = ?", jobID, "javbooks").First(&deferred).Error; err != nil || deferred.Status != "deferred" {
		t.Fatalf("失败记录未延期: %+v, %v", deferred, err)
	}
	if err := manager.runEnrichment(context.Background(), jobID, movie.NormalizedID, []string{"javbooks"}, collect); err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("job_id = ? AND source = ?", jobID, "javbooks").First(&deferred).Error; err != nil || deferred.Status != "completed" || calls != 2 {
		t.Fatalf("延期重试未完成: %+v, %v", deferred, err)
	}
	if err := manager.runSourceHistoryBackfill(context.Background(), "javbooks", collect); err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&progress, "source = ?", "javbooks").Error; err != nil || !progress.Completed || calls != 2 {
		t.Fatalf("补采未从断点完成: %+v, %v", progress, err)
	}
}

func TestCoverRepairFailureIsDurableAndCanRecover(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要专用测试数据库")
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
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	id := fmt.Sprintf("COVERRETRY-%d", time.Now().UnixNano())
	item := catalog.SourceItem{Source: "libredmm", ExternalID: id, NormalizedID: id, Title: "标题"}
	if err := tx.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{db: tx}
	if err := manager.repairCover(context.Background(), item.ID, func(context.Context, catalog.SourceItem) error { return errors.New("network error") }); err != nil {
		t.Fatal(err)
	}
	var deferred coverRepairDeferred
	if err := tx.First(&deferred, "source_item_id = ?", item.ID).Error; err != nil || deferred.Attempts != 1 || !deferred.NextRetry.After(time.Now()) {
		t.Fatalf("封面延期状态错误: %+v %v", deferred, err)
	}
	if err := manager.repairCover(context.Background(), item.ID, func(context.Context, catalog.SourceItem) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := tx.Model(&coverRepairDeferred{}).Where("source_item_id = ?", item.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("恢复后延期记录未移除: %d %v", count, err)
	}
}

func TestCooledHistorySourceRetainsCursor(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要专用测试数据库")
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
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	id := fmt.Sprintf("COOLDOWN-%d", time.Now().UnixNano())
	movie := catalog.Movie{NormalizedID: id, CompressedID: id}
	if err := tx.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("source = ?", "javquery").Delete(&sourceHistoryProgress{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("source = ?", "javquery").Delete(&sourcehealth.State{}).Error; err != nil {
		t.Fatal(err)
	}
	next := time.Now().UTC().Add(time.Hour)
	if err := tx.Create(&sourcehealth.State{Source: "javquery", Status: "cooldown", NextRetry: &next, UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	progress := sourceHistoryProgress{Source: "javquery", CursorID: movie.ID - 1, LimitID: movie.ID, UpdatedAt: time.Now().UTC()}
	if err := tx.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{db: tx, logs: logbook.New(tx, io.Discard)}
	calls := 0
	if err := manager.runSourceHistoryBackfill(context.Background(), "javquery", func(context.Context, string, string) (bool, error) {
		calls++
		return false, &sources.DeferredError{RetryAt: next}
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&progress, "source = ?", "javquery").Error; err != nil || progress.CursorID != movie.ID-1 || calls != 0 {
		t.Fatalf("冷却期间不应推进游标: %+v 请求次数=%d %v", progress, calls, err)
	}
}
