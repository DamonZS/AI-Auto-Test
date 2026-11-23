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
	if err := MigrateWorkbench(db); err != nil {
		return nil, err
	}
	if err := MigrateCoverage(db); err != nil {
		return nil, err
	}
	if err := MigrateRequirements(db); err != nil {
		return nil, err
	}
	if err := MigrateCaseGeneration(db); err != nil {
		return nil, err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_runs_batch_status ON runs (tenant_id, suite_run_id, status)"); err != nil {
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
		"ALTER TABLE suites ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 1 CHECK (max_concurrency > 0)",
		"ALTER TABLE suite_runs ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 1 CHECK (max_concurrency > 0)",
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}
