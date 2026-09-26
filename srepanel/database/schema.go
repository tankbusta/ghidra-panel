package database

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"log"
	_ "modernc.org/sqlite"
	"strconv"
	"strings"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var fs embed.FS

type DB struct {
	*sql.DB
	postgres bool
}

// IsPostgres reports whether dsn refers to a PostgreSQL database
// rather than a SQLite file path.
func IsPostgres(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// Open opens a PostgreSQL database if dsn is a postgres:// URL,
// otherwise a SQLite database at the file path dsn.
func Open(dsn string) (*DB, error) {
	var (
		db      *sql.DB
		dialect string
		err     error
	)
	postgres := IsPostgres(dsn)
	if postgres {
		dialect = "postgres"
		db, err = sql.Open("pgx", dsn)
	} else {
		dialect = "sqlite"
		db, err = sql.Open("sqlite", dsn+"?_pragma=journal_mode(WAL)")
	}
	if err != nil {
		return nil, err
	}

	// Initialize migrations
	source, err := iofs.New(fs, "migrations/"+dialect)
	if err != nil {
		return nil, fmt.Errorf("migrate iofs source failed: %w", err)
	}
	var driver database.Driver
	if postgres {
		driver, err = migratepgx.WithInstance(db, &migratepgx.Config{})
	} else {
		driver, err = sqlite.WithInstance(db, &sqlite.Config{})
	}
	if err != nil {
		return nil, fmt.Errorf("migrate %s driver failed: %w", dialect, err)
	}
	m, err := migrate.NewWithInstance("iofs", source, dialect, driver)
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

	return &DB{DB: db, postgres: postgres}, nil
}

// rebind converts ? placeholders to the $N form expected by PostgreSQL.
// Queries must not contain literal question marks.
func (d *DB) rebind(query string) string {
	if !d.postgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, c := range query {
		if c == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}
