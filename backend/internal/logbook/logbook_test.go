package logbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

func TestRecorderOutputsSafeFailureWithoutDatabase(t *testing.T) {
	secret := "private-secret-token"
	var output bytes.Buffer
	recorder := New(nil, &output)
	entry := Entry{Level: "error", Event: "task_failed", TaskType: "source_crawl", Source: "avmoo", TaskID: "4c8a24a1-bdeb-4b4d-96b5-6efdb1387829", Attempt: 2}
	ApplyFailure(&entry, &sources.CrawlError{PositionType: "page", Position: 4, Stage: "fetch", Err: &sources.RequestError{Reason: "http", Status: 403}})
	recorder.Record(context.Background(), entry)
	var result Entry
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("标准输出不是 JSON：%v", err)
	}
	if result.Reason != "http" || result.HTTPStatus != 403 || result.Position != 4 || result.Attempt != 2 {
		t.Fatalf("失败信息不完整：%+v", result)
	}
	if !strings.Contains(output.String(), "任务执行失败") || !strings.Contains(output.String(), "来源返回错误状态") {
		t.Fatalf("容器日志缺少可读的失败描述：%s", output.String())
	}
	output.Reset()
	ApplyFailure(&entry, errors.New("https://user:"+secret+"@example.com/?key="+secret))
	recorder.Record(context.Background(), entry)
	if strings.Contains(output.String(), secret) || entry.Reason != "unknown" {
		t.Fatalf("未知错误泄露了敏感信息：%s", output.String())
	}
}

func TestRecommendationLogSanitizesDetails(t *testing.T) {
	var output bytes.Buffer
	New(nil, &output).Record(context.Background(), Entry{Level: "info", Event: "recommendation_served", Source: "recommendation", DetailsJSON: `{"algorithm":"local","returned":12,"duration_ms":8,"secret":"private-secret-token"}`})
	if strings.Contains(output.String(), "private-secret-token") || !strings.Contains(output.String(), `\"returned\":12`) || !strings.Contains(output.String(), `"source":"recommendation"`) {
		t.Fatalf("推荐日志详情不正确或包含敏感信息：%s", output.String())
	}
}

func TestKnownConfigurationErrorsProvideSpecificSafeReason(t *testing.T) {
	for _, test := range []struct{ message, reason string }{
		{"Fanza credentials are not configured", "fanza_credentials"},
		{"MGStage series URL is not configured", "mgstage_series"},
		{"invalid proxy URL", "proxy_configuration"},
	} {
		entry := Entry{}
		ApplyFailure(&entry, errors.New(test.message))
		if entry.Reason != test.reason {
			t.Errorf("%s 的原因 = %s，期望 %s", test.message, entry.Reason, test.reason)
		}
	}
}

func TestRecorderPersistsFiltersAndCleansLogs(t *testing.T) {
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
	jobID := "4c8a24a1-bdeb-4b4d-96b5-6efdb1387829"
	defer db.Where("task_id = ?", jobID).Delete(&Entry{})
	recorder := New(db, &bytes.Buffer{})
	recorder.Record(context.Background(), Entry{Level: "error", Event: "task_failed", Source: "avmoo", TaskType: "source_crawl", TaskID: jobID, Reason: "dns"})
	recorder.Record(context.Background(), Entry{Level: "info", Event: "task_started", Source: "jable", TaskType: "source_crawl", TaskID: jobID})
	entries, total, err := List(context.Background(), db, Filter{Level: "error", Source: "avmoo", TaskID: jobID, Page: 1, PageSize: 1})
	if err != nil || total != 1 || len(entries) != 1 || entries[0].Reason != "dns" {
		t.Fatalf("日志筛选结果 = %+v，总数 = %d，错误 = %v", entries, total, err)
	}
	if err := db.Model(&Entry{}).Where("id = ?", entries[0].ID).Update("created_at", time.Now().UTC().Add(-31*24*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if err := recorder.Clean(context.Background(), time.Now().UTC().Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, total, err = List(context.Background(), db, Filter{Level: "error", Source: "avmoo", TaskID: jobID, Page: 1, PageSize: 1})
	if err != nil || total != 0 {
		t.Fatalf("过期日志未清理：%d，错误 = %v", total, err)
	}
}

func TestRecorderKeepsOutputWhenDatabaseIsUnavailable(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	connection, _ := db.DB()
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	New(db, &output).Record(context.Background(), Entry{Level: "error", Event: "task_failed", Reason: "dns"})
	if !strings.Contains(output.String(), `"reason":"dns"`) || !strings.Contains(output.String(), `"event":"log_storage_failed"`) {
		t.Fatalf("数据库不可用时缺少标准输出：%s", output.String())
	}
}
