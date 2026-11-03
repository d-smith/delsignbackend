package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"

	"github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

// Open opens the SQLite database at path and applies the schema. The returned
// handle is shared by all stores; the caller closes it on shutdown.
func Open(path string) (*sql.DB, error) {
	v, _, _ := sqlite3.Version()
	log.Println("Opening sqlite with driver version", v)

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema to %s: %w", path, err)
	}

	return db, nil
}
