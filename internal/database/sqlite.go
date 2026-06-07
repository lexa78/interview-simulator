package database

import (
	"database/sql"
	"errors"
	"fmt"
	"interview/migrations"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"modernc.org/sqlite"
)

func init() {
	sql.Register("modernc-sqlite", &sqlite.Driver{})
}

func InitDB(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", dbPath)

	db, err := sql.Open("modernc-sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err = runMigrations(db); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			slog.Warn("failed to close migrations table", "error", closeErr)
		}
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	// Читаем файлы миграций из embed.FS
	d, err := iofs.New(migrations.Files, ".")
	if err != nil {
		return fmt.Errorf("read migrations file from embed.FS: %w", err)
	}

	// Инициализируем драйвер мигратора. Он под капотом ожидает имя "sqlite3"
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("driver init: %w", err)
	}

	// Запускаем процесс
	m, err := migrate.NewWithInstance("iofs", d, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("run process: %w", err)
	}

	// Накатываем миграции UP
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrations UP: %w", err)
	}

	return nil
}
