package store

import (
	"database/sql"
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	DB        *sql.DB
	MasterKey []byte
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	if err := migrateAdditiveColumns(db); err != nil {
		return nil, err
	}
	masterKey, err := loadMasterKey(path + ".key")
	if err != nil {
		return nil, err
	}
	return &Store{DB: db, MasterKey: masterKey}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func migrateAdditiveColumns(db *sql.DB) error {
	statements := []string{
		"ALTER TABLE cases ADD COLUMN dataset_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE cases ADD COLUMN data_row_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE runs ADD COLUMN dataset_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE runs ADD COLUMN data_row_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE runs ADD COLUMN suite_run_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE suites ADD COLUMN case_ids TEXT NOT NULL DEFAULT '[]'",
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}
