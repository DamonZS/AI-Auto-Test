package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type caseSchedule struct {
	CaseID string `json:"case_id"`
	weeklySchedule
}

type suiteSchedule struct {
	SuiteID string `json:"suite_id"`
	weeklySchedule
}

type weeklySchedule struct {
	Enabled bool   `json:"enabled"`
	Weekday int    `json:"weekday"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	NextRun string `json:"next_run_at"`
}

func (schedule weeklySchedule) validTime() bool {
	return schedule.Weekday >= 0 && schedule.Weekday <= 6 && schedule.Hour >= 0 && schedule.Hour <= 23 && schedule.Minute >= 0 && schedule.Minute <= 59
}

func (s *Server) getCaseSchedule(w http.ResponseWriter, r *http.Request, tid string) {
	cid := r.PathValue("id")
	var exists int
	if err := s.st.DB.QueryRow("SELECT 1 FROM cases WHERE id = ? AND tenant_id = ?", cid, tid).Scan(&exists); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "用例不存在")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	var item caseSchedule
	var enabled int
	err := s.st.DB.QueryRow(`SELECT case_id, enabled, weekday, hour, minute FROM case_schedules WHERE case_id = ? AND tenant_id = ?`, cid, tid).
		Scan(&item.CaseID, &enabled, &item.Weekday, &item.Hour, &item.Minute)
	if err == sql.ErrNoRows {
		write(w, http.StatusOK, map[string]any{"schedule": nil})
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	item.Enabled = enabled == 1
	if item.Enabled {
		item.NextRun = nextScheduleRun(time.Now(), item.Weekday, item.Hour, item.Minute).Format(time.RFC3339)
	}
	write(w, http.StatusOK, map[string]any{"schedule": item})
}

func (s *Server) putCaseSchedule(w http.ResponseWriter, r *http.Request, tid string) {
	var body caseSchedule
	if err := decode(r, &body); err != nil || !body.validTime() {
		fail(w, http.StatusBadRequest, "weekday、hour 或 minute 超出范围")
		return
	}
	cid := r.PathValue("id")
	var status string
	if err := s.st.DB.QueryRow("SELECT status FROM cases WHERE id = ? AND tenant_id = ?", cid, tid).Scan(&status); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "用例不存在")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status != "approved" {
		fail(w, http.StatusConflict, "仅已定稿用例可设置计划")
		return
	}
	enabled := 0
	if body.Enabled {
		enabled = 1
	}
	_, err := s.st.DB.Exec(`INSERT INTO case_schedules (case_id, tenant_id, enabled, weekday, hour, minute, last_run_key, last_run_id)
		VALUES (?, ?, ?, ?, ?, ?, '', '') ON CONFLICT(case_id) DO UPDATE SET tenant_id = excluded.tenant_id,
		enabled = excluded.enabled, weekday = excluded.weekday, hour = excluded.hour, minute = excluded.minute,
		last_run_key = '', last_run_id = '', updated_at = datetime('now')`, cid, tid, enabled, body.Weekday, body.Hour, body.Minute)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, "", "web", "case.schedule.save", cid)
	body.CaseID = cid
	if body.Enabled {
		body.NextRun = nextScheduleRun(time.Now(), body.Weekday, body.Hour, body.Minute).Format(time.RFC3339)
	}
	write(w, http.StatusOK, body)
}

func (s *Server) deleteCaseSchedule(w http.ResponseWriter, r *http.Request, tid string) {
	cid := r.PathValue("id")
	_, err := s.st.DB.Exec("DELETE FROM case_schedules WHERE case_id = ? AND tenant_id = ?", cid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, "", "web", "case.schedule.delete", cid)
	write(w, http.StatusOK, map[string]string{"case_id": cid})
}

func (s *Server) requireScheduleSuite(w http.ResponseWriter, tid, suiteID string) bool {
	var exists int
	switch err := s.st.DB.QueryRow("SELECT 1 FROM suites WHERE id = ? AND tenant_id = ?", suiteID, tid).Scan(&exists); err {
	case nil:
		return true
	case sql.ErrNoRows:
		fail(w, http.StatusNotFound, "套件不存在")
	default:
		fail(w, http.StatusInternalServerError, err.Error())
	}
	return false
}

func (s *Server) getSuiteSchedule(w http.ResponseWriter, r *http.Request, tid string) {
	suiteID := r.PathValue("id")
	if !s.requireScheduleSuite(w, tid, suiteID) {
		return
	}
	item := suiteSchedule{SuiteID: suiteID}
	err := s.st.DB.QueryRow(`SELECT enabled, weekday, hour, minute FROM suite_schedules WHERE tenant_id = ? AND suite_id = ?`, tid, suiteID).
		Scan(&item.Enabled, &item.Weekday, &item.Hour, &item.Minute)
	if err == sql.ErrNoRows {
		write(w, http.StatusOK, map[string]any{"schedule": nil})
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item.Enabled {
		item.NextRun = nextScheduleRun(time.Now(), item.Weekday, item.Hour, item.Minute).Format(time.RFC3339)
	}
	write(w, http.StatusOK, map[string]any{"schedule": item})
}

func (s *Server) putSuiteSchedule(w http.ResponseWriter, r *http.Request, tid string) {
	var schedule weeklySchedule
	if err := decode(r, &schedule); err != nil || !schedule.validTime() {
		fail(w, http.StatusBadRequest, "weekday、hour 或 minute 超出范围")
		return
	}
	suiteID := r.PathValue("id")
	if !s.requireScheduleSuite(w, tid, suiteID) {
		return
	}
	_, err := s.st.DB.Exec(`INSERT INTO suite_schedules (suite_id, tenant_id, enabled, weekday, hour, minute)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(suite_id, tenant_id) DO UPDATE SET
		enabled = excluded.enabled, weekday = excluded.weekday, hour = excluded.hour, minute = excluded.minute`,
		suiteID, tid, schedule.Enabled, schedule.Weekday, schedule.Hour, schedule.Minute)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, "", "web", "suite.schedule.save", suiteID)
	schedule.NextRun = ""
	if schedule.Enabled {
		schedule.NextRun = nextScheduleRun(time.Now(), schedule.Weekday, schedule.Hour, schedule.Minute).Format(time.RFC3339)
	}
	write(w, http.StatusOK, suiteSchedule{SuiteID: suiteID, weeklySchedule: schedule})
}

func (s *Server) deleteSuiteSchedule(w http.ResponseWriter, r *http.Request, tid string) {
	suiteID := r.PathValue("id")
	if !s.requireScheduleSuite(w, tid, suiteID) {
		return
	}
	if _, err := s.st.DB.Exec("DELETE FROM suite_schedules WHERE tenant_id = ? AND suite_id = ?", tid, suiteID); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, "", "web", "suite.schedule.delete", suiteID)
	write(w, http.StatusOK, map[string]string{"suite_id": suiteID})
}

var scheduleRunMu sync.Mutex

func (s *Server) fireDueSchedules(now time.Time) []string {
	scheduleRunMu.Lock()
	defer scheduleRunMu.Unlock()
	return append(s.fireDueCaseSchedules(now), s.fireDueSuiteSchedules(now)...)
}

func (s *Server) fireDueCaseSchedules(now time.Time) []string {
	rows, err := s.st.DB.Query(`SELECT cs.case_id, cs.tenant_id, cs.weekday, cs.hour, cs.minute, cs.last_run_key,
		c.version_no, c.dataset_id, c.data_row_id FROM case_schedules cs JOIN cases c ON c.id = cs.case_id
		WHERE cs.enabled = 1 AND c.status = 'approved'`)
	if err != nil {
		return nil
	}
	type dueSchedule struct {
		caseID, tenantID, lastKey, datasetID, dataRowID string
		weekday, hour, minute, version                  int
	}
	var due []dueSchedule
	for rows.Next() {
		var item dueSchedule
		if rows.Scan(&item.caseID, &item.tenantID, &item.weekday, &item.hour, &item.minute, &item.lastKey,
			&item.version, &item.datasetID, &item.dataRowID) == nil {
			due = append(due, item)
		}
	}
	_ = rows.Close()
	var queued []string
	for _, item := range due {
		if now.Weekday() != time.Weekday(item.weekday) || now.Hour() != item.hour || now.Minute() != item.minute {
			continue
		}
		runKey := now.Format("2006-01-02 15:04")
		if runKey == item.lastKey {
			continue
		}
		runID := id()
		result, err := s.st.DB.Exec(`INSERT INTO runs (id, tenant_id, case_id, version_id, dataset_id, data_row_id)
			SELECT ?, ?, ?, '', ?, ? WHERE EXISTS (
				SELECT 1 FROM case_schedules WHERE case_id = ? AND tenant_id = ? AND enabled = 1 AND last_run_key = ?
			)`, runID, item.tenantID, item.caseID, item.datasetID, item.dataRowID, item.caseID, item.tenantID, item.lastKey)
		if err != nil {
			continue
		}
		inserted, _ := result.RowsAffected()
		if inserted == 0 {
			continue
		}
		_, _ = s.st.DB.Exec("UPDATE case_schedules SET last_run_key = ?, last_run_id = ?, updated_at = datetime('now') WHERE case_id = ? AND tenant_id = ?", runKey, runID, item.caseID, item.tenantID)
		_, _ = s.st.DB.Exec("INSERT INTO audit_logs (tenant_id, actor, channel, action, target) VALUES (?, '', 'scheduler', 'run.create', ?)", item.tenantID, runID)
		queued = append(queued, runID)
	}
	return queued
}

func (s *Server) fireDueSuiteSchedules(now time.Time) []string {
	rows, err := s.st.DB.Query(`SELECT tenant_id, suite_id FROM suite_schedules
		WHERE enabled = 1 AND weekday = ? AND hour = ? AND minute = ?`, now.Weekday(), now.Hour(), now.Minute())
	if err != nil {
		return nil
	}
	type dueSuite struct{ tenantID, suiteID string }
	var due []dueSuite
	for rows.Next() {
		var item dueSuite
		if rows.Scan(&item.tenantID, &item.suiteID) == nil {
			due = append(due, item)
		}
	}
	_ = rows.Close()
	var queued []string
	for _, item := range due {
		runKey := mustJSON([]string{"suite-schedule", item.tenantID, item.suiteID, now.Format("2006-01-02 15:04")})
		runID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(runKey)).String()
		if s.enqueueScheduledSuite(item.tenantID, item.suiteID, runID, now) {
			queued = append(queued, runID)
		}
	}
	return queued
}

func (s *Server) enqueueScheduledSuite(tid, suiteID, runID string, now time.Time) bool {
	tx, err := s.st.DB.Begin()
	if err != nil {
		return false
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO suite_runs (id, tenant_id, suite_id, max_concurrency)
		SELECT ?, suite.tenant_id, suite.id, suite.max_concurrency FROM suites suite JOIN suite_schedules schedule
		ON schedule.suite_id = suite.id AND schedule.tenant_id = suite.tenant_id
		WHERE suite.id = ? AND suite.tenant_id = ? AND schedule.enabled = 1
		AND schedule.weekday = ? AND schedule.hour = ? AND schedule.minute = ?
		ON CONFLICT(id) DO NOTHING`, runID, suiteID, tid, now.Weekday(), now.Hour(), now.Minute())
	if err != nil {
		return false
	}
	if inserted, err := result.RowsAffected(); err != nil || inserted == 0 {
		return false
	}
	var members string
	if err := tx.QueryRow("SELECT case_ids FROM suites WHERE id = ? AND tenant_id = ?", suiteID, tid).Scan(&members); err != nil {
		return false
	}
	var caseIDs []string
	if err := json.Unmarshal([]byte(members), &caseIDs); err != nil || len(caseIDs) == 0 {
		return false
	}
	if err := enqueueSuiteMembers(tx, tid, runID, caseIDs); err != nil {
		return false
	}
	if _, err := tx.Exec("INSERT INTO audit_logs (tenant_id, actor, channel, action, target) VALUES (?, '', 'scheduler', 'suite.run.create', ?)", tid, runID); err != nil {
		return false
	}
	return tx.Commit() == nil
}

func (s *Server) StartScheduleLoop() {
	go func() {
		for {
			now := time.Now()
			s.fireDueSchedules(now)
			time.Sleep(time.Minute - time.Duration(now.Second())*time.Second - time.Duration(now.Nanosecond()))
		}
	}()
}

func nextScheduleRun(after time.Time, weekday, hour, minute int) time.Time {
	days := (weekday - int(after.Weekday()) + 7) % 7
	target := time.Date(after.Year(), after.Month(), after.Day()+days, hour, minute, 0, 0, after.Location())
	if !target.After(after) {
		target = target.AddDate(0, 0, 7)
	}
	return target
}
