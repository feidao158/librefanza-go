package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/auth"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func TestLogsRequireAuthentication(t *testing.T) {
	store := config.NewStore(t.TempDir())
	service := setup.NewService(store, setupBackend{})
	defer service.Close()
	router := NewRouter(service, database.NewProvider(store))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("未登录日志接口状态码 = %d", response.Code)
	}
}

func TestAdminLogsFilterAndPermissions(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	if err := store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}}); err != nil {
		t.Fatal(err)
	}
	provider := database.NewProvider(store)
	t.Cleanup(func() { _ = provider.Close() })
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(db)
	makeToken := func(admin bool) string {
		user, err := service.Register(context.Background(), fmt.Sprintf("logs-%t-%d@example.com", admin, time.Now().UnixNano()), "password123")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Delete(&user) })
		if admin {
			if err := db.Model(&user).Update("is_admin", true).Error; err != nil {
				t.Fatal(err)
			}
		}
		tokens, err := service.Login(context.Background(), user.Email, "password123")
		if err != nil {
			t.Fatal(err)
		}
		return tokens.AccessToken
	}
	adminToken, userToken := makeToken(true), makeToken(false)
	id := "7281166a-f25c-4dbb-8c06-179d4da365c0"
	defer db.Where("task_id = ?", id).Delete(&logbook.Entry{})
	logs := logbook.New(db, io.Discard)
	logs.Record(context.Background(), logbook.Entry{Level: "error", Event: "task_failed", Source: "jable", TaskType: "source_crawl", TaskID: id, Reason: "http", HTTPStatus: 403, PositionType: "page", Position: 2})
	logs.Record(context.Background(), logbook.Entry{Level: "info", Event: "task_started", Source: "jable", TaskType: "source_crawl", TaskID: id})
	setupService := setup.NewService(store, setupBackend{})
	defer setupService.Close()
	router := NewRouter(setupService, provider)
	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?level=error&source=jable&task_id="+id, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if response := request(userToken); response.Code != http.StatusForbidden {
		t.Fatalf("普通用户日志接口状态码 = %d", response.Code)
	}
	response := request(adminToken)
	var body struct {
		Data []logbook.Entry `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || len(body.Data) != 1 || body.Meta.Total != 1 || body.Data[0].HTTPStatus != 403 || body.Data[0].Position != 2 {
		t.Fatalf("日志接口结果 = %d %s", response.Code, response.Body.String())
	}
}
