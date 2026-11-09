package database

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/libredmm/librefanza/backend/internal/config"
)

func TestConnectionStringEscapesCredentials(t *testing.T) {
	for _, driver := range []string{"postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, Host: "127.0.0.1", Port: 1234, Name: "librefanza", User: "app", Password: "a:/@?+% !"}
			dsn, err := ConnectionString(cfg)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := pgx.ParseConfig(dsn)
			if err != nil || parsed.Password != cfg.Password || parsed.Database != cfg.Name {
				t.Fatalf("invalid postgres configuration: %#v %v", parsed, err)
			}
		})
	}
}

func TestConnectionStringRejectsInvalidConfiguration(t *testing.T) {
	for _, cfg := range []config.Database{
		{Driver: "mysql", Host: "localhost", Port: 3306, Name: "app", User: "app"},
		{Driver: "sqlite", Host: "localhost", Port: 5432, Name: "app", User: "app"},
		{Driver: "postgres", Host: "", Port: 5432, Name: "app", User: "app"},
		{Driver: "postgres", Host: "localhost", Port: 0, Name: "app", User: "app"},
	} {
		if _, err := ConnectionString(cfg); err == nil {
			t.Fatalf("accepted invalid database configuration: %+v", cfg)
		}
	}
}
