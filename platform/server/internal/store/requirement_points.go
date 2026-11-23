package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type RequirementPoint struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type RequirementPointAnalysis struct {
	ID               string             `json:"id"`
	RequirementID    string             `json:"requirement_id"`
	ProjectID        string             `json:"project_id"`
	RevisionNo       int                `json:"revision_no"`
	RoundNo          int                `json:"round_no"`
	ParentAnalysisID string             `json:"parent_analysis_id,omitempty"`
	Status           string             `json:"status"`
	Points           []RequirementPoint `json:"points,omitempty"`
	Feedback         string             `json:"feedback,omitempty"`
	VersionID        string             `json:"version_id,omitempty"`
	CreatedAt        string             `json:"created_at"`
	SubmittedBy      string             `json:"submitted_by,omitempty"`
	SubmittedAt      string             `json:"submitted_at,omitempty"`
	ReviewedBy       string             `json:"reviewed_by,omitempty"`
	ReviewedAt       string             `json:"reviewed_at,omitempty"`
}

type RequirementPointHistory struct {
	Rounds     []RequirementPointAnalysis `json:"rounds"`
	VersionIDs []string                   `json:"version_ids"`
}

type RequirementPointSource struct {
	Title     string
	Content   string
	LastRound int
}

var ErrRequirementPointConflict = errors.New("requirement point round already exists or changed state")

func MigrateRequirementPoints(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS requirement_point_analyses (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		requirement_id TEXT NOT NULL,
		revision_no INTEGER NOT NULL,
		round_no INTEGER NOT NULL CHECK (round_no > 0),
		parent_analysis_id TEXT REFERENCES requirement_point_analyses(id),
		status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'returned', 'approved')),
		points_json TEXT NOT NULL,
		feedback TEXT NOT NULL DEFAULT '',
		version_id TEXT REFERENCES versions(id),
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		submitted_by TEXT NOT NULL DEFAULT '',
		submitted_at TEXT NOT NULL DEFAULT '',
		reviewed_by TEXT NOT NULL DEFAULT '',
		reviewed_at TEXT NOT NULL DEFAULT '',
		UNIQUE (tenant_id, requirement_id, revision_no, round_no),
		FOREIGN KEY (tenant_id, requirement_id, revision_no)
			REFERENCES requirement_revisions(tenant_id, requirement_id, revision_no)
	)`)
	return err
}

const requirementPointSourceFrom = ` FROM requirement_revisions rev
	JOIN requirements req ON req.id = rev.requirement_id AND req.tenant_id = rev.tenant_id
	JOIN projects proj ON proj.id = req.project_id AND proj.tenant_id = req.tenant_id
	WHERE rev.tenant_id = ? AND rev.requirement_id = ? AND rev.revision_no = ?`

func (s *Store) GetRequirementPointSource(ctx context.Context, tenantID, requirementID string, revisionNo int) (RequirementPointSource, error) {
	var source RequirementPointSource
	err := s.DB.QueryRowContext(ctx, `SELECT rev.title, rev.content,
		COALESCE((SELECT MAX(round_no) FROM requirement_point_analyses
			WHERE tenant_id = rev.tenant_id AND requirement_id = rev.requirement_id AND revision_no = rev.revision_no), 0)`+
		requirementPointSourceFrom, tenantID, requirementID, revisionNo).
		Scan(&source.Title, &source.Content, &source.LastRound)
	return source, err
}

const requirementPointColumns = `id, requirement_id, revision_no, round_no, COALESCE(parent_analysis_id, ''), status,
	feedback, COALESCE(version_id, ''), created_at, submitted_by, submitted_at, reviewed_by, reviewed_at`

func scanRequirementPointAnalysis(row interface{ Scan(...any) error }) (RequirementPointAnalysis, error) {
	var analysis RequirementPointAnalysis
	var points []byte
	err := row.Scan(&analysis.ID, &analysis.RequirementID, &analysis.RevisionNo, &analysis.RoundNo, &analysis.ParentAnalysisID,
		&analysis.Status, &analysis.Feedback, &analysis.VersionID, &analysis.CreatedAt, &analysis.SubmittedBy,
		&analysis.SubmittedAt, &analysis.ReviewedBy, &analysis.ReviewedAt, &analysis.ProjectID, &points)
	if err == nil && points != nil {
		err = json.Unmarshal(points, &analysis.Points)
	}
	return analysis, err
}

const requirementPointProject = `(SELECT req.project_id FROM requirements req
	WHERE req.id = requirement_point_analyses.requirement_id AND req.tenant_id = requirement_point_analyses.tenant_id)`

func (s *Store) GetRequirementPointAnalysis(ctx context.Context, tenantID, requirementID, analysisID string) (RequirementPointAnalysis, error) {
	return scanRequirementPointAnalysis(s.DB.QueryRowContext(ctx, `SELECT `+requirementPointColumns+`, `+requirementPointProject+`, points_json
		FROM requirement_point_analyses WHERE tenant_id = ? AND requirement_id = ? AND id = ?`, tenantID, requirementID, analysisID))
}

func (s *Store) ListRequirementPointAnalyses(ctx context.Context, tenantID, requirementID string, revisionNo int) (RequirementPointHistory, error) {
	history := RequirementPointHistory{Rounds: []RequirementPointAnalysis{}, VersionIDs: []string{}}
	var projectID string
	if err := s.DB.QueryRowContext(ctx, `SELECT proj.id`+requirementPointSourceFrom, tenantID, requirementID, revisionNo).Scan(&projectID); err != nil {
		return history, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+requirementPointColumns+`, ?, NULL
		FROM requirement_point_analyses WHERE tenant_id = ? AND requirement_id = ? AND revision_no = ? ORDER BY round_no DESC`,
		projectID, tenantID, requirementID, revisionNo)
	if err != nil {
		return history, err
	}
	defer rows.Close()
	for rows.Next() {
		analysis, err := scanRequirementPointAnalysis(rows)
		if err != nil {
			return history, err
		}
		history.Rounds = append(history.Rounds, analysis)
	}
	if err := rows.Err(); err != nil {
		return history, err
	}
	versions, err := s.DB.QueryContext(ctx, `SELECT version_id FROM requirement_version_links
		WHERE tenant_id = ? AND requirement_id = ? AND revision_no = ? ORDER BY version_id`, tenantID, requirementID, revisionNo)
	if err != nil {
		return history, err
	}
	defer versions.Close()
	for versions.Next() {
		var versionID string
		if err := versions.Scan(&versionID); err != nil {
			return history, err
		}
		history.VersionIDs = append(history.VersionIDs, versionID)
	}
	return history, versions.Err()
}

func (s *Store) CreateRequirementPointAnalysis(ctx context.Context, tenantID string, draft RequirementPointAnalysis) (RequirementPointAnalysis, error) {
	points, err := json.Marshal(draft.Points)
	if err != nil {
		return draft, err
	}
	draft, err = scanRequirementPointAnalysis(s.DB.QueryRowContext(ctx, `INSERT INTO requirement_point_analyses
		(id, tenant_id, requirement_id, revision_no, round_no, parent_analysis_id, points_json)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?) ON CONFLICT (tenant_id, requirement_id, revision_no, round_no) DO NOTHING
		RETURNING `+requirementPointColumns+`, `+requirementPointProject+`, points_json`,
		draft.ID, tenantID, draft.RequirementID, draft.RevisionNo, draft.RoundNo, draft.ParentAnalysisID, string(points)))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrRequirementPointConflict
	}
	return draft, err
}

func (s *Store) IsRequirementPointVersionLinked(ctx context.Context, tenantID string, analysis RequirementPointAnalysis, versionID string) (bool, error) {
	var linked bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM requirement_version_links link
		JOIN versions ver ON ver.id = link.version_id AND ver.tenant_id = link.tenant_id AND ver.project_id = ?
		WHERE link.tenant_id = ? AND link.requirement_id = ? AND link.revision_no = ? AND link.version_id = ?)`,
		analysis.ProjectID, tenantID, analysis.RequirementID, analysis.RevisionNo, versionID).Scan(&linked)
	return linked, err
}

func (s *Store) UpdateRequirementPointReview(ctx context.Context, tenantID string, analysis RequirementPointAnalysis, nextStatus, actor string) (RequirementPointAnalysis, error) {
	updated, err := scanRequirementPointAnalysis(s.DB.QueryRowContext(ctx, `UPDATE requirement_point_analyses SET
		status = ?, feedback = ?, version_id = NULLIF(?, ''),
		submitted_by = CASE WHEN ? = 'submitted' THEN ? ELSE submitted_by END,
		submitted_at = CASE WHEN ? = 'submitted' THEN datetime('now') ELSE submitted_at END,
		reviewed_by = CASE WHEN ? = 'submitted' THEN reviewed_by ELSE ? END,
		reviewed_at = CASE WHEN ? = 'submitted' THEN reviewed_at ELSE datetime('now') END
		WHERE tenant_id = ? AND requirement_id = ? AND id = ? AND status = ?
		RETURNING `+requirementPointColumns+`, `+requirementPointProject+`, points_json`,
		nextStatus, analysis.Feedback, analysis.VersionID, nextStatus, actor, nextStatus, nextStatus, actor, nextStatus,
		tenantID, analysis.RequirementID, analysis.ID, analysis.Status))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrRequirementPointConflict
	}
	return updated, err
}
