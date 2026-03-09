package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Database manages SQLite persistence with separate writer/reader connections.
// Writer is limited to 1 connection for serialized writes; reader allows
// concurrent reads via WAL mode.
type Database struct {
	writer *sql.DB
	reader *sql.DB
	path   string
}

// Open creates or opens a SQLite database at path with WAL mode enabled.
func Open(path string) (*Database, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	writer, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	if _, err := writer.Exec("PRAGMA journal_mode=WAL"); err != nil {
		writer.Close()
		return nil, fmt.Errorf("enabling WAL: %w", err)
	}

	if _, err := writer.Exec(schema); err != nil {
		writer.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	reader, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("opening reader: %w", err)
	}
	reader.SetMaxOpenConns(4)

	return &Database{writer: writer, reader: reader, path: path}, nil
}

// OpenMemory opens an in-memory database for testing.
// Uses a single connection for both reads and writes since
// in-memory databases are per-connection.
func OpenMemory() (*Database, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Database{writer: db, reader: db, path: ":memory:"}, nil
}

// Close closes both reader and writer connections.
func (d *Database) Close() error {
	if d.reader != d.writer {
		d.reader.Close()
	}
	return d.writer.Close()
}
