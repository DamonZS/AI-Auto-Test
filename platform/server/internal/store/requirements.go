package store

import (
	"context"
	"database/sql"
	"strings"
)

type RequirementContent struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type RequirementRevision struct {
	RequirementContent
	RevisionNo int    `json:"revision_no"`
	CreatedAt  string `json:"created_at"`
}

type RequirementChange struct {
	RequirementID     string               `json:"requirement_id"`
	Previous          *RequirementRevision `json:"previous"`
	Current           RequirementRevision  `json:"current"`
	AffectedCaseCount int                  `json:"affected_case_count"`
}

type RequirementImpact struct {
	AffectedCaseCount int                 `json:"affected_case_count"`
	Changes           []RequirementChange `json:"changes"`
}

const requirementProjectMismatch = "requirement project not in tenant"

func MigrateRequirements(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS requirement_revisions (
			tenant_id TEXT NOT NULL,
			requirement_id TEXT NOT NULL REFERENCES requirements(id),
			revision_no INTEGER NOT NULL,
			code TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (tenant_id, requirement_id, revision_no)
		);
		CREATE TABLE IF NOT EXISTS requirement_version_links (
			tenant_id TEXT NOT NULL,
			version_id TEXT NOT NULL REFERENCES versions(id),
			requirement_id TEXT NOT NULL,
			revision_no INTEGER NOT NULL,
			PRIMARY KEY (tenant_id, version_id, requirement_id, revision_no),
			FOREIGN KEY (tenant_id, requirement_id, revision_no)
				REFERENCES requirement_revisions(tenant_id, requirement_id, revision_no)
		);
		CREATE TRIGGER IF NOT EXISTS requirements_initial_revision AFTER INSERT ON requirements
		BEGIN
			SELECT RAISE(ABORT, '` + requirementProjectMismatch + `') WHERE NOT EXISTS (
				SELECT 1 FROM projects WHERE tenant_id = NEW.tenant_id AND id = NEW.project_id
			);
			INSERT INTO requirement_revisions (tenant_id, requirement_id, revision_no, code, title, content, created_at)
			VALUES (NEW.tenant_id, NEW.id, 1, NEW.code, NEW.title, NEW.content, NEW.created_at);
		END;
		INSERT OR IGNORE INTO requirement_revisions (tenant_id, requirement_id, revision_no, code, title, content, created_at)
		SELECT q.tenant_id, q.id, 1, q.code, q.title, q.content, q.created_at
		FROM requirements q JOIN projects p ON p.id = q.project_id AND p.tenant_id = q.tenant_id;
	`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ImportRequirements(ctx context.Context, tenantID, projectID string, ids []string, items []RequirementContent) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index, item := range items {
		_, err := tx.ExecContext(ctx, `INSERT INTO requirements (id, tenant_id, project_id, code, title, content)
			VALUES (?, ?, ?, ?, ?, ?)`, ids[index], tenantID, projectID, item.Code, item.Title, item.Content)
		if err != nil {
			if strings.Contains(err.Error(), requirementProjectMismatch) {
				return sql.ErrNoRows
			}
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ReviseRequirement(ctx context.Context, tenantID, requirementID string, code, title, content *string) (RequirementRevision, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	var revision RequirementRevision
	if err != nil {
		return revision, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `UPDATE requirements
		SET code = COALESCE(?, code), title = COALESCE(?, title), content = COALESCE(?, content)
		WHERE tenant_id = ? AND id = ? AND EXISTS (
			SELECT 1 FROM projects p WHERE p.id = requirements.project_id AND p.tenant_id = requirements.tenant_id
		) RETURNING code, title, content`, code, title, content, tenantID, requirementID).
		Scan(&revision.Code, &revision.Title, &revision.Content)
	if err != nil {
		return revision, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO requirement_revisions (tenant_id, requirement_id, revision_no, code, title, content)
		SELECT ?, ?, MAX(revision_no) + 1, ?, ?, ? FROM requirement_revisions WHERE tenant_id = ? AND requirement_id = ?
		RETURNING revision_no, created_at`, tenantID, requirementID, revision.Code, revision.Title, revision.Content, tenantID, requirementID).
		Scan(&revision.RevisionNo, &revision.CreatedAt)
	if err != nil {
		return revision, err
	}
	return revision, tx.Commit()
}

func (s *Store) RequirementHistory(ctx context.Context, tenantID, requirementID string) ([]RequirementRevision, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.revision_no, r.code, r.title, r.content, r.created_at
		FROM requirement_revisions r
		JOIN requirements q ON q.id = r.requirement_id AND q.tenant_id = r.tenant_id
		JOIN projects p ON p.id = q.project_id AND p.tenant_id = q.tenant_id
		WHERE r.tenant_id = ? AND r.requirement_id = ? ORDER BY r.revision_no`, tenantID, requirementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []RequirementRevision
	for rows.Next() {
		var revision RequirementRevision
		if err := rows.Scan(&revision.RevisionNo, &revision.Code, &revision.Title, &revision.Content, &revision.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, sql.ErrNoRows
	}
	return history, nil
}

func (s *Store) LinkRequirementVersion(ctx context.Context, tenantID, requirementID string, revisionNo int, versionID string) error {
	return s.DB.QueryRowContext(ctx, `INSERT INTO requirement_version_links (tenant_id, version_id, requirement_id, revision_no)
		SELECT r.tenant_id, v.id, r.requirement_id, r.revision_no FROM requirement_revisions r
		JOIN requirements q ON q.id = r.requirement_id AND q.tenant_id = r.tenant_id
		JOIN projects p ON p.id = q.project_id AND p.tenant_id = q.tenant_id
		JOIN versions v ON v.project_id = p.id AND v.tenant_id = p.tenant_id
		WHERE r.tenant_id = ? AND r.requirement_id = ? AND r.revision_no = ? AND v.id = ?
		ON CONFLICT (tenant_id, version_id, requirement_id, revision_no) DO UPDATE SET version_id = excluded.version_id
		RETURNING version_id`, tenantID, requirementID, revisionNo, versionID).Scan(&versionID)
}

func (s *Store) VersionRequirementImpact(ctx context.Context, tenantID, versionID string) (RequirementImpact, error) {
	report := RequirementImpact{Changes: []RequirementChange{}}
	var projectID string
	err := s.DB.QueryRowContext(ctx, `SELECT p.id FROM versions v
		JOIN projects p ON p.id = v.project_id AND p.tenant_id = v.tenant_id
		WHERE v.tenant_id = ? AND v.id = ?`, tenantID, versionID).Scan(&projectID)
	if err != nil {
		return report, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT q.id, curr.revision_no, curr.code, curr.title, curr.content, curr.created_at,
		COALESCE(prev.code, ''), COALESCE(prev.title, ''), COALESCE(prev.content, ''), COALESCE(prev.created_at, ''),
		(SELECT COUNT(*) FROM cases c WHERE c.tenant_id = q.tenant_id AND c.project_id = q.project_id AND c.requirement_id = q.id)
		FROM requirement_version_links link
		JOIN requirements q ON q.id = link.requirement_id AND q.tenant_id = link.tenant_id
		JOIN requirement_revisions curr ON curr.tenant_id = link.tenant_id AND curr.requirement_id = link.requirement_id AND curr.revision_no = link.revision_no
		LEFT JOIN requirement_revisions prev ON prev.tenant_id = curr.tenant_id AND prev.requirement_id = curr.requirement_id AND prev.revision_no = curr.revision_no - 1
		WHERE link.tenant_id = ? AND link.version_id = ? AND q.project_id = ?
		ORDER BY q.id, curr.revision_no`, tenantID, versionID, projectID)
	if err != nil {
		return report, err
	}
	defer rows.Close()
	lastRequirement := ""
	for rows.Next() {
		var change RequirementChange
		var previous RequirementRevision
		current := &change.Current
		if err := rows.Scan(&change.RequirementID, &current.RevisionNo, &current.Code, &current.Title, &current.Content, &current.CreatedAt,
			&previous.Code, &previous.Title, &previous.Content, &previous.CreatedAt, &change.AffectedCaseCount); err != nil {
			return report, err
		}
		if current.RevisionNo > 1 {
			previous.RevisionNo = current.RevisionNo - 1
			change.Previous = &previous
		}
		if lastRequirement != change.RequirementID {
			report.AffectedCaseCount += change.AffectedCaseCount
			lastRequirement = change.RequirementID
		}
		report.Changes = append(report.Changes, change)
	}
	return report, rows.Err()
}
