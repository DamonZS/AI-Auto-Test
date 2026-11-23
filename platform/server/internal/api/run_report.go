package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"regexp"
	"strings"
)

type runReportStep struct {
	Number     int    `json:"step"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	DurationMS int    `json:"duration_ms"`
}

type runReportPerformance struct {
	Requests  *int     `json:"requests"`
	Failures  *int     `json:"failed_requests"`
	ErrorRate *float64 `json:"error_rate"`
	RPS       *float64 `json:"rps"`
	P50       *float64 `json:"p50_ms"`
	P95       *float64 `json:"p95_ms"`
	P99       *float64 `json:"p99_ms"`
}

type runReportData struct {
	ID          string
	CaseName    string
	Status      string
	StartedAt   string
	EndedAt     string
	DurationMS  int
	NeedsReview bool
	Report      string
	Steps       []runReportStep
	Performance *runReportPerformance
}

func (s *Server) exportRunReport(w http.ResponseWriter, r *http.Request, tenantID string) {
	var report runReportData
	var steps, performance string
	err := s.st.DB.QueryRowContext(r.Context(), `SELECT run.id, COALESCE(test_case.name, ''), run.status,
		run.started_at, COALESCE(run.ended_at, ''), run.duration_ms, run.needs_review, run.report,
		COALESCE(evidence.steps_json, '[]'), run.performance_json
		FROM runs run
		LEFT JOIN cases test_case ON test_case.id = run.case_id AND test_case.tenant_id = run.tenant_id
		LEFT JOIN run_step_evidence evidence ON evidence.run_id = run.id AND evidence.tenant_id = run.tenant_id
		WHERE run.id = ? AND run.tenant_id = ?`, r.PathValue("id"), tenantID).
		Scan(&report.ID, &report.CaseName, &report.Status, &report.StartedAt, &report.EndedAt,
			&report.DurationMS, &report.NeedsReview, &report.Report, &steps, &performance)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "执行记录不存在")
		return
	}
	if err != nil || json.Unmarshal([]byte(steps), &report.Steps) != nil || json.Unmarshal([]byte(performance), &report.Performance) != nil {
		fail(w, http.StatusInternalServerError, "无法读取执行证据")
		return
	}
	report.Report = strings.Join(runReportSummaryLine.FindAllString(report.Report, -1), "\n")
	for index := range report.Steps {
		switch report.Steps[index].Action {
		case "navigate", "click", "fill", "select", "check", "uncheck", "wait_text", "assert_text", "assert_url":
		default:
			report.Steps[index].Action = "未知动作"
		}
	}
	var body bytes.Buffer
	if err := runReportHTML.Execute(&body, report); err != nil {
		fail(w, http.StatusInternalServerError, "无法生成执行报告")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="run-report.html"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body.Bytes())
}

var runReportSummaryLine = regexp.MustCompile(`(?m)^(断言 [0-9]+ \((status_code|json_equals|json_exists|response_time)\): (通过|失败)|请求已执行，但场景没有断言，结果需要人工复核。|结构化 UI 步骤执行 [0-9]+/[0-9]+，断言 [0-9]+ 项。|完成 [0-9]+ 个请求，失败 [0-9]+ 个。)\r?$`)

var runReportHTML = template.Must(template.New("run-report").Funcs(template.FuncMap{
	"percent": func(rate float64) float64 { return rate * 100 },
}).Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>单次执行报告</title>
  <style>
    body { margin: 2rem auto; max-width: 54rem; padding: 0 1rem; font: 16px/1.55 system-ui, sans-serif; overflow-wrap: anywhere; }
    dl { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 0.25rem 1rem; }
    dd { margin: 0; }
    .summary { white-space: pre-wrap; }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 0.25rem 0.6rem; border-bottom: 1px solid #aaa; text-align: start; }
    @media print { body { margin: 0; padding: 0; } h2 { break-after: avoid; } tr { break-inside: avoid; } }
  </style>
</head>
<body>
<main>
  <h1>单次执行报告</h1>
  <dl>
    <dt>执行 ID</dt><dd>{{.ID}}</dd>
    {{with .CaseName}}<dt>用例</dt><dd>{{.}}</dd>{{end}}
    <dt>状态</dt><dd>{{.Status}}</dd>
    <dt>开始时间（UTC）</dt><dd>{{.StartedAt}}</dd>
    <dt>结束时间（UTC）</dt><dd>{{with .EndedAt}}{{.}}{{else}}—{{end}}</dd>
    <dt>耗时</dt><dd>{{.DurationMS}} ms</dd>
    <dt>需复核</dt><dd>{{if .NeedsReview}}是{{else}}否{{end}}</dd>
  </dl>
  <h2>断言报告</h2>
  <p class="summary">{{with .Report}}{{.}}{{else}}暂无断言报告{{end}}</p>
  <h2 id="steps">执行步骤</h2>
  {{if .Steps}}
    <table aria-labelledby="steps">
      <thead><tr><th scope="col">序号</th><th scope="col">动作</th><th scope="col">状态</th><th scope="col">耗时</th></tr></thead>
      <tbody>{{range .Steps}}
        <tr><th scope="row">{{.Number}}</th><td>{{.Action}}</td><td>{{.Status}}</td><td>{{.DurationMS}} ms</td></tr>
      {{end}}</tbody>
    </table>
  {{else}}<p>暂无步骤证据</p>{{end}}
  {{with .Performance}}
    <h2>性能指标</h2>
    <dl>
      <dt>请求数</dt><dd>{{with .Requests}}{{.}}{{else}}—{{end}}</dd>
      <dt>失败数</dt><dd>{{with .Failures}}{{.}}{{else}}—{{end}}</dd>
      <dt>错误率</dt><dd>{{with .ErrorRate}}{{printf "%.2f" (percent .)}}%{{else}}—{{end}}</dd>
      <dt>吞吐量</dt><dd>{{with .RPS}}{{.}} 请求/秒{{else}}—{{end}}</dd>
      <dt>P50</dt><dd>{{with .P50}}{{.}} ms{{else}}—{{end}}</dd>
      <dt>P95</dt><dd>{{with .P95}}{{.}} ms{{else}}—{{end}}</dd>
      <dt>P99</dt><dd>{{with .P99}}{{.}} ms{{else}}—{{end}}</dd>
    </dl>
  {{end}}
</main>
</body>
</html>`))
