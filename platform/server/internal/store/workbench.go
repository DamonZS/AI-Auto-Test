package store

import "database/sql"

func MigrateWorkbench(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS workbench_documents (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		project_id TEXT NOT NULL REFERENCES projects(id),
		version_id TEXT NOT NULL REFERENCES versions(id),
		kind TEXT NOT NULL,
		content TEXT NOT NULL,
		metrics TEXT NOT NULL DEFAULT 'null',
		status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'approved', 'rejected')),
		revision INTEGER NOT NULL DEFAULT 1,
		reviewer TEXT NOT NULL DEFAULT '',
		review_note TEXT NOT NULL DEFAULT '',
		reviewed_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_workbench_documents_tenant_updated
		ON workbench_documents(tenant_id, updated_at DESC, id DESC);`)
	return err
}
