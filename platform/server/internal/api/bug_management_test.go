package api

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestBugManagementScopedWorkflow(t *testing.T) {
	database := openTestStore(t)
	seedMCPData(t, database)
	_, err := database.DB.Exec(`INSERT INTO projects (id, tenant_id, name, base_url)
		VALUES ('other-project', 'tenant-a', 'Other', 'https://other.test');
		INSERT INTO versions (id, tenant_id, project_id, name) VALUES
		('found-a', 'tenant-a', 'project-a', 'v1.0'), ('fix-a', 'tenant-a', 'project-a', 'v1.1'),
		('other-version', 'tenant-a', 'other-project', 'Other version'),
		('foreign-version', 'tenant-b', 'project-b', 'Foreign version');
		INSERT INTO bugs (id, tenant_id, project_id, bug_code, title) VALUES
		('other-bug', 'tenant-a', 'other-project', 'BUG-other', 'Login 100%'),
		('similar-bug', 'tenant-a', 'project-a', 'BUG-similar', 'Login 100 success'),
		('foreign-bug', 'tenant-b', 'project-b', 'BUG-foreign', 'Login 100%');`)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(database).Routes()

	request := func(method, path, tenant, body string, want int, into any) {
		t.Helper()
		response := serveTenant(handler, method, path, tenant, []byte(body))
		if response.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.Code, want, response.Body)
		}
		if into != nil {
			if err := json.Unmarshal(response.Body.Bytes(), into); err != nil {
				t.Fatal(err)
			}
		}
	}
	request("POST", "/api/bugs", "tenant-a", `{"project_id":"project-a","title":"Wrong link","discovered_version_id":"other-version"}`, 400, nil)
	var created struct {
		ID   string `json:"id"`
		Code string `json:"bug_code"`
	}
	request("POST", "/api/bugs", "tenant-a", `{"project_id":"project-a","title":"Login 100%","severity":"S2","discovered_version_id":"found-a","assignee":"Anne","repro_steps":"Open login","expected_result":"Signed in","actual_result":"Error"}`, 200, &created)
	if created.Code != "BUG-tena-3" || created.ID == "" {
		t.Fatalf("unexpected defect identifier: %+v", created)
	}
	path := "/api/bugs/" + created.ID
	var matches []struct{ ID string }
	request("GET", "/api/bugs?project_id=project-a&status=new&severity=S2&q="+url.QueryEscape("login 100%"), "tenant-a", "", 200, &matches)
	if len(matches) != 1 || matches[0].ID != created.ID {
		t.Fatalf("unexpected combined filter results: %+v", matches)
	}
	request("GET", "/api/bugs?project_id=project-a&q="+url.QueryEscape("100%"), "tenant-a", "", 200, &matches)
	if len(matches) != 1 || matches[0].ID != created.ID {
		t.Fatalf("query must treat %% literally: %+v", matches)
	}
	request("GET", path, "tenant-b", "", 404, nil)
	request("PATCH", path, "tenant-b", `{"assignee":"Other"}`, 404, nil)
	request("PATCH", path, "tenant-a", `{"discovered_version_id":"foreign-version"}`, 400, nil)
	request("PATCH", path, "tenant-a", `{"assignee":"Lee"}`, 200, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"closed","note":"skip"}`, 409, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"confirmed","note":"Confirmed"}`, 200, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"fixing"}`, 200, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"fixed","fixed_version_id":"other-version"}`, 400, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"fixed","fixed_version_id":"foreign-version"}`, 400, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"fixed","fixed_version_id":"fix-a","note":"Fixed in build"}`, 200, nil)

	var detail struct {
		Status      string
		Assignee    string
		Repro       string   `json:"repro_steps"`
		Expected    string   `json:"expected_result"`
		Actual      string   `json:"actual_result"`
		Discovered  string   `json:"discovered_version_name"`
		Fixed       string   `json:"fixed_version_name"`
		FixedID     string   `json:"fixed_version_id"`
		ReopenCount int      `json:"reopen_count"`
		Next        []string `json:"available_transitions"`
		History     []struct {
			From string `json:"from_status"`
			To   string `json:"to_status"`
			Note string
		}
	}
	request("GET", path, "tenant-a", "", 200, &detail)
	if detail.Status != "fixed" || detail.Assignee != "Lee" || detail.Repro != "Open login" || detail.Expected != "Signed in" || detail.Actual != "Error" || detail.Discovered != "v1.0" || detail.Fixed != "v1.1" || detail.FixedID != "fix-a" {
		t.Fatalf("unexpected defect details: %+v", detail)
	}
	if len(detail.Next) != 2 || detail.Next[0] != "verified" || detail.Next[1] != "reopened" || len(detail.History) != 5 || detail.History[0].From != "fixing" || detail.History[0].To != "fixed" || detail.History[0].Note != "Fixed in build" {
		t.Fatalf("unexpected actions or history: %+v", detail)
	}
	request("POST", path+"/transition", "tenant-a", `{"to":"verified","note":"Retest passed"}`, 200, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"closed","note":"Done"}`, 200, nil)
	request("POST", path+"/transition", "tenant-a", `{"to":"reopened","note":"Regression"}`, 200, nil)
	request("GET", path, "tenant-a", "", 200, &detail)
	if detail.Status != "reopened" || detail.ReopenCount != 1 || detail.FixedID != "fix-a" || len(detail.History) != 8 || detail.History[0].Note != "Regression" {
		t.Fatalf("unexpected reopen state: %+v", detail)
	}
}
