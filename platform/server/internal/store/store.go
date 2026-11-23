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
	if err := MigrateProjectWorkspace(db); err != nil {
		return nil, err
	}
	if err := MigrateRequirementPoints(db); err != nil {
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
		"ALTER TABLE tenant_settings ADD COLUMN playwright_browser TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE cases ADD COLUMN dataset_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE cases ADD COLUMN data_row_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE cases ADD COLUMN review_feedback TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE cases ADD COLUMN review_feedback_version INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE bugs ADD COLUMN repro_steps TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE bugs ADD COLUMN expected_result TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE bugs ADD COLUMN actual_result TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE bugs ADD COLUMN assignee TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE bugs ADD COLUMN discovered_version_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE bugs ADD COLUMN fixed_version_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE runs ADD COLUMN dataset_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE runs ADD COLUMN data_row_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE runs ADD COLUMN suite_run_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE runs ADD COLUMN performance_json TEXT NOT NULL DEFAULT 'null'",
		"ALTER TABLE suites ADD COLUMN case_ids TEXT NOT NULL DEFAULT '[]'",
		"ALTER TABLE suites ADD COLUMN project_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE suites ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 1 CHECK (max_concurrency > 0)",
		"ALTER TABLE suite_runs ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 1 CHECK (max_concurrency > 0)",
		"ALTER TABLE test_data_sets ADD COLUMN generation_source TEXT NOT NULL DEFAULT 'rule' CHECK (generation_source IN ('rule', 'ai'))",
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}
