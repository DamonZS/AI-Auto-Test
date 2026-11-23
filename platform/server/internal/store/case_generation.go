package store

import "database/sql"

func MigrateCaseGeneration(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS case_generation_sources (
		tenant_id TEXT NOT NULL,
		case_id TEXT NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
		requirement_id TEXT NOT NULL,
		revision_no INTEGER NOT NULL CHECK (revision_no > 0),
		version_id TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (tenant_id, case_id),
		FOREIGN KEY (tenant_id, requirement_id, revision_no)
			REFERENCES requirement_revisions (tenant_id, requirement_id, revision_no)
	)`)
	return err
}
