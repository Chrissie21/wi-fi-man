package repository

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(databaseDSN, migrationsPath string) error {
	if databaseDSN == "" {
		return fmt.Errorf("database dsn is required")
	}
	if migrationsPath == "" {
		return fmt.Errorf("migrations path is required")
	}
	absPath, err := filepath.Abs(migrationsPath)
	if err != nil {
		return err
	}
	m, err := migrate.New("file://"+absPath, databaseDSN)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
