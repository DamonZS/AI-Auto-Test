package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestRunReportExport(t *testing.T) {
	database := openTestStore(t)
	seedMCPData(t, database)
	if _, err := database.DB.Exec(`UPDATE cases SET name = '库存检查 <b>用例</b>', scenario = 'private-scenario' WHERE id = 'case-a';
		UPDATE runs SET status = 'failed', needs_review = 1, duration_ms = 1250, ended_at = '2026-11-23 09:30:00',
			fallback_reason = 'private-provider-detail',
			report = '断言 1 (status_code): 失败' || char(10) || '断言 2 (json_exists): 通过' || char(10) || 'private-freeform-report',
			performance_json = '{"requests":80,"failed_requests":0,"error_rate":0,"rps":64,"p50_ms":4,"p95_ms":12.5,"p99_ms":null,"raw":"private-perf"}'
		WHERE id = 'run-a';
		INSERT INTO run_step_evidence (run_id, tenant_id, steps_json) VALUES
			('run-a', 'tenant-a', '[{"step":1,"action":"navigate","status":"passed","duration_ms":7,"screenshot_jpeg":"private-image"},{"step":2,"action":"assert_text","status":"failed","duration_ms":125,"request":"private-request","response":"private-response"},{"step":3,"action":"private-action","status":"failed","duration_ms":3}]'),
			('run-a', 'tenant-b', '[{"step":1,"action":"foreign-step","status":"failed","duration_ms":2}]');
		INSERT INTO runs (id, tenant_id, case_id, status, report) VALUES
			('old-run', 'tenant-a', 'case-a', 'queued', ''),
			('ui-run', 'tenant-a', 'case-a', 'passed', '结构化 UI 步骤执行 3/3，断言 2 项。');`); err != nil {
		t.Fatal(err)
	}
	if err := ProvisionUser(t.Context(), database, "report-viewer", fixturePassword, "tenant-a", "", "viewer"); err != nil {
		t.Fatal(err)
	}
	handler := New(database, AuthOptions{}).Routes()
	token := authLogin(t, handler, "report-viewer")
	const reportPath = "/api/runs/run-a/report"
	authRequest(t, handler, http.MethodGet, reportPath, "tenant-a", "", "", http.StatusUnauthorized)
	authRequest(t, handler, http.MethodGet, reportPath, "", token, "", http.StatusBadRequest)
	authRequest(t, handler, http.MethodGet, reportPath, "tenant-b", token, "", http.StatusForbidden)
	authRequest(t, handler, http.MethodGet, "/api/runs/run-b/report", "tenant-a", token, "", http.StatusNotFound)
	authRequest(t, handler, http.MethodGet, "/api/runs/missing/report", "tenant-a", token, "", http.StatusNotFound)

	response := authRequest(t, handler, http.MethodGet, reportPath, "tenant-a", token, "", http.StatusOK)
	if response.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		response.Header().Get("Content-Disposition") != `attachment; filename="run-report.html"` ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected download headers: %v", response.Header())
	}
	html := response.Body.String()
	for _, text := range []string{
		"库存检查 &lt;b&gt;用例&lt;/b&gt;", "run-a", "failed", "1250 ms", "2026-11-23 09:30:00",
		"断言 1 (status_code): 失败\n断言 2 (json_exists): 通过", "navigate", "assert_text", "125 ms", "未知动作",
		"<dt>请求数</dt><dd>80</dd>", "<dt>失败数</dt><dd>0</dd>", "0.00%", "64 请求/秒", "12.5 ms", "<dt>P99</dt><dd>—</dd>",
	} {
		if !strings.Contains(html, text) {
			t.Errorf("report is missing %q", text)
		}
	}
	for _, text := range []string{"private-", "foreign-step", "<b>", "<script"} {
		if strings.Contains(html, text) {
			t.Errorf("unexpected content in report: %q", text)
		}
	}
	oldReport := authRequest(t, handler, http.MethodGet, "/api/runs/old-run/report", "tenant-a", token, "", http.StatusOK).Body.String()
	if !strings.Contains(oldReport, "queued") || !strings.Contains(oldReport, "暂无断言报告") ||
		!strings.Contains(oldReport, "暂无步骤证据") || strings.Contains(oldReport, "P95") {
		t.Errorf("unexpected report for a run without evidence: %s", oldReport)
	}
	uiReport := authRequest(t, handler, http.MethodGet, "/api/runs/ui-run/report", "tenant-a", token, "", http.StatusOK).Body.String()
	if !strings.Contains(uiReport, "结构化 UI 步骤执行 3/3，断言 2 项。") {
		t.Error("report is missing the persisted UI assertion summary")
	}
}
