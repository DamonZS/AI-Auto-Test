package store

import (
	"context"
	"database/sql"
)

const (
	CoverageGo        = "go-coverprofile"
	CoverageCobertura = "cobertura"
)

type CoverageCount struct {
	Covered int64 `json:"covered"`
	Total   int64 `json:"total"`
}

type CoverageSnapshot struct {
	ProjectID  string         `json:"project_id"`
	VersionID  string         `json:"version_id"`
	Format     string         `json:"format"`
	Statements *CoverageCount `json:"statements,omitempty"`
	Lines      *CoverageCount `json:"lines,omitempty"`
	Branches   *CoverageCount `json:"branches,omitempty"`
}

func MigrateCoverage(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS version_coverage (
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		project_id TEXT NOT NULL REFERENCES projects(id),
		version_id TEXT NOT NULL REFERENCES versions(id),
		format TEXT NOT NULL,
		covered INTEGER NOT NULL,
		total INTEGER NOT NULL,
		branches_covered INTEGER NOT NULL,
		branches_total INTEGER NOT NULL,
		PRIMARY KEY (tenant_id, project_id, version_id, format)
	)`)
	return err
}

func (s *Store) SaveVersionCoverage(ctx context.Context, tenantID, versionID, format string, units, branches CoverageCount) (CoverageSnapshot, error) {
	var projectID string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO version_coverage
		(tenant_id, project_id, version_id, format, covered, total, branches_covered, branches_total)
		SELECT p.tenant_id, p.id, v.id, ?, ?, ?, ?, ? FROM versions v
		JOIN projects p ON p.tenant_id = v.tenant_id AND p.id = v.project_id
		WHERE v.tenant_id = ? AND v.id = ?
		ON CONFLICT (tenant_id, project_id, version_id, format) DO UPDATE SET
		covered = excluded.covered, total = excluded.total,
		branches_covered = excluded.branches_covered, branches_total = excluded.branches_total
		RETURNING project_id`, format, units.Covered, units.Total, branches.Covered, branches.Total,
		tenantID, versionID).Scan(&projectID)
	if err != nil {
		return CoverageSnapshot{}, err
	}
	return coverageSnapshot(projectID, versionID, format, units, branches), nil
}

func ListVersionCoverage(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, tenantID, versionID string) ([]CoverageSnapshot, error) {
	rows, err := db.QueryContext(ctx, `SELECT p.id, COALESCE(c.format, ''), COALESCE(c.covered, 0),
		COALESCE(c.total, 0), COALESCE(c.branches_covered, 0), COALESCE(c.branches_total, 0)
		FROM versions v JOIN projects p ON p.tenant_id = v.tenant_id AND p.id = v.project_id
		LEFT JOIN version_coverage c ON c.tenant_id = p.tenant_id AND c.project_id = p.id AND c.version_id = v.id
		WHERE v.tenant_id = ? AND v.id = ? ORDER BY c.format`, tenantID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	snapshots := []CoverageSnapshot{}
	for {
		var projectID, format string
		var units, branches CoverageCount
		if err := rows.Scan(&projectID, &format, &units.Covered, &units.Total, &branches.Covered, &branches.Total); err != nil {
			return nil, err
		}
		if format != "" {
			snapshots = append(snapshots, coverageSnapshot(projectID, versionID, format, units, branches))
		}
		if !rows.Next() {
			return snapshots, rows.Err()
		}
	}
}

func coverageSnapshot(projectID, versionID, format string, units, branches CoverageCount) CoverageSnapshot {
	snapshot := CoverageSnapshot{ProjectID: projectID, VersionID: versionID, Format: format}
	if format == CoverageGo {
		snapshot.Statements = &units
	} else {
		snapshot.Lines, snapshot.Branches = &units, &branches
	}
	return snapshot
}
