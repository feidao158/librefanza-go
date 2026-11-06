package tasks

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
)

func TestCrashWorkerProcess(t *testing.T) {
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
	options := []jobs.WorkerOption{jobs.WithStaleLockInterval(500 * time.Millisecond), jobs.WithStaleLockAge(2 * time.Second), jobs.WithLockDuration(3 * time.Second)}
	if id := os.Getenv("LIBREFANZA_TEST_CRASH_ID"); id != "" {
		manager, err := newManager(db, func(_ context.Context, current string) (bool, error) {
			if current == id {
				fmt.Fprintln(os.Stdout, "LIBREFANZA_TEST_JOB_RUNNING")
				select {}
			}
			return true, nil
		}, options...)
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		select {}
	}
	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	var recovered atomic.Int32
	manager, err := newManager(db, func(context.Context, string) (bool, error) {
		recovered.Add(1)
		return true, nil
	}, options...)
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("CRASH-%d", time.Now().UnixNano())
	jobID, err := manager.Enqueue(context.Background(), id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM search_cooldowns WHERE normalized_id = ?", id)
	defer db.Where("id = ?", jobID).Delete(&jobs.Job{})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestCrashWorkerProcess$")
	child.Env = append(os.Environ(), "LIBREFANZA_TEST_CRASH_ID="+id)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	started := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "LIBREFANZA_TEST_JOB_RUNNING") {
				started <- true
				return
			}
		}
		started <- false
	}()
	select {
	case ok := <-started:
		if !ok {
			t.Fatal("子进程未开始执行任务")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("子进程启动任务超时")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitCompleted(t, manager, jobID)
	if recovered.Load() != 1 {
		t.Fatalf("进程退出后的恢复次数 = %d", recovered.Load())
	}
}
