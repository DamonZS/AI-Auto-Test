package api

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) trends(w http.ResponseWriter, r *http.Request, tid string) {
	comparison, err := s.loadTrendComparison(r.Context(), tid, time.Now())
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法读取趋势对比")
		return
	}

	rows, _ := s.st.DB.Query(`SELECT substr(started_at,1,10) AS day,
		count(*) AS total,
		COALESCE(sum(CASE WHEN passed = 1 THEN 1 ELSE 0 END),0) AS passed,
		COALESCE(sum(CASE WHEN passed = 0 THEN 1 ELSE 0 END),0) AS failed,
		COALESCE(sum(CASE WHEN needs_review = 1 THEN 1 ELSE 0 END),0) AS needs_review,
		COALESCE(sum(self_healed),0) AS self_healed
		FROM runs WHERE tenant_id = ? AND passed IS NOT NULL
		GROUP BY day ORDER BY day DESC LIMIT 30`, tid)
	days, _ := rowsToMaps(rows)

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
