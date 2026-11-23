package store

import (
	"context"
	"database/sql"
	"strings"
)

type ProjectWorkspace struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	Name         string         `json:"name"`
	BaseURL      string         `json:"base_url"`
	AllowedHosts string         `json:"allowed_hosts"`
	Code         string         `json:"code"`
	Lead         string         `json:"lead"`
	Description  string         `json:"description"`
	Lifecycle    string         `json:"lifecycle"`
	CreatedAt    string         `json:"created_at"`
	Metrics      ProjectMetrics `json:"metrics"`
}

type ProjectMetrics struct {
	Requirements  int `json:"requirements"`
	Cases         int `json:"cases"`
	ApprovedCases int `json:"approved_cases"`
	Versions      int `json:"versions"`
	Environments  int `json:"environments"`
	Runs          int `json:"runs"`
	PassedRuns    int `json:"passed_runs"`
	OpenBugs      int `json:"open_bugs"`
}

type ProjectEdit struct {
	Name         *string `json:"name"`
	BaseURL      *string `json:"base_url"`
	AllowedHosts *string `json:"allowed_hosts"`
	Code         *string `json:"code"`
	Lead         *string `json:"lead"`
	Description  *string `json:"description"`
	Lifecycle    *string `json:"lifecycle"`
}

type EnvironmentEdit struct {
	Name      *string `json:"name"`
	BaseURL   *string `json:"base_url"`
	Source    *string `json:"source"`
	Level     *string `json:"level"`
	VersionID *string `json:"version_id"`
}

func MigrateProjectWorkspace(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		"ALTER TABLE projects ADD COLUMN code TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE projects ADD COLUMN lead TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE projects ADD COLUMN lifecycle TEXT NOT NULL DEFAULT 'active'",
		"ALTER TABLE environments ADD COLUMN level TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE environments ADD COLUMN version_id TEXT NOT NULL DEFAULT ''",
	} {
		if _, err := tx.Exec(statement); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetProjectWorkspace(ctx context.Context, tenantID, projectID string) (ProjectWorkspace, error) {
	var project ProjectWorkspace
	metrics := &project.Metrics
	err := s.DB.QueryRowContext(ctx, `WITH
		project_cases AS (SELECT id, status FROM cases WHERE tenant_id = ?1 AND project_id = ?2),
		project_runs AS (SELECT passed FROM runs WHERE tenant_id = ?1 AND case_id IN (SELECT id FROM project_cases))
		SELECT id, tenant_id, name, base_url, allowed_hosts, created_at, code, lead, description, lifecycle,
			(SELECT COUNT(*) FROM requirements WHERE tenant_id = ?1 AND project_id = ?2),
			(SELECT COUNT(*) FROM project_cases),
			(SELECT COUNT(*) FROM project_cases WHERE status = 'approved'),
			(SELECT COUNT(*) FROM versions WHERE tenant_id = ?1 AND project_id = ?2),
			(SELECT COUNT(*) FROM environments WHERE tenant_id = ?1 AND project_id = ?2),
			(SELECT COUNT(*) FROM project_runs),
			(SELECT COUNT(*) FROM project_runs WHERE passed = 1),
			(SELECT COUNT(*) FROM bugs WHERE tenant_id = ?1 AND project_id = ?2 AND status NOT IN ('closed', 'rejected'))
		FROM projects WHERE tenant_id = ?1 AND id = ?2`, tenantID, projectID).Scan(
		&project.ID, &project.TenantID, &project.Name, &project.BaseURL, &project.AllowedHosts, &project.CreatedAt,
		&project.Code, &project.Lead, &project.Description, &project.Lifecycle,
		&metrics.Requirements, &metrics.Cases, &metrics.ApprovedCases, &metrics.Versions, &metrics.Environments,
		&metrics.Runs, &metrics.PassedRuns, &metrics.OpenBugs,
	)
	return project, err
}

func (s *Store) EditProject(ctx context.Context, tenantID, projectID string, edit ProjectEdit) error {
	return s.DB.QueryRowContext(ctx, `UPDATE projects
		SET name = COALESCE(?, name), base_url = COALESCE(?, base_url), allowed_hosts = COALESCE(?, allowed_hosts),
			code = COALESCE(?, code), lead = COALESCE(?, lead), description = COALESCE(?, description), lifecycle = COALESCE(?, lifecycle)
		WHERE tenant_id = ? AND id = ? RETURNING id`, edit.Name, edit.BaseURL, edit.AllowedHosts,
		edit.Code, edit.Lead, edit.Description, edit.Lifecycle, tenantID, projectID).Scan(&projectID)
}

func (s *Store) CreateProject(ctx context.Context, tenantID, projectID string, edit ProjectEdit) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO projects (id, tenant_id, name, base_url, allowed_hosts, code, lead, description, lifecycle)
		VALUES (?, ?, ?, ?, COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, 'active'))`,
		projectID, tenantID, edit.Name, edit.BaseURL, edit.AllowedHosts, edit.Code, edit.Lead, edit.Description, edit.Lifecycle)
	return err
}

func (s *Store) CreateProjectEnvironment(ctx context.Context, tenantID, projectID, environmentID string, edit EnvironmentEdit) error {
	return s.DB.QueryRowContext(ctx, `INSERT INTO environments (id, tenant_id, project_id, name, base_url, source, level, version_id)
		SELECT ?1, p.tenant_id, p.id, ?2, ?3, COALESCE(?4, 'platform'), COALESCE(?5, ''), COALESCE(?6, '')
		FROM projects p WHERE p.tenant_id = ?7 AND p.id = ?8
		AND (COALESCE(?6, '') = '' OR EXISTS (
			SELECT 1 FROM versions v WHERE v.tenant_id = p.tenant_id AND v.project_id = p.id AND v.id = ?6
		)) RETURNING id`, environmentID, edit.Name, edit.BaseURL, edit.Source, edit.Level, edit.VersionID, tenantID, projectID).Scan(&environmentID)
}

func (s *Store) EditEnvironment(ctx context.Context, tenantID, environmentID string, edit EnvironmentEdit) error {
	return s.DB.QueryRowContext(ctx, `UPDATE environments
		SET name = COALESCE(?1, name), base_url = COALESCE(?2, base_url), source = COALESCE(?3, source),
			level = COALESCE(?4, level), version_id = COALESCE(?5, version_id)
		WHERE tenant_id = ?6 AND id = ?7 AND (COALESCE(?5, '') = '' OR EXISTS (
			SELECT 1 FROM versions v WHERE v.tenant_id = environments.tenant_id
				AND v.project_id = environments.project_id AND v.id = ?5
		)) RETURNING id`, edit.Name, edit.BaseURL, edit.Source, edit.Level, edit.VersionID, tenantID, environmentID).Scan(&environmentID)
}
