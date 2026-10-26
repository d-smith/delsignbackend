package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"users", "wallets", "addresses"} {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not created: %v", table, err)
		}
	}

	var colType string
	err = db.QueryRow("SELECT type FROM pragma_table_info('addresses') WHERE name='private_key'").Scan(&colType)
	if err != nil || !strings.EqualFold(colType, "text") {
		t.Errorf("addresses.private_key type = %q (err %v), want text", colType, err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, err := first.Exec("INSERT INTO users(email, pubkey) VALUES('a@example.com', 'k')"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	first.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer second.Close()

	var count int
	if err := second.QueryRow("SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Errorf("users count = %d (err %v), want 1: reopening must keep existing data", count, err)
	}
}
