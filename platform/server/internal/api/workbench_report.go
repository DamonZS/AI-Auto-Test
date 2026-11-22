package api

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"platform/server/internal/store"
)

type workbenchCaseStatistics struct {
	Total    int `json:"total"`
	Executed int `json:"executed"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	Blocked  int `json:"blocked"`
}

type workbenchCoverage struct {
	Covered int `json:"covered"`
	Total   int `json:"total"`
}

type workbenchDefect struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

type workbenchMetrics struct {
	CaseStatistics       workbenchCaseStatistics  `json:"case_statistics"`
	RequirementCoverage  workbenchCoverage        `json:"requirement_coverage"`
	WhiteboxCoverage     []store.CoverageSnapshot `json:"whitebox_coverage,omitempty"`
	ExecutionRate        float64                  `json:"execution_rate"`
	PassRate             float64                  `json:"pass_rate"`
	DefectTotal          int                      `json:"defect_total"`
	DefectDistribution   map[string]int           `json:"defect_distribution"`
	SeverityDistribution map[string]int           `json:"severity_distribution"`
	ResidualDefects      []workbenchDefect        `json:"residual_defects"`
	StartedAt            string                   `json:"started_at"`
	EndedAt              string                   `json:"ended_at"`
	RequiresHumanReview  bool                     `json:"requires_human_review"`
}

func (s *Server) collectWorkbenchMetrics(ctx context.Context, tenantID string, doc workbenchDocument) (*workbenchMetrics, error) {
	metrics := &workbenchMetrics{
		DefectDistribution:   map[string]int{},
		SeverityDistribution: map[string]int{},
		ResidualDefects:      []workbenchDefect{},
	}
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	stats := &metrics.CaseStatistics
	err = tx.QueryRowContext(ctx, `WITH eligible_cases AS (
		SELECT id FROM cases WHERE tenant_id = ? AND project_id = ? AND status = 'approved'
	), finished AS (
		SELECT r.status, r.started_at, r.ended_at,
			ROW_NUMBER() OVER (PARTITION BY r.case_id ORDER BY r.ended_at DESC, r.rowid DESC) AS rank
		FROM runs r JOIN eligible_cases c ON c.id = r.case_id
		WHERE r.tenant_id = ? AND r.version_id = ? AND r.ended_at IS NOT NULL AND r.status NOT IN ('queued', 'running')
	)
	SELECT (SELECT COUNT(*) FROM eligible_cases), COUNT(*) FILTER (WHERE rank = 1),
		COUNT(*) FILTER (WHERE rank = 1 AND status = 'passed'),
		COUNT(*) FILTER (WHERE rank = 1 AND status = 'failed'),
		COALESCE(MIN(started_at), ''), COALESCE(MAX(ended_at), '') FROM finished`,
		tenantID, doc.ProjectID, tenantID, doc.VersionID).Scan(&stats.Total, &stats.Executed, &stats.Passed,
		&stats.Failed, &metrics.StartedAt, &metrics.EndedAt)
	if err != nil {
		return nil, err
	}
	stats.Blocked = stats.Executed - stats.Passed - stats.Failed
	if stats.Total > 0 {
		metrics.ExecutionRate = float64(stats.Executed) / float64(stats.Total)
	}
	if stats.Executed > 0 {
		metrics.PassRate = float64(stats.Passed) / float64(stats.Executed)
	}
	coverage := &metrics.RequirementCoverage
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE EXISTS (
		SELECT 1 FROM cases c WHERE c.tenant_id = q.tenant_id AND c.project_id = q.project_id
		AND c.requirement_id = q.id AND c.status = 'approved'
	)) FROM requirements q WHERE q.tenant_id = ? AND q.project_id = ?`, tenantID, doc.ProjectID).
		Scan(&coverage.Total, &coverage.Covered)
	if err != nil {
		return nil, err
	}
	metrics.WhiteboxCoverage, err = store.ListVersionCoverage(ctx, tx, tenantID, doc.VersionID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT b.bug_code, b.title, b.severity, b.priority, b.status
		FROM bugs b JOIN runs r ON r.id = b.run_id AND r.tenant_id = b.tenant_id
		JOIN cases c ON c.id = r.case_id AND c.tenant_id = r.tenant_id AND c.project_id = b.project_id
		WHERE b.tenant_id = ? AND b.project_id = ? AND r.version_id = ? ORDER BY b.bug_code`,
		tenantID, doc.ProjectID, doc.VersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var defect workbenchDefect
		if err := rows.Scan(&defect.Code, &defect.Title, &defect.Severity, &defect.Priority, &defect.Status); err != nil {
			return nil, err
		}
		defect.Priority = strings.ToUpper(strings.TrimSpace(defect.Priority))
		defect.Severity = strings.ToUpper(strings.TrimSpace(defect.Severity))
		metrics.DefectTotal++
		metrics.DefectDistribution[defect.Priority]++
		metrics.SeverityDistribution[defect.Severity]++
		if defect.Status != "closed" && defect.Status != "rejected" {
			metrics.ResidualDefects = append(metrics.ResidualDefects, defect)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	metrics.RequiresHumanReview = doc.RiskLevel == "P0" || doc.RiskLevel == "P1" || metrics.DefectDistribution["P0"] > 0 ||
		metrics.DefectDistribution["P1"] > 0 || strings.Contains(doc.Fields["conclusion"], "上线")
	return metrics, tx.Commit()
}

var workbenchExport = template.Must(template.New("workbench").Funcs(template.FuncMap{
	"percent": func(rate float64) string { return fmt.Sprintf("%.1f%%", rate*100) },
}).Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>{{.Doc.Title}}</title>
<style>body{max-width:54rem;margin:3rem auto;padding:0 1rem;font:16px/1.55 system-ui,sans-serif}dt,th{font-weight:600}dd{margin:0 0 .75rem;white-space:pre-wrap;overflow-wrap:anywhere}table{border-collapse:collapse}td,th{padding:.25rem .6rem;border:1px solid #ccc;text-align:left}</style>
</head><body>
<h1>{{.Doc.Title}}</h1>
<p>{{.Template.Label}} · {{.ProjectName}} · {{.VersionName}} · 修订 {{.Doc.Revision}}</p>
{{if .Doc.RiskLevel}}<p>风险级别 {{.Doc.RiskLevel}}</p>{{end}}
<dl>{{range .Template.Fields}}<dt>{{.Label}}</dt><dd>{{index $.Doc.Fields .Key}}</dd>{{end}}
{{range $item := .Template.Checks}}{{with index $.Doc.Checks $item.Key}}<dt>{{$item.Label}}</dt><dd>通过 — {{.Evidence}}</dd>{{end}}{{end}}</dl>
{{with .Doc.Metrics}}
<h2>执行统计</h2>
<dl><dt>执行时间（UTC）</dt><dd>{{if .StartedAt}}{{.StartedAt}} 至 {{.EndedAt}}{{else}}无已结束执行{{end}}</dd>
<dt>用例</dt><dd>有效 {{.CaseStatistics.Total}} · 执行 {{.CaseStatistics.Executed}} · 通过 {{.CaseStatistics.Passed}} · 失败 {{.CaseStatistics.Failed}} · 阻塞 {{.CaseStatistics.Blocked}}</dd>
<dt>需求覆盖</dt><dd>{{.RequirementCoverage.Covered}} / {{.RequirementCoverage.Total}}</dd>
{{range .WhiteboxCoverage}}<dt>白盒覆盖率 · {{.Format}}</dt><dd>{{with .Statements}}语句 {{.Covered}} / {{.Total}}{{end}}{{with .Lines}}行 {{.Covered}} / {{.Total}}{{end}}{{with .Branches}} · 分支 {{.Covered}} / {{.Total}}{{end}}</dd>{{end}}
<dt>执行率 / 通过率</dt><dd>{{percent .ExecutionRate}} / {{percent .PassRate}}</dd>
<dt>缺陷总数</dt><dd>{{.DefectTotal}}</dd>
<dt>缺陷优先级</dt><dd>{{range $level, $count := .DefectDistribution}}{{$level}} {{$count}}　{{else}}无{{end}}</dd>
<dt>缺陷严重度</dt><dd>{{range $level, $count := .SeverityDistribution}}{{$level}} {{$count}}　{{else}}无{{end}}</dd></dl>
<h2>遗留缺陷</h2>
{{if .ResidualDefects}}<table><thead><tr><th>编号</th><th>标题</th><th>优先级</th><th>严重度</th><th>状态</th></tr></thead><tbody>
{{range .ResidualDefects}}<tr><td>{{.Code}}</td><td>{{.Title}}</td><td>{{.Priority}}</td><td>{{.Severity}}</td><td>{{.Status}}</td></tr>{{end}}
</tbody></table>{{else}}<p>无</p>{{end}}
{{end}}
<h2>审核签核</h2>
<dl><dt>结论</dt><dd>已批准</dd><dt>审核人</dt><dd>{{.Doc.Reviewer}}</dd><dt>审核时间</dt><dd>{{.Doc.ReviewedAt}}</dd>
{{if .Doc.ReviewNote}}<dt>意见</dt><dd>{{.Doc.ReviewNote}}</dd>{{end}}</dl></body></html>`))

func (s *Server) exportWorkbenchDocument(w http.ResponseWriter, r *http.Request, tenantID string) {
	doc, ok := s.loadWorkbenchDocument(w, r, tenantID)
	if !ok {
		return
	}
	if doc.Status != "approved" {
		fail(w, http.StatusConflict, "产物批准后方可导出")
		return
	}
	data := struct {
		Doc         workbenchDocument
		Template    *workbenchTemplate
		ProjectName string
		VersionName string
	}{Doc: doc, Template: findWorkbenchTemplate(doc.Kind)}
	err := s.st.DB.QueryRowContext(r.Context(), `SELECT p.name, v.name FROM versions v
		JOIN projects p ON p.id = v.project_id AND p.tenant_id = v.tenant_id
		WHERE v.tenant_id = ? AND p.id = ? AND v.id = ?`, tenantID, doc.ProjectID, doc.VersionID).
		Scan(&data.ProjectName, &data.VersionName)
	var body bytes.Buffer
	if err != nil || workbenchExport.Execute(&body, data) != nil {
		fail(w, http.StatusInternalServerError, "无法导出测试产物")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.html"`, doc.Kind, doc.ID))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body.Bytes())
}
