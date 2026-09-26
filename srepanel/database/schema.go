package database

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"log"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var fs embed.FS

type DB struct {
	*sql.DB
}

func Open(filePath string) (*DB, error) {
	// Initialize the database
	db, err := sql.Open("sqlite", filePath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}

	// Initialize migrations
	source, err := iofs.New(fs, "migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate iofs source failed: %w", err)
	}
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return nil, fmt.Errorf("migrate sqlite driver failed: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		return nil, fmt.Errorf("migrate creation failed: %w", err)
	}

	// Fetch the current schema version
	v, _, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return nil, fmt.Errorf("database version failed: %w", err)
	}
	log.Println("Schema version before migrations:", v)

	// Run any pending migrations
	err = m.Up()
	if err == nil {
		// We ran migrations, fetch the schema version again
		v, _, err = m.Version()
		if err != nil {
			return nil, fmt.Errorf("database version failed: %w", err)
		}
		log.Println("Schema version after migrations:", v)
	} else if !errors.Is(err, migrate.ErrNoChange) {
		return nil, fmt.Errorf("database migrations failed: %w", err)
	}

	return &DB{db}, nil
}
