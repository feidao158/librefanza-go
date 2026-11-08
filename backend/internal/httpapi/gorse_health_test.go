package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGorseHealthRequiresReadyResponse(t *testing.T) {
	ready := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health/ready" {
			t.Errorf("健康检查路径错误：%s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("缺少推荐服务认证")
		}
		if ready {
			_, _ = w.Write([]byte(`{"Ready":true}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if checkGorseHealth(context.Background(), server.URL, "test-key") {
		t.Fatal("未就绪服务不应连接成功")
	}
	ready = true
	if !checkGorseHealth(context.Background(), server.URL, "test-key") {
		t.Fatal("已就绪服务检查失败")
	}
}
