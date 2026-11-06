package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

//go:embed migrations/*/*.sql
var migrations embed.FS

func Migrate(ctx context.Context, db *gorm.DB, driver string) error {
	if driver != "mysql" && driver != "postgres" {
		return fmt.Errorf("unsupported database driver: %s", driver)
	}
	files, err := fs.Sub(migrations, "migrations/"+driver)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.Dialect(driver), sqlDB, files)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
