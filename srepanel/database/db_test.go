package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// forEachBackend runs fn against SQLite, and against PostgreSQL
// if TEST_POSTGRES_URL is set.
func forEachBackend(t *testing.T, fn func(t *testing.T, db *DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := Open(filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		fn(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("TEST_POSTGRES_URL")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_URL not set")
		}
		db, err := Open(isolatedSchema(t, dsn))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		fn(t, db)
	})
}

// isolatedSchema creates a throwaway schema and returns dsn with it as search_path.
func isolatedSchema(t *testing.T, dsn string) string {
	var buf [8]byte
	rand.Read(buf[:])
	schema := "test_" + hex.EncodeToString(buf[:])

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestRebind(t *testing.T) {
	const query = "SELECT a FROM t WHERE b = ? AND c = ?"
	sqlite := &DB{}
	if got := sqlite.rebind(query); got != query {
		t.Errorf("sqlite rebind = %q", got)
	}
	pg := &DB{postgres: true}
	if got, want := pg.rebind(query), "SELECT a FROM t WHERE b = $1 AND c = $2"; got != want {
		t.Errorf("postgres rebind = %q, want %q", got, want)
	}
}

func TestIsPostgres(t *testing.T) {
	for dsn, want := range map[string]bool{
		"ghidra_panel.db":           false,
		"/data/ghidra_panel.db":     false,
		"postgres://u@h/db":         true,
		"postgresql://u@h/db?x=y":   true,
		"postgres-backup/ghidra.db": false,
	} {
		if got := IsPostgres(dsn); got != want {
			t.Errorf("IsPostgres(%q) = %v, want %v", dsn, got, want)
		}
	}
}

func TestRepositoryWebhook(t *testing.T) {
	forEachBackend(t, func(t *testing.T, db *DB) {
		ctx := t.Context()
		repo, err := db.GetRepository(ctx, "missing")
		if err != nil {
			t.Fatal(err)
		}
		if repo.WebhookURL != "" {
			t.Fatalf("unexpected webhook %q", repo.WebhookURL)
		}
		for _, hook := range []string{"https://a", "https://b"} {
			if err := db.SetRepositoryWebhook(ctx, "repo", hook); err != nil {
				t.Fatal(err)
			}
			repo, err := db.GetRepository(ctx, "repo")
			if err != nil {
				t.Fatal(err)
			}
			if repo.WebhookURL != hook {
				t.Fatalf("webhook = %q, want %q", repo.WebhookURL, hook)
			}
		}
	})
}

func TestAccounts(t *testing.T) {
	forEachBackend(t, func(t *testing.T, db *DB) {
		ctx := t.Context()
		if exists, err := db.UsernameExists(ctx, "alice"); err != nil || exists {
			t.Fatalf("UsernameExists = %v, %v", exists, err)
		}
		if err := db.CreateAccount(ctx, 42, "alice", "hunter2"); err != nil {
			t.Fatal(err)
		}
		if exists, err := db.UsernameExists(ctx, "alice"); err != nil || !exists {
			t.Fatalf("UsernameExists = %v, %v", exists, err)
		}
		if err := db.UpdateAccount(ctx, 42, "alice2", "hunter3"); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdatePassword(ctx, 43, "nope"); err == nil {
			t.Fatal("UpdatePassword on missing user succeeded")
		}
		if err := db.SetUsername(ctx, 42, "alice3"); err != nil {
			t.Fatal(err)
		}
	})
}
