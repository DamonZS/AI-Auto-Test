package api

import (
	"context"
	"database/sql"
)

func claimQueuedRun(ctx context.Context, tx *sql.Tx, tenantID string) (string, error) {
	var runID string
	err := tx.QueryRowContext(ctx, `UPDATE runs SET status = 'running', claimed_case_version_no = (
		SELECT version_no FROM cases WHERE id = runs.case_id AND tenant_id = runs.tenant_id
	)
		WHERE rowid = (
			SELECT candidate.rowid FROM runs candidate
			LEFT JOIN suite_runs batch ON batch.id = candidate.suite_run_id AND batch.tenant_id = candidate.tenant_id
			WHERE candidate.tenant_id = ? AND candidate.status = 'queued'
			AND (candidate.suite_run_id = '' OR (
				SELECT COUNT(*) FROM runs active WHERE active.tenant_id = candidate.tenant_id
				AND active.suite_run_id = candidate.suite_run_id AND active.status = 'running'
			) < COALESCE(batch.max_concurrency, 1))
			ORDER BY candidate.started_at, candidate.rowid LIMIT 1
		) RETURNING id`, tenantID).Scan(&runID)
	return runID, err
}
