package ingest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
)

func TestImprovedCoverRequiresLargerMeasuredPixels(t *testing.T) {
	for _, test := range []struct {
		old, candidate image.Config
		hasOld         bool
		want           bool
	}{
		{image.Config{Width: 147, Height: 200}, image.Config{Width: 800, Height: 538}, true, true},
		{image.Config{Width: 800, Height: 538}, image.Config{Width: 147, Height: 200}, true, false},
		{image.Config{Width: 800, Height: 538}, image.Config{Width: 800, Height: 538}, true, false},
		{image.Config{}, image.Config{Width: 147, Height: 200}, false, false},
		{image.Config{}, image.Config{Width: 800, Height: 538}, false, true},
	} {
		if got := improvedCover(test.old, test.candidate, test.hasOld); got != test.want {
			t.Fatalf("原图=%+v 新图=%+v 旧图存在=%v 结果=%v", test.old, test.candidate, test.hasOld, got)
		}
	}
}

func TestCoverRepairMatchesOriginalSourceVersion(t *testing.T) {
	original := catalog.SourceItem{NormalizedID: "ABC-123", ExternalID: "version-one"}
	if sameCoverItem(original, catalog.SourceItem{NormalizedID: "ABC-123", ExternalID: "version-two"}) {
		t.Fatal("同番号不同来源版本不能共用封面")
	}
	if !sameCoverItem(original, catalog.SourceItem{NormalizedID: "ABC-123", ExternalID: "version-one"}) {
		t.Fatal("相同来源版本应允许比较封面")
	}
}

func TestCoverRepairKeepsManualCoverAndNewerConcurrentSourceCover(t *testing.T) {
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
	id := fmt.Sprintf("COVER-%d", time.Now().UnixNano())
	movie := catalog.Movie{NormalizedID: id, CompressedID: id, CoverImageURL: "https://example.com/manual.jpg"}
	if err := tx.Create(&movie).Error; err != nil {
		t.Fatal(err)
	}
	item := catalog.SourceItem{Source: "libredmm", ExternalID: id, NormalizedID: id, Title: "标题", CoverImageURL: "https://example.com/small.jpg"}
	if err := tx.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(tx)
	if err := service.saveImprovedCover(context.Background(), item, "https://example.com/large.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&movie, movie.ID).Error; err != nil || movie.CoverImageURL != "https://example.com/manual.jpg" {
		t.Fatalf("手动封面被覆盖: %+v %v", movie, err)
	}
	if err := service.saveImprovedCover(context.Background(), item, "https://example.com/stale.jpg"); err != nil {
		t.Fatal(err)
	}
	var actual catalog.SourceItem
	if err := tx.First(&actual, item.ID).Error; err != nil || actual.CoverImageURL != "https://example.com/large.jpg" {
		t.Fatalf("已更新的来源封面被覆盖: %+v %v", actual, err)
	}
}

func TestCoverImageAddressRejectsInternalNetworks(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.0.1", "::1", "fc00::1"} {
		if publicImageIP(net.ParseIP(raw)) {
			t.Errorf("不应允许内部地址 %s", raw)
		}
	}
	if !publicImageIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("应允许公开地址")
	}
}

func TestCoverSupportsWebPDimensions(t *testing.T) {
	body, err := base64.StdEncoding.DecodeString("UklGRooJAABXRUJQVlA4IH4JAAAyLwCdASqWAGQAPo04lUeioYwGg0EUBGJZQDWZNsO1JP5n64fB3Ub8v+o9XG3d53fTuYGQ1jfacgmJk6PthcDb6Dzr0z01vyivwkv9E+4YUHpwH65k6mVaIXqrC4XTBQLqkGc1rCgGes04gKPUimJAmZhyXeG4DBy84mCBiQxZwB3y0ABGURmcpLymWUnJPKhGe8RujkoIZ810gU5JiH/ONuOi9qprpe/D+igi50G8+UMtjxOVM02DI1cX8zoiV/kuK0uHAtfQaqJC3eZhgnSAYCLcn9Ayj9pMHbfk0JJRbdF9H8CZCBhGkcIF2A==")
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || format != "webp" || config.Width != 150 || config.Height != 100 {
		t.Fatalf("WebP 尺寸解码失败: %+v %s %v", config, format, err)
	}
}

func TestImageProxyUsesValidatedFixedAddressAndTLSHost(t *testing.T) {
	var gotTarget, gotAuthorization string
	var mu sync.Mutex
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotTarget, gotAuthorization = r.Host, r.Header.Get("Proxy-Authorization")
		mu.Unlock()
		connection, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		_, _ = io.Copy(io.Discard, connection)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	proxyURL.User = url.UserPassword("user", "secret")
	dial := imageDialer(proxyURL, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})
	conn, err := dial(context.Background(), "tcp", "images.example:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	mu.Lock()
	defer mu.Unlock()
	if gotTarget != "8.8.8.8:443" || gotAuthorization != "Basic dXNlcjpzZWNyZXQ=" {
		t.Fatalf("代理目标或凭据错误: %s %s", gotTarget, gotAuthorization)
	}
}

func TestImageProxyPreservesOriginalTLSHostname(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream, err := net.Dial("tcp", origin.Listener.Addr().String())
		if err != nil {
			return
		}
		defer upstream.Close()
		connection, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		go func() { _, _ = io.Copy(upstream, buffer) }()
		_, _ = io.Copy(connection, upstream)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	transport := &http.Transport{DialContext: imageDialer(proxyURL, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}), TLSClientConfig: &tls.Config{RootCAs: origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Get("https://incorrect-certificate.example/")
	if err == nil {
		response.Body.Close()
		t.Fatal("证书名称不匹配时不能成功访问")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("预期校验证书名，实际错误: %v", err)
	}
}
