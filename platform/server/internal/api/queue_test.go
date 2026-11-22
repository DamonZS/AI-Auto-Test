package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"platform/server/internal/store"
)

func TestWorkersClaimSuiteBatchUpToSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	first, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	seedMCPData(t, first)
	second, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := first.DB.Exec(`UPDATE runs SET status = 'passed';
		UPDATE cases SET status = 'approved';
		INSERT INTO suites (id, tenant_id, name) VALUES ('suite-a', 'tenant-a', 'Suite A')`); err != nil {
		t.Fatal(err)
	}
	web := New(first).Routes()
	primary := httptest.NewServer(web)
	defer primary.Close()
	secondary := httptest.NewServer(New(second).Routes())
	defer secondary.Close()
	servers := []*httptest.Server{primary, secondary}
	const suitePath = "/api/suites/suite-a"
	setLimit := func(limit string) {
		t.Helper()
		response := serveTenant(web, http.MethodPatch, suitePath, "tenant-a", []byte(`{"max_concurrency":`+limit+`}`))
		if response.Code != http.StatusOK {
			t.Fatalf("save suite limit %s: %d %s", limit, response.Code, response.Body.String())
		}
	}
	setLimit("2")
	response := serveTenant(web, http.MethodPost, suitePath+"/run", "tenant-a", []byte(`{"case_ids":["case-a","case-a","case-a","case-a"]}`))
	if response.Code != http.StatusOK {
		t.Fatalf("enqueue suite: %d %s", response.Code, response.Body.String())
	}
	var batch struct {
		MaxConcurrency int `json:"max_concurrency"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil || batch.MaxConcurrency != 2 {
		t.Fatalf("manual batch snapshot: %s, err = %v", response.Body.String(), err)
	}
	setLimit("1")

	type claim struct {
		Run *struct {
			ID string `json:"run_id"`
		} `json:"run"`
	}
	workerPost := func(server *httptest.Server, route string, body []byte) []byte {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+route, bytes.NewReader(body))
		if err != nil {
			t.Error(err)
			return nil
		}
		request.Header.Set("X-Agent-Token", "tenant:tenant-a")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Error(err)
			return nil
		}
		defer response.Body.Close()
		result, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Errorf("%s: %d %s, err = %v", route, response.StatusCode, result, err)
			return nil
		}
		return result
	}
	const heartbeat = "/api/runs/heartbeat"
	const workers = 12
	start := make(chan struct{})
	claims := make(chan string, workers)
	var ready sync.WaitGroup
	ready.Add(workers)
	for index := range workers {
		go func() {
			ready.Done()
			<-start
			body := workerPost(servers[index%len(servers)], heartbeat, nil)
			var payload claim
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("heartbeat: %s, err = %v", body, err)
			}
			if payload.Run == nil {
				claims <- ""
				return
			}
			claims <- payload.Run.ID
		}()
	}
	ready.Wait()
	close(start)
	claimed := map[string]bool{}
	for range workers {
		if runID := <-claims; runID != "" {
			if claimed[runID] {
				t.Errorf("duplicate claim %s", runID)
			}
			claimed[runID] = true
		}
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %v, want 2 runs under the batch snapshot", claimed)
	}
	rows, err := first.DB.Query("SELECT id, status FROM runs WHERE suite_run_id <> '' ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}
	var runIDs []string
	for rows.Next() {
		var runID, status string
		if err := rows.Scan(&runID, &status); err != nil {
			t.Fatal(err)
		}
		runIDs = append(runIDs, runID)
		if (len(runIDs) <= 2) != claimed[runID] || (status == "running") != claimed[runID] {
			t.Errorf("unexpected claim order or status for run %s: %s", runID, status)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(runIDs) != 4 {
		t.Fatalf("persisted %d batch runs, want 4", len(runIDs))
	}
	if body := workerPost(primary, "/api/runs/"+runIDs[0]+"/finish", []byte(`{"status":"passed","passed":true}`)); body == nil {
		t.Fatal("finish run failed")
	}
	body := workerPost(secondary, heartbeat, nil)
	var next claim
	if err := json.Unmarshal(body, &next); err != nil || next.Run == nil || next.Run.ID != runIDs[2] {
		t.Fatalf("claim after slot released: %s, err = %v", body, err)
	}
}

func TestWorkerSkipsFullLegacyDefaultSuiteBatch(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	if _, err := st.DB.Exec(`UPDATE runs SET status = 'passed';
		INSERT INTO suites (id, tenant_id, name) VALUES ('suite-a', 'tenant-a', 'Suite A');
		INSERT INTO suite_runs (id, tenant_id, suite_id) VALUES ('batch-a', 'tenant-a', 'suite-a');
		INSERT INTO runs (id, tenant_id, case_id, suite_run_id, status) VALUES
		('running-member', 'tenant-a', 'case-a', 'batch-a', 'running'),
		('queued-member', 'tenant-a', 'case-a', 'batch-a', 'queued'),
		('standalone-run', 'tenant-a', 'case-a', '', 'queued')`); err != nil {
		t.Fatal(err)
	}
	response := serveAgent(New(st), "/api/runs/heartbeat", "tenant-a", nil)
	var payload struct {
		Run struct {
			ID string `json:"run_id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || response.Code != http.StatusOK || payload.Run.ID != "standalone-run" {
		t.Fatalf("claim behind full batch: %d %s, err = %v", response.Code, response.Body.String(), err)
	}
}
