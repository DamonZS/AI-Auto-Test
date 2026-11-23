package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTrendsCalendarMonthComparisons(t *testing.T) {
	database := openTestStore(t)
	if _, err := database.DB.Exec(`INSERT INTO tenants (id, name) VALUES ('tenant-a', 'A'), ('tenant-b', 'B'), ('empty', 'Empty')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	current := month.AddDate(0, -1, 0)
	previous := month.AddDate(0, -2, 0)
	lastYear := current.AddDate(-1, 0, 0)
	stamp := func(value time.Time) string { return value.Format(time.DateTime) }
	if _, err := database.DB.Exec(`INSERT INTO runs (id, tenant_id, case_id, status, passed, needs_review, started_at, ended_at) VALUES
		('run-current-pass', 'tenant-a', 'case-a', 'passed', 1, 0, ?, ?),
		('run-current-fail', 'tenant-a', 'case-a', 'failed', 0, 0, ?, ?),
		('run-current-review', 'tenant-a', 'case-a', 'failed', 0, 1, ?, ?),
		('run-current-skip', 'tenant-a', 'case-a', 'skipped', 0, 0, ?, ?),
		('run-current-error', 'tenant-a', 'case-a', 'error', 0, 0, ?, ?),
		('run-current-running', 'tenant-a', 'case-a', 'running', NULL, 0, ?, NULL),
		('run-open-month', 'tenant-a', 'case-a', 'passed', 1, 0, ?, ?),
		('run-previous-fail', 'tenant-a', 'case-a', 'failed', 0, 0, ?, ?),
		('run-year-pass', 'tenant-a', 'case-a', 'passed', 1, 0, ?, ?),
		('run-year-review', 'tenant-a', 'case-a', 'needs_review', 0, 1, ?, ?),
		('run-other-tenant', 'tenant-b', 'case-b', 'passed', 1, 0, ?, ?)`,
		stamp(current.Add(-time.Second)), stamp(current),
		stamp(current), stamp(month.Add(-time.Second)),
		stamp(current), stamp(current),
		stamp(current), stamp(current),
		stamp(current), stamp(current),
		stamp(current),
		stamp(current), stamp(month),
		stamp(previous), stamp(previous),
		stamp(lastYear), stamp(lastYear),
		stamp(lastYear), stamp(lastYear),
		stamp(current), stamp(current)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`INSERT INTO bugs (id, tenant_id, project_id, bug_code, title, status, reopen_count, created_at, updated_at) VALUES
		('bug-current', 'tenant-a', 'project-a', 'A-1', 'Current', 'closed', 0, ?, ?),
		('bug-current-end', 'tenant-a', 'project-a', 'A-2', 'Current end', 'new', 0, ?, ?),
		('bug-open-month', 'tenant-a', 'project-a', 'A-3', 'Open month', 'new', 0, ?, ?),
		('bug-old-reopened', 'tenant-a', 'project-a', 'A-4', 'Reopened', 'open', 1, ?, ?),
		('bug-year', 'tenant-a', 'project-a', 'A-5', 'Last year', 'new', 0, ?, ?),
		('bug-other-tenant', 'tenant-b', 'project-b', 'B-1', 'Other tenant', 'new', 0, ?, ?)`,
		stamp(current), stamp(current),
		stamp(month.Add(-time.Second)), stamp(month),
		stamp(month), stamp(month),
		stamp(previous.Add(-time.Second)), stamp(current),
		stamp(lastYear), stamp(lastYear),
		stamp(current), stamp(current)); err != nil {
		t.Fatal(err)
	}

	handler := New(database).Routes()
	read := func(tenant string) map[string]json.RawMessage {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/trends", nil)
		request.Header.Set("X-Tenant-Id", tenant)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /api/trends returned %d: %s", response.Code, response.Body)
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"days", "bugs_by_sev", "bugs_by_stat", "reopen_rate", "closed_bugs", "comparison"} {
			if _, ok := result[key]; !ok {
				t.Fatalf("response missing %q: %s", key, response.Body)
			}
		}
		return result
	}
	type comparisonPeriod struct {
		Baseline map[string]any `json:"baseline"`
		Change   map[string]any `json:"change"`
	}
	var result, emptyResult struct {
		Current map[string]any   `json:"current"`
		MoM     comparisonPeriod `json:"mom"`
		YoY     comparisonPeriod `json:"yoy"`
	}
	assertFields := func(label string, got, want map[string]any) {
		t.Helper()
		for key, value := range want {
			if field, ok := got[key]; !ok || field != value {
				t.Errorf("%s.%s = %v (present: %t), want %v", label, key, field, ok, value)
			}
		}
	}
	if err := json.Unmarshal(read("tenant-a")["comparison"], &result); err != nil {
		t.Fatal(err)
	}
	assertFields("current", result.Current, map[string]any{
		"start": current.Format(time.DateOnly), "end": month.Format(time.DateOnly),
		"completed_runs": float64(5), "pass_rate": 0.5, "new_defects": float64(2),
	})
	assertFields("mom.baseline", result.MoM.Baseline, map[string]any{
		"start": previous.Format(time.DateOnly), "end": current.Format(time.DateOnly),
		"completed_runs": float64(1), "pass_rate": float64(0), "new_defects": float64(0),
	})
	assertFields("mom.change", result.MoM.Change, map[string]any{
		"completed_runs": float64(4), "pass_rate": nil, "new_defects": nil,
	})
	assertFields("yoy.baseline", result.YoY.Baseline, map[string]any{
		"start": lastYear.Format(time.DateOnly), "end": lastYear.AddDate(0, 1, 0).Format(time.DateOnly),
		"completed_runs": float64(2), "pass_rate": float64(1), "new_defects": float64(1),
	})
	assertFields("yoy.change", result.YoY.Change, map[string]any{
		"completed_runs": 1.5, "pass_rate": -0.5, "new_defects": float64(1),
	})

	if err := json.Unmarshal(read("empty")["comparison"], &emptyResult); err != nil {
		t.Fatal(err)
	}
	assertFields("empty.current", emptyResult.Current, map[string]any{
		"completed_runs": float64(0), "pass_rate": nil, "new_defects": float64(0),
	})
	for label, change := range map[string]map[string]any{"empty.mom": emptyResult.MoM.Change, "empty.yoy": emptyResult.YoY.Change} {
		assertFields(label, change, map[string]any{"completed_runs": nil, "pass_rate": nil, "new_defects": nil})
	}
}

func TestTrendsUTCMonthBoundaries(t *testing.T) {
	server := New(openTestStore(t))
	for _, fixture := range []struct {
		instant string
		starts  [3]string
		ends    [3]string
	}{
		{"2025-01-01T03:00:00+08:00", [3]string{"2024-11-01", "2024-10-01", "2023-11-01"}, [3]string{"2024-12-01", "2024-11-01", "2023-12-01"}},
		{"2025-01-01T00:00:00Z", [3]string{"2024-12-01", "2024-11-01", "2023-12-01"}, [3]string{"2025-01-01", "2024-12-01", "2024-01-01"}},
		{"2024-03-01T00:00:00Z", [3]string{"2024-02-01", "2024-01-01", "2023-02-01"}, [3]string{"2024-03-01", "2024-02-01", "2023-03-01"}},
	} {
		instant, err := time.Parse(time.RFC3339, fixture.instant)
		if err != nil {
			t.Fatal(err)
		}
		result, err := server.loadTrendComparison(t.Context(), "tenant-a", instant)
		if err != nil {
			t.Fatal(err)
		}
		for index, period := range []trendMonth{result.Current, result.MoM.Baseline, result.YoY.Baseline} {
			if period.Start != fixture.starts[index] || period.End != fixture.ends[index] {
				t.Errorf("%s period %d = [%s, %s), want [%s, %s)", fixture.instant, index, period.Start, period.End, fixture.starts[index], fixture.ends[index])
			}
		}
	}
}
