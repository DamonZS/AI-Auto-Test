package api

import (
	"net/http"
)

func (s *Server) trends(w http.ResponseWriter, r *http.Request, tid string) {
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
	})
}
