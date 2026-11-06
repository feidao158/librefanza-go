package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Database struct {
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password"`
	TLS      bool   `json:"tls"`
}

type Redis struct {
	Enabled       bool   `json:"enabled"`
	Address       string `json:"address"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	ClearPassword bool   `json:"clear_password,omitempty"`
	Database      int    `json:"database"`
	TLS           bool   `json:"tls"`
}

type Config struct {
	Database        Database `json:"database"`
	Installed       bool     `json:"installed"`
	PendingEmail    string   `json:"pending_email,omitempty"`
	PendingPassword string   `json:"pending_password_hash,omitempty"`
	PendingRedis    Redis    `json:"pending_redis,omitempty"`
}

type Store struct {
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) Directory() string { return s.dir }

func (s *Store) Load() (Config, bool, error) {
	var cfg Config
	data, err := os.ReadFile(filepath.Join(s.dir, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, true, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, true, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Database.Driver == "" {
		return Config{}, true, errors.New("invalid configuration")
	}
	return cfg, true, nil
}

func (s *Store) Save(cfg Config) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	file, err := os.CreateTemp(s.dir, ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(name, filepath.Join(s.dir, "config.json")); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
