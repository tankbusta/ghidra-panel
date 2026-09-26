package database

import (
	"context"
	"testing"
)

func TestGetOrCreateOIDCUserID(t *testing.T) {
	forEachBackend(t, testGetOrCreateOIDCUserID)
}

func testGetOrCreateOIDCUserID(t *testing.T, db *DB) {
	ctx := context.Background()

	a, err := db.GetOrCreateOIDCUserID(ctx, "https://idp", "alice")
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.GetOrCreateOIDCUserID(ctx, "https://idp", "bob")
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.GetOrCreateOIDCUserID(ctx, "https://idp", "alice")
	if err != nil {
		t.Fatal(err)
	}

	if a&OIDCUserIDBase == 0 || b&OIDCUserIDBase == 0 {
		t.Fatalf("IDs not tagged: %d %d", a, b)
	}
	if a == b {
		t.Fatal("distinct subjects share an ID")
	}
	if a != again {
		t.Fatalf("ID not stable: %d != %d", a, again)
	}

	// Tagged IDs must round-trip through the passwords table
	if err := db.CreateAccount(ctx, a, "alice", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdatePassword(ctx, a, "hunter3"); err != nil {
		t.Fatal(err)
	}
}
