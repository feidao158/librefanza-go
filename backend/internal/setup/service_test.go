package setup

import (
	"context"
	"errors"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/config"
)

type fakeBackend struct {
	preparations int
	fail         bool
}

func (f *fakeBackend) CheckDatabase(context.Context, config.Database) error { return nil }
func (f *fakeBackend) CheckRedis(context.Context, Redis) error              { return nil }
func (f *fakeBackend) Prepare(context.Context, config.Database, Redis, string, string) error {
	f.preparations++
	if f.fail {
		return errors.New("prepare failed")
	}
	return nil
}

func testRequest() Request {
	return Request{Database: config.Database{Driver: "postgres", Host: "localhost", Port: 5432, Name: "app", User: "app"}, Email: "admin@example.com", Password: "password123"}
}

func TestSetupCanOnlyCompleteOnce(t *testing.T) {
	backend := &fakeBackend{}
	service := NewService(config.NewStore(t.TempDir()), backend)
	if installed, err := service.Installed(); err != nil || installed {
		t.Fatalf("initial state: installed=%v err=%v", installed, err)
	}
	if err := service.Initialize(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if installed, err := service.Installed(); err != nil || !installed {
		t.Fatalf("completed state: installed=%v err=%v", installed, err)
	}
	if err := service.Initialize(context.Background(), testRequest()); !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("second initialization: %v", err)
	}
	if backend.preparations != 1 {
		t.Fatalf("preparation count = %d", backend.preparations)
	}
}

func TestManagedDatabaseUsesServerConfiguration(t *testing.T) {
	t.Setenv("LIBREFANZA_MANAGED_DB_HOST", "postgres")
	t.Setenv("LIBREFANZA_MANAGED_DB_NAME", "internal")
	t.Setenv("LIBREFANZA_MANAGED_DB_USER", "internal")
	t.Setenv("LIBREFANZA_MANAGED_DB_PASSWORD", "private-password")
	store := config.NewStore(t.TempDir())
	service := NewService(store, &fakeBackend{})
	defer service.Close()
	request := testRequest()
	request.ManagedDatabase = true
	request.Database.Host = "untrusted-client-host"
	if err := service.Initialize(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := store.Load()
	if err != nil || cfg.Database.Host != "postgres" || cfg.Database.Password != "private-password" || cfg.Database.Driver != "postgres" {
		t.Fatalf("托管数据库配置不正确：%v", err)
	}
}

func TestManagedDatabaseRejectsMissingServerCredentials(t *testing.T) {
	t.Setenv("LIBREFANZA_MANAGED_DB_HOST", "")
	backend := &fakeBackend{}
	service := NewService(config.NewStore(t.TempDir()), backend)
	defer service.Close()
	request := testRequest()
	request.ManagedDatabase = true
	if err := service.Initialize(context.Background(), request); !errors.Is(err, ErrInvalidInput) || backend.preparations != 0 {
		t.Fatalf("内置数据库配置缺失时应拒绝安装：%v", err)
	}
}

func TestSetupResumesAfterPreparationFailure(t *testing.T) {
	backend := &fakeBackend{fail: true}
	service := NewService(config.NewStore(t.TempDir()), backend)
	request := testRequest()
	if err := service.Initialize(context.Background(), request); err == nil {
		t.Fatal("preparation error was ignored")
	}
	changed := request
	changed.Password = "another-password"
	if err := service.Initialize(context.Background(), changed); !errors.Is(err, ErrPendingMismatch) {
		t.Fatalf("pending credentials changed: %v", err)
	}
	backend.fail = false
	if err := service.Initialize(context.Background(), request); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
}

func TestInstallationRequiresExclusiveStateDirectory(t *testing.T) {
	dir := t.TempDir()
	first := NewService(config.NewStore(dir), &fakeBackend{})
	defer first.Close()
	second := NewService(config.NewStore(dir), &fakeBackend{})
	defer second.Close()
	if err := second.Initialize(context.Background(), testRequest()); !errors.Is(err, ErrSetupLocked) {
		t.Fatalf("second installer did not fail closed: %v", err)
	}
}

func TestInitializationRejectsInvalidAdminWithoutPreparing(t *testing.T) {
	backend := &fakeBackend{}
	service := NewService(config.NewStore(t.TempDir()), backend)
	defer service.Close()
	request := testRequest()
	request.Email = "invalid"
	if err := service.Initialize(context.Background(), request); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid email: %v", err)
	}
	request = testRequest()
	request.Password = "short"
	if err := service.Initialize(context.Background(), request); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid password: %v", err)
	}
	if backend.preparations != 0 {
		t.Fatalf("unexpected preparations: %d", backend.preparations)
	}
}
