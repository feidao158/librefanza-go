package tasks

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

func TestCrawlJobDeduplicationAndRetryProgress(t *testing.T) {
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
	var attempts atomic.Int32
	manager.ConfigureCrawls(func(_ context.Context, _ sources.CrawlRequest, progress sources.CrawlProgress, save func(sources.CrawlProgress) error) error {
		if attempts.Add(1) == 1 {
			if err := save(sources.CrawlProgress{NextPage: 4, Processed: 10}); err != nil {
				return err
			}
			return &sources.CrawlError{PositionType: "page", Position: 3, Stage: "fetch", Err: &sources.RequestError{Reason: "http", Status: 403}}
		}
		if progress.NextPage != 4 || progress.Processed != 10 {
			return errors.New("进度未恢复")
		}
		return save(sources.CrawlProgress{NextPage: 5, Processed: 12})
	})
	request := sources.CrawlRequest{Source: "libredmm", Mode: "full"}
	id, err := manager.EnqueueCrawl(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := jobs.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Where("job_id = ?", id).Delete(&CrawlProgress{})
	defer db.Where("id = ?", jobID).Delete(&jobs.Job{})
	defer db.Where("task_id = ?", id).Delete(&logbook.Entry{})
	again, err := manager.EnqueueCrawl(context.Background(), request)
	if err != nil || again != id {
		t.Fatalf("重复任务 = %s，错误 = %v", again, err)
	}
	if _, err := manager.EnqueueCrawl(context.Background(), sources.CrawlRequest{Source: "libredmm", Mode: "incremental"}); !errors.Is(err, ErrCrawlInProgress) {
		t.Fatalf("并发任务错误 = %v", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var job jobs.Job
		if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
			t.Fatal(err)
		}
		if job.Status == "completed" {
			var progress CrawlProgress
			if err := db.Where("job_id = ?", id).First(&progress).Error; err != nil || progress.NextPage != 5 || progress.Processed != 12 || attempts.Load() < 2 {
				t.Fatalf("最终进度 = %+v，次数 = %d，错误 = %v", progress, attempts.Load(), err)
			}
			var entries []logbook.Entry
			if err := db.Where("task_id = ?", id).Order("id ASC").Find(&entries).Error; err != nil {
				t.Fatal(err)
			}
			var failure, completed bool
			for _, entry := range entries {
				if entry.Event == "task_failed" && entry.Source == "libredmm" && entry.Attempt == 1 && entry.Position == 3 && entry.HTTPStatus == 403 && entry.Reason == "http" {
					failure = true
				}
				if entry.Event == "task_completed" && entry.Attempt >= 2 {
					completed = true
				}
			}
			if !failure || !completed {
				t.Fatalf("缺少重试日志：%+v", entries)
			}
			return
		}
		if job.Status == "failed" {
			t.Fatalf("任务失败：%s", job.LastError)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("采集任务重试超时")
}

func TestCrawlPanicRetriesWithoutLoggingSecret(t *testing.T) {
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
	var output bytes.Buffer
	manager.logs = logbook.New(db, &output)
	var attempts atomic.Int32
	secret := "private-panic-token"
	manager.ConfigureCrawls(func(context.Context, sources.CrawlRequest, sources.CrawlProgress, func(sources.CrawlProgress) error) error {
		if attempts.Add(1) == 1 {
			panic(secret)
		}
		return nil
	})
	id, err := manager.EnqueueCrawl(context.Background(), sources.CrawlRequest{Source: "jable", Mode: "incremental"})
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := jobs.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Where("task_id = ?", id).Delete(&logbook.Entry{})
	defer db.Where("job_id = ?", id).Delete(&CrawlProgress{})
	defer db.Where("id = ?", jobID).Delete(&jobs.Job{})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var job jobs.Job
		if err := db.Where("id = ?", jobID).First(&job).Error; err != nil {
			t.Fatal(err)
		}
		if job.Status == "completed" {
			var entry logbook.Entry
			if err := db.Where("task_id = ? AND event = ?", id, "task_failed").First(&entry).Error; err != nil || entry.Reason != "panic" {
				t.Fatalf("异常日志 = %+v，错误 = %v", entry, err)
			}
			if attempts.Load() < 2 || strings.Contains(output.String(), secret) || strings.Contains(job.LastError, secret) {
				t.Fatalf("异常重试不正确或泄露敏感信息：次数 = %d", attempts.Load())
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("异常任务重试超时")
}
