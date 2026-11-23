package api

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) trends(w http.ResponseWriter, r *http.Request, tid string) {
	now := time.Now().UTC()
	comparison, err := s.loadTrendComparison(r.Context(), tid, now)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法读取趋势对比")
		return
	}

	days, err := s.loadTrendDays(r.Context(), tid, now)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法读取每日趋势")
		return
	}

	sev, _ := s.st.DB.Query(`SELECT severity, count(*) AS n FROM bugs WHERE tenant_id = ?
		GROUP BY severity ORDER BY severity`, tid)
	bySeverity, _ := rowsToMaps(sev)

	bstat, _ := s.st.DB.Query(`SELECT status, count(*) AS n FROM bugs WHERE tenant_id = ?
		GROUP BY status`, tid)
	byStatus, _ := rowsToMaps(bstat)

	var total, closed, reopened int
	_ = s.st.DB.QueryRow("SELECT count(*) FROM bugs WHERE tenant_id = ?", tid).Scan(&total)
	_ = s.st.DB.QueryRow("SELECT count(*) FROM bugs WHERE tenant_id = ? AND status = 'closed'", tid).Scan(&closed)
	_ = s.st.DB.QueryRow("SELECT count(*) FROM bugs WHERE tenant_id = ? AND reopen_count > 0", tid).Scan(&reopened)

	reopenRate := 0.0
	if total > 0 {
		reopenRate = float64(reopened) / float64(total)
	}
	write(w, http.StatusOK, map[string]any{
		"days":         days,
		"bugs_by_sev":  bySeverity,
		"bugs_by_stat": byStatus,
		"reopen_rate":  reopenRate,
		"closed_bugs":  closed,
		"comparison":   comparison,
	})
}

type trendDay struct {
	Day         string `json:"day"`
	Total       int    `json:"total"`
	Passed      int    `json:"passed"`
	Failed      int    `json:"failed"`
	NeedsReview int    `json:"needs_review"`
	Error       int    `json:"error"`
	SelfHealed  int    `json:"self_healed"`
	NewBugs     int    `json:"new_bugs"`
}

func (s *Server) loadTrendDays(ctx context.Context, tenantID string, now time.Time) ([]trendDay, error) {
	today := now.UTC()
	start := today.AddDate(0, 0, -29).Format(time.DateOnly)
	end := today.AddDate(0, 0, 1).Format(time.DateOnly)
	rows, err := s.st.DB.QueryContext(ctx, `WITH RECURSIVE days(day) AS (
		VALUES (?)
		UNION ALL
		SELECT date(day, '+1 day') FROM days WHERE date(day, '+1 day') < ?
	), run_days AS (
		SELECT substr(ended_at, 1, 10) AS day, count(*) AS total,
			count(*) FILTER (WHERE status = 'passed' AND needs_review = 0) AS passed,
			count(*) FILTER (WHERE status = 'failed' AND needs_review = 0) AS failed,
			count(*) FILTER (WHERE status = 'needs_review' OR needs_review = 1) AS needs_review,
			count(*) FILTER (WHERE status = 'error' AND needs_review = 0) AS error,
			sum(self_healed) AS self_healed
		FROM runs WHERE tenant_id = ? AND ended_at >= ? AND ended_at < ? AND status NOT IN ('queued', 'running')
		GROUP BY day
	), bug_days AS (
		SELECT substr(created_at, 1, 10) AS day, count(*) AS new_bugs
		FROM bugs WHERE tenant_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY day
	)
	SELECT days.day, COALESCE(run_days.total, 0), COALESCE(run_days.passed, 0), COALESCE(run_days.failed, 0),
		COALESCE(run_days.needs_review, 0), COALESCE(run_days.error, 0), COALESCE(run_days.self_healed, 0),
		COALESCE(bug_days.new_bugs, 0)
	FROM days LEFT JOIN run_days USING (day) LEFT JOIN bug_days USING (day) ORDER BY days.day`,
		start, end, tenantID, start, end, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := make([]trendDay, 0, 30)
	for rows.Next() {
		var day trendDay
		if err := rows.Scan(&day.Day, &day.Total, &day.Passed, &day.Failed, &day.NeedsReview, &day.Error, &day.SelfHealed, &day.NewBugs); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

type trendMonth struct {
	Start         string   `json:"start"`
	End           string   `json:"end"`
	CompletedRuns int      `json:"completed_runs"`
	PassRate      *float64 `json:"pass_rate"`
	NewDefects    int      `json:"new_defects"`
}

type trendChanges struct {
	CompletedRuns *float64 `json:"completed_runs"`
	PassRate      *float64 `json:"pass_rate"`
	NewDefects    *float64 `json:"new_defects"`
}

type trendPeriodComparison struct {
	Baseline trendMonth   `json:"baseline"`
	Change   trendChanges `json:"change"`
}

type trendComparison struct {
	Current trendMonth            `json:"current"`
	MoM     trendPeriodComparison `json:"mom"`
	YoY     trendPeriodComparison `json:"yoy"`
}

func (s *Server) loadTrendComparison(ctx context.Context, tenantID string, now time.Time) (trendComparison, error) {
	utc := now.UTC()
	month := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	currentStart := month.AddDate(0, -1, 0)
	current, err := s.readTrendMonth(ctx, tenantID, currentStart)
	if err != nil {
		return trendComparison{}, err
	}
	previous, err := s.readTrendMonth(ctx, tenantID, currentStart.AddDate(0, -1, 0))
	if err != nil {
		return trendComparison{}, err
	}
	lastYear, err := s.readTrendMonth(ctx, tenantID, currentStart.AddDate(-1, 0, 0))
	if err != nil {
		return trendComparison{}, err
	}
	return trendComparison{current, compareTrendMonths(current, previous), compareTrendMonths(current, lastYear)}, nil
}

func (s *Server) readTrendMonth(ctx context.Context, tenantID string, start time.Time) (trendMonth, error) {
	month := trendMonth{Start: start.Format(time.DateOnly), End: start.AddDate(0, 1, 0).Format(time.DateOnly)}
	var passed, failed int
	err := s.st.DB.QueryRowContext(ctx, `SELECT count(*),
		count(*) FILTER (WHERE status = 'passed' AND needs_review = 0),
		count(*) FILTER (WHERE status = 'failed' AND needs_review = 0),
		(SELECT count(*) FROM bugs WHERE tenant_id = ? AND created_at >= ? AND created_at < ?)
		FROM runs WHERE tenant_id = ? AND ended_at >= ? AND ended_at < ? AND status NOT IN ('queued', 'running')`,
		tenantID, month.Start, month.End, tenantID, month.Start, month.End).
		Scan(&month.CompletedRuns, &passed, &failed, &month.NewDefects)
	if passed+failed > 0 {
		rate := float64(passed) / float64(passed+failed)
		month.PassRate = &rate
	}
	return month, err
}

func compareTrendMonths(current, baseline trendMonth) trendPeriodComparison {
	changes := trendChanges{
		CompletedRuns: trendRelativeChange(float64(current.CompletedRuns), float64(baseline.CompletedRuns)),
		NewDefects:    trendRelativeChange(float64(current.NewDefects), float64(baseline.NewDefects)),
	}
	if current.PassRate != nil && baseline.PassRate != nil {
		changes.PassRate = trendRelativeChange(*current.PassRate, *baseline.PassRate)
	}
	return trendPeriodComparison{Baseline: baseline, Change: changes}
}

func trendRelativeChange(current, baseline float64) *float64 {
	if baseline == 0 {
		return nil
	}
	change := current/baseline - 1
	return &change
}
