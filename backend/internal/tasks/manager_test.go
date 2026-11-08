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

func TestSourceBackfillQueueRunsWhileMetadataBackfillIsBusy(t *testing.T) {
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
	search := func(context.Context, string) (bool, error) { return false, nil }
	oldManager, err := NewManager(db, search)
	if err != nil {
		t.Fatal(err)
	}
	oldManager.queue.Register("source_history_backfill", func(context.Context, sourceHistoryArgs) error { return nil })
	oldJob, err := oldManager.queue.Enqueue(context.Background(), "source_history_backfill", sourceHistoryArgs{Source: "javdb"}, jobs.QueueOpt("backfill"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", oldJob).Delete(&jobs.Job{})
	manager, err := NewManager(db, search)
	if err != nil {
		t.Fatal(err)
	}
	var migrated jobs.Job
	if err := db.First(&migrated, "id = ?", oldJob).Error; err != nil || migrated.Queue != "source_backfill" {
		t.Fatalf("旧来源回填任务应迁入独立队列：%+v %v", migrated, err)
	}
	lateJob, err := oldManager.queue.Enqueue(context.Background(), "source_history_backfill", sourceHistoryArgs{Source: "javbooks"}, jobs.QueueOpt("backfill"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", lateJob).Delete(&jobs.Job{})
	completed := make(chan struct{}, 1)
	manager.queue.Register("source_history_backfill", func(context.Context, sourceHistoryArgs) error {
		select {
		case completed <- struct{}{}:
		default:
		}
		return nil
	})
	if err := manager.EnsureSourceBackfill(context.Background()); err != nil {
		t.Fatal(err)
	}
	migrated = jobs.Job{}
	if err := db.First(&migrated, "id = ?", lateJob).Error; err != nil || migrated.Queue != "source_backfill" {
		t.Fatalf("恢复后重新排队的旧任务应迁入独立队列：%+v %v", migrated, err)
	}
	runningJob, err := oldManager.queue.Enqueue(context.Background(), "source_history_backfill", sourceHistoryArgs{Source: "javquery"}, jobs.QueueOpt("backfill"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", runningJob).Delete(&jobs.Job{})
	if err := db.Model(&jobs.Job{}).Where("id = ?", runningJob).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureSourceBackfill(context.Background()); err != nil {
		t.Fatal(err)
	}
	migrated = jobs.Job{}
	if err := db.First(&migrated, "id = ?", runningJob).Error; err != nil || migrated.Queue != "backfill" {
		t.Fatalf("运行中的任务不能迁移：%+v %v", migrated, err)
	}
	if err := db.Where("id = ?", runningJob).Delete(&jobs.Job{}).Error; err != nil {
		t.Fatal(err)
	}
	blocked := make(chan struct{})
	started := make(chan struct{})
	manager.queue.Register("test_blocking_backfill", func(ctx context.Context, _ struct{}) error {
		close(started)
		select {
		case <-blocked:
		case <-ctx.Done():
		}
		return nil
	})
	if _, err := manager.queue.Enqueue(context.Background(), "test_blocking_backfill", struct{}{}, jobs.QueueOpt("backfill")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	defer close(blocked)
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("元数据回填未开始")
	}
	select {
	case <-completed:
	case <-time.After(10 * time.Second):
		t.Fatal("来源回填被元数据回填阻塞")
	}
}

func TestDatabaseQueueDeduplicatesAndRespectsCooldown(t *testing.T) {
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
	var calls atomic.Int32
	manager, err := NewManager(db, func(context.Context, string) (bool, error) {
		calls.Add(1)
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	id := fmt.Sprintf("JOB-%d", time.Now().UnixNano())
	defer db.Exec("DELETE FROM search_cooldowns WHERE normalized_id = ?", id)
	first, err := manager.Enqueue(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, manager, first)
	second, err := manager.Enqueue(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, manager, second)
	if calls.Load() != 1 {
		t.Fatalf("cooldown call count = %d", calls.Load())
	}
	forced, err := manager.Enqueue(context.Background(), id, true)
	if err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, manager, forced)
	if calls.Load() != 2 {
		t.Fatalf("forced call count = %d", calls.Load())
	}
}

func TestSearchResultReflectsVisibleMovieDuringCooldown(t *testing.T) {
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
	id := fmt.Sprintf("HIDDEN-%d", time.Now().UnixNano())
	movie := catalog.Movie{NormalizedID: id, CompressedID: id, IsHidden: true}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&movie)
	defer db.Exec("DELETE FROM search_cooldowns WHERE normalized_id = ?", id)
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	first, err := manager.Enqueue(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, manager, first)
	status, err := manager.Get(context.Background(), first)
	if err != nil || status.Result == nil || status.Result.Found {
		t.Fatalf("隐藏影片的检索结果 = %+v，错误 = %v", status, err)
	}
	if err := db.Model(&movie).Update("is_hidden", false).Error; err != nil {
		t.Fatal(err)
	}
	second, err := manager.Enqueue(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, manager, second)
	status, err = manager.Get(context.Background(), second)
	if err != nil || status.Result == nil || !status.Result.Found || !status.Result.Cached {
		t.Fatalf("冷却期的可见影片结果 = %+v，错误 = %v", status, err)
	}
	if err := db.Model(&movie).Update("is_hidden", true).Error; err != nil {
		t.Fatal(err)
	}
	status, err = manager.Get(context.Background(), second)
	if err != nil || status.Result == nil || status.Result.Found {
		t.Fatalf("隐藏后的历史任务结果 = %+v，错误 = %v", status, err)
	}
}

func waitCompleted(t *testing.T, manager *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.Get(context.Background(), id)
		if err == nil && job.Status == "completed" {
			return
		}
		if err == nil && job.Status == "failed" {
			t.Fatalf("job failed: %+v", job)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}

func TestScheduledTickQueuesMaintenanceJob(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	settings := DefaultScheduleSettings()
	settings.Timezone = "UTC"
	settings.Actress.Enabled = false
	settings.MGStage.Enabled = false
	settings.Fanza.Cron = "* * * * *"
	if err := manager.ConfigureMaintenance(func(context.Context) (ScheduleSettings, error) { return settings, nil }, map[string]func(context.Context) error{
		"actress_crawl":  func(context.Context) error { return nil },
		"fanza_crawl":    func(context.Context) error { calls.Add(1); return nil },
		"mgstage_crawl":  func(context.Context) error { return nil },
		"rebuild_movies": func(context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := manager.queue.Enqueue(ctx, "maintenance_tick", struct{}{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && calls.Load() == 0 {
		time.Sleep(100 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("maintenance call count = %d", calls.Load())
	}
}

func TestInterruptedJobIsRecoveredByAnotherWorker(t *testing.T) {
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
	started := make(chan struct{})
	var calls atomic.Int32
	search := func(ctx context.Context, _ string) (bool, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return false, ctx.Err()
		}
		return true, nil
	}
	first, err := NewManager(db, search)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("RECOVER-%d", time.Now().UnixNano())
	defer db.Exec("DELETE FROM search_cooldowns WHERE normalized_id = ?", id)
	job, err := first.Enqueue(context.Background(), id, true)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("job did not start")
	}
	cancel()
	first.Close()
	second, err := NewManager(db, search)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	waitCompleted(t, second, job)
	if calls.Load() < 2 {
		t.Fatalf("job was not retried: %d", calls.Load())
	}
}

func TestDuplicateSearchSharesJobButForcedSearchIsSeparate(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("DEDUP-%d", time.Now().UnixNano())
	first, err := manager.Enqueue(context.Background(), id, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Enqueue(context.Background(), id, false)
	if err != nil || second != first {
		t.Fatalf("duplicate job %q != %q: %v", second, first, err)
	}
	forced, err := manager.Enqueue(context.Background(), id, true)
	if err != nil || forced == first {
		t.Fatalf("forced job=%q first=%q err=%v", forced, first, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitCompleted(t, manager, first)
	waitCompleted(t, manager, forced)
	defer db.Exec("DELETE FROM search_cooldowns WHERE normalized_id = ?", id)
}

func TestFailedSearchIsRetried(t *testing.T) {
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
	var calls atomic.Int32
	manager, err := NewManager(db, func(context.Context, string) (bool, error) {
		if calls.Add(1) == 1 {
			return false, jobs.RetryAfter(200*time.Millisecond, errors.New("temporary failure"))
		}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	id := fmt.Sprintf("RETRY-%d", time.Now().UnixNano())
	defer db.Exec("DELETE FROM search_cooldowns WHERE normalized_id = ?", id)
	job, err := manager.Enqueue(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, manager, job)
	if calls.Load() != 2 {
		t.Fatalf("retry count = %d", calls.Load())
	}
}

func TestWorkerReportsStoppedAfterCancellation(t *testing.T) {
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
	manager, err := NewManager(db, func(context.Context, string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if !manager.Running() {
		t.Fatal("worker not marked running")
	}
	cancel()
	manager.Close()
	if manager.Running() {
		t.Fatal("stopped worker still marked running")
	}
}
