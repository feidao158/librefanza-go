package setup

import (
	"context"
	"errors"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofrs/flock"
	"github.com/libredmm/librefanza/backend/internal/config"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrAlreadyInstalled = errors.New("application is already installed")
	ErrPendingMismatch  = errors.New("setup parameters do not match the pending installation")
	ErrInvalidInput     = errors.New("invalid setup input")
	ErrSetupLocked      = errors.New("setup is locked by another process")
)

type Redis = config.Redis

type Request struct {
	Database        config.Database `json:"database"`
	ManagedDatabase bool            `json:"managed_database"`
	Redis           Redis           `json:"redis"`
	Email           string          `json:"email"`
	Password        string          `json:"password"`
}

// ManagedDatabase 返回服务端配置的内置数据库连接信息。
func ManagedDatabase() (config.Database, bool) {
	host := strings.TrimSpace(os.Getenv("LIBREFANZA_MANAGED_DB_HOST"))
	name := strings.TrimSpace(os.Getenv("LIBREFANZA_MANAGED_DB_NAME"))
	user := strings.TrimSpace(os.Getenv("LIBREFANZA_MANAGED_DB_USER"))
	password := os.Getenv("LIBREFANZA_MANAGED_DB_PASSWORD")
	if host == "" || name == "" || user == "" || password == "" {
		return config.Database{}, false
	}
	return config.Database{Driver: "postgres", Host: host, Port: 5432, Name: name, User: user, Password: password}, true
}

type Backend interface {
	CheckDatabase(context.Context, config.Database) error
	CheckRedis(context.Context, Redis) error
	Prepare(context.Context, config.Database, Redis, string, string) error
}

type Service struct {
	store   *config.Store
	backend Backend
	mu      sync.Mutex
	lock    *flock.Flock
	lockErr error
}

func NewService(store *config.Store, backend Backend) *Service {
	service := &Service{store: store, backend: backend}
	if err := os.MkdirAll(store.Directory(), 0700); err != nil {
		service.lockErr = err
		return service
	}
	lock := flock.New(filepath.Join(store.Directory(), ".setup.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		service.lockErr = err
	} else if !locked {
		service.lockErr = ErrSetupLocked
	} else {
		service.lock = lock
	}
	return service
}

func (s *Service) Installed() (bool, error) {
	if s.lockErr != nil {
		return false, s.lockErr
	}
	cfg, found, err := s.store.Load()
	if err != nil {
		return false, err
	}
	return found && cfg.Installed, nil
}

func (s *Service) CheckDatabase(ctx context.Context, cfg config.Database) error {
	installed, err := s.Installed()
	if err != nil {
		return err
	}
	if installed {
		return ErrAlreadyInstalled
	}
	return s.backend.CheckDatabase(ctx, cfg)
}

func (s *Service) CheckRedis(ctx context.Context, cfg Redis) error {
	installed, err := s.Installed()
	if err != nil {
		return err
	}
	if installed {
		return ErrAlreadyInstalled
	}
	return s.backend.CheckRedis(ctx, cfg)
}

func (s *Service) Initialize(ctx context.Context, request Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockErr != nil {
		return s.lockErr
	}
	if request.ManagedDatabase {
		var ok bool
		request.Database, ok = ManagedDatabase()
		if !ok {
			return ErrInvalidInput
		}
	}

	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	address, err := mail.ParseAddress(request.Email)
	if err != nil || address.Address != request.Email || len(request.Password) < 8 || len(request.Password) > 72 {
		return ErrInvalidInput
	}
	cfg, found, err := s.store.Load()
	if err != nil {
		return err
	}
	if found && cfg.Installed {
		return ErrAlreadyInstalled
	}
	if found {
		if cfg.Database != request.Database || cfg.PendingEmail != request.Email || cfg.PendingRedis != request.Redis || bcrypt.CompareHashAndPassword([]byte(cfg.PendingPassword), []byte(request.Password)) != nil {
			return ErrPendingMismatch
		}
	} else {
		if err := s.backend.CheckDatabase(ctx, request.Database); err != nil {
			return err
		}
		if request.Redis.Enabled {
			if err := s.backend.CheckRedis(ctx, request.Redis); err != nil {
				return err
			}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
		if err != nil {
			return ErrInvalidInput
		}
		cfg = config.Config{Database: request.Database, PendingEmail: request.Email, PendingPassword: string(hash), PendingRedis: request.Redis}
		if err := s.store.Save(cfg); err != nil {
			return err
		}
	}
	if err := s.backend.Prepare(ctx, cfg.Database, cfg.PendingRedis, cfg.PendingEmail, cfg.PendingPassword); err != nil {
		return err
	}
	cfg.Installed = true
	cfg.PendingEmail = ""
	cfg.PendingPassword = ""
	cfg.PendingRedis = Redis{}
	return s.store.Save(cfg)
}

func (s *Service) Close() error {
	if s.lock == nil {
		return nil
	}
	return s.lock.Unlock()
}
