package cache

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/settings"
)

func TestRedisCacheAndVersionInvalidation(t *testing.T) {
	driver, redisPort := os.Getenv("LIBREFANZA_TEST_DB_DRIVER"), os.Getenv("LIBREFANZA_TEST_REDIS_PORT")
	if driver == "" || redisPort == "" {
		t.Skip("需要设置测试数据库和 Redis 连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	if err := store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}}); err != nil {
		t.Fatal(err)
	}
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	configService := settings.NewService(db)
	if err := configService.UpdateRedis(context.Background(), config.Redis{Enabled: true, Address: fmt.Sprintf("127.0.0.1:%s", redisPort)}); err != nil {
		t.Fatal(err)
	}
	defer configService.UpdateRedis(context.Background(), config.Redis{Enabled: false})
	manager := NewManager(provider)
	defer manager.Close()
	_, handle := manager.Get(context.Background(), "test")
	if handle == nil {
		t.Fatal("redis was not enabled")
	}
	handle.Set(context.Background(), []byte("cached"))
	if value, _ := manager.Get(context.Background(), "test"); string(value) != "cached" {
		t.Fatalf("cache value = %q", value)
	}
	if err := BumpVersion(db); err != nil {
		t.Fatal(err)
	}
	if value, _ := manager.Get(context.Background(), "test"); value != nil {
		t.Fatalf("stale cache returned after version changed: %q", value)
	}
	if err := configService.UpdateRedis(context.Background(), config.Redis{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, handle := manager.Get(context.Background(), "test"); handle != nil {
		t.Fatal("disabled cache returned a handle")
	}
}

func TestRedisFailureFallsBackWithoutBlocking(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	_ = store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}})
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM settings WHERE name = ?", "redis")
	value := `{"enabled":true,"address":"127.0.0.1:1","database":0}`
	if err := db.Exec("UPDATE settings SET value = ? WHERE name = ?", value, "redis").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO settings (name, value, updated_at) SELECT ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM settings WHERE name = ?)", "redis", value, time.Now().UTC(), "redis").Error; err != nil {
		t.Fatal(err)
	}
	manager := NewManager(provider)
	defer manager.Close()
	start := time.Now()
	if value, handle := manager.Get(context.Background(), "missing"); value != nil || handle != nil {
		t.Fatal("unavailable cache did not fall back")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("unavailable cache blocked the request")
	}
}

func TestRedisDisconnectAndRecovery(t *testing.T) {
	driver, redisPort := os.Getenv("LIBREFANZA_TEST_DB_DRIVER"), os.Getenv("LIBREFANZA_TEST_REDIS_PORT")
	if driver == "" || redisPort == "" {
		t.Skip("需要设置测试数据库和 Redis 连接")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var mu sync.Mutex
	blocked := false
	connections := make(map[net.Conn]struct{})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if blocked {
				mu.Unlock()
				_ = conn.Close()
				continue
			}
			connections[conn] = struct{}{}
			mu.Unlock()
			go func() {
				defer func() {
					mu.Lock()
					delete(connections, conn)
					mu.Unlock()
					_ = conn.Close()
				}()
				upstream, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", redisPort))
				if err != nil {
					return
				}
				defer upstream.Close()
				go func() { _, _ = io.Copy(upstream, conn); _ = upstream.Close() }()
				_, _ = io.Copy(conn, upstream)
			}()
		}
	}()

	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	if err := store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}}); err != nil {
		t.Fatal(err)
	}
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	service := settings.NewService(db)
	if err := service.UpdateRedis(context.Background(), config.Redis{Enabled: true, Address: listener.Addr().String()}); err != nil {
		t.Fatal(err)
	}
	defer service.UpdateRedis(context.Background(), config.Redis{Enabled: false})
	manager := NewManager(provider)
	defer manager.Close()
	_, handle := manager.Get(context.Background(), "recovery")
	if handle == nil {
		t.Fatal("redis did not connect")
	}
	handle.Set(context.Background(), []byte("before"))
	mu.Lock()
	blocked = true
	for conn := range connections {
		_ = conn.Close()
	}
	mu.Unlock()
	if value, handle := manager.Get(context.Background(), "recovery"); value != nil || handle != nil {
		t.Fatal("disconnected redis did not fall back")
	}
	mu.Lock()
	blocked = false
	mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if value, _ := manager.Get(context.Background(), "recovery"); string(value) == "before" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("redis did not recover")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
