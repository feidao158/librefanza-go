package tasks

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
)

func TestSourceProbeHonorsManualDisable(t *testing.T) {
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
	var enabled atomic.Bool
	enabled.Store(true)
	var calls atomic.Int32
	manager.ConfigureSourceProbes(func(context.Context) (ScheduleSettings, error) {
		settings := DefaultScheduleSettings()
		settings.JavBus.Auto, settings.JavBooks.Auto, settings.JavQuery.Auto = false, false, false
		settings.JavDB.Auto = enabled.Load()
		return settings, nil
	}, func(context.Context, string) error { calls.Add(1); return nil })
	if err := db.Where("unique_key = ?", "source_probe:javdb").Delete(&jobs.Job{}).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Where("unique_key = ?", "source_probe:javdb").Delete(&jobs.Job{})
	if err := manager.EnsureSourceProbes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureSourceProbes(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&jobs.Job{}).Where("unique_key = ?", "source_probe:javdb").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("来源探测任务未去重: %d %v", count, err)
	}
	enabled.Store(false)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var job jobs.Job
		if err := db.Where("unique_key = ?", "source_probe:javdb").First(&job).Error; err != nil {
			t.Fatal(err)
		}
		if job.Status == jobs.StatusCompleted {
			if calls.Load() != 0 {
				t.Fatal("管理员禁用后仍执行来源探测")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("来源探测任务未完成")
}
