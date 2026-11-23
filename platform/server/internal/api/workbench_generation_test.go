package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const workbenchGenerationPath = workbenchTestDocuments + "/generate"

func TestWorkbenchGenerationPersistsSevenDrafts(t *testing.T) {
	database, _ := workbenchTestServer(t)
	if err := ProvisionUser(t.Context(), database, "wb-qa", fixturePassword, "tenant-a", "", "editor"); err != nil {
		t.Fatal(err)
	}
	handler := New(database, AuthOptions{}).Routes()
	token := authLogin(t, handler, "wb-qa")
	const context = "核对商品列表是否显示库存状态"
	for _, fixture := range []struct {
		kind, key, completion string
	}{
		{"test_application", "scope", `{"fields":{"scope":"商品列表"}}`},
		{"test_plan", "milestones", `{"fields":{"milestones":"确认库存展示规则后执行验证"}}`},
		{"entrance_checklist", "smoke_passed", `{"fields":{},"checks":{"smoke_passed":{"guidance":"核对商品列表冒烟记录"}}}`},
		{"peer_review", "subject", `{"fields":{"subject":"库存状态的功能用例"}}`},
		{"daily_report", "next_steps", `{"fields":{"next_steps":"核对商品列表的库存状态"}}`},
		{"exit_checklist", "execution_archived", `{"fields":{},"checks":{"execution_archived":{"guidance":"核对版本的功能执行记录"}}}`},
		{"test_report", "test_scope_environment", `{"fields":{"test_scope_environment":"商品列表库存状态"}}`},
	} {
		t.Run(fixture.kind, func(t *testing.T) {
			request := func(method, path, body string, status int) *httptest.ResponseRecorder {
				return authRequest(t, handler, method, path, "tenant-a", token, body, status)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Model     string `json:"model"`
					MaxTokens int    `json:"max_tokens"`
					Messages  []struct{ Role, Content string }
				}
				err := json.NewDecoder(r.Body).Decode(&request)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+generationTestKey ||
					err != nil || request.Model != "tenant-model" || request.MaxTokens != 400 || len(request.Messages) != 2 {
					t.Errorf("unexpected tenant model request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				prompt := request.Messages[0].Content + request.Messages[1].Content
				if !strings.Contains(prompt, fixture.kind) || !strings.Contains(prompt, fixture.key) || !strings.Contains(prompt, context) ||
					strings.Contains(prompt, generationTestKey) || strings.Contains(prompt, "tenant-a") {
					t.Error("model prompt must include template and user context only")
				}
				writeCaseGenerationCompletion(w, fixture.completion)
			}))
			t.Cleanup(upstream.Close)
			if _, err := database.DB.Exec(`DELETE FROM tenant_settings WHERE tenant_id = 'tenant-a'`); err != nil {
				t.Fatal(err)
			}
			setCaseGenerationTestModel(t, database, upstream.URL)
			payload := mustJSON(map[string]string{"kind": fixture.kind, "project_id": "project-a", "version_id": "v-a", "title": "商品列表", "context": context})
			created := workbenchResponse[workbenchDocument](t, request("POST", workbenchGenerationPath, payload, 201))
			if created.Status != "draft" || created.Revision != 1 || created.Kind != fixture.kind || created.Title != "商品列表" ||
				created.Reviewer != "" || created.Metrics != nil {
				t.Fatalf("unexpected generated draft: %#v", created)
			}
			stored, err := scanWorkbenchDocument(database.DB.QueryRow(`SELECT `+workbenchColumns+`
				FROM workbench_documents WHERE id = ? AND tenant_id = 'tenant-a' AND project_id = 'project-a' AND version_id = 'v-a'`, created.ID))
			if err != nil || mustJSON(stored) != mustJSON(created) {
				t.Fatalf("draft was not persisted: %v", err)
			}
			if strings.Contains(fixture.kind, "checklist") {
				check := stored.Checks[fixture.key]
				if check == nil || check.Result != "" || check.Evidence != "" || !strings.HasPrefix(check.Guidance, "核对") {
					t.Fatalf("guidance must be distinct from evidence and results: %s", mustJSON(check))
				}
			} else if stored.Fields[fixture.key] == "" {
				t.Fatalf("generated field %q missing", fixture.key)
			}
			path := workbenchTestDocuments + "/" + created.ID
			request("POST", path+"/review", `{"revision":1,"action":"submit"}`, 422)
			request("POST", path+"/review", `{"revision":1,"action":"approve"}`, 409)
			request("GET", path+"/export", "", 409)
			if fixture.kind == "entrance_checklist" {
				for _, item := range findWorkbenchTemplate(fixture.kind).Checks {
					if stored.Checks[item.Key] == nil {
						stored.Checks[item.Key] = &workbenchCheck{}
					}
					stored.Checks[item.Key].Result = "pass"
					stored.Checks[item.Key].Evidence = "人工核验记录"
				}
				saved := workbenchResponse[workbenchDocument](t, request("PATCH", path,
					mustJSON(map[string]any{"revision": 1, "title": stored.Title, "fields": stored.Fields, "checks": stored.Checks}), 200))
				if saved.Checks[fixture.key].Guidance != "核对商品列表冒烟记录" {
					t.Fatalf("guidance lost after editing: %#v", saved.Checks[fixture.key])
				}
				request("POST", path+"/review", `{"revision":2,"action":"submit"}`, 200)
				request("GET", path+"/export", "", 409)
				request("POST", path+"/review", `{"revision":3,"action":"approve","reviewer":"spoofed"}`, 200)
				html := request("GET", path+"/export", "", 200).Body.String()
				if !strings.Contains(html, "人工核验记录") || !strings.Contains(html, "wb-qa") || strings.Contains(html, "核对商品列表冒烟记录") {
					t.Fatal("signed export must present human evidence and the session reviewer")
				}
			}
		})
	}
}

func TestWorkbenchGenerationRejectsInvalidRequestsAndOutput(t *testing.T) {
	database, _ := workbenchTestServer(t)
	for _, account := range []struct{ name, role string }{{"wb-editor", "editor"}, {"wb-viewer", "viewer"}} {
		if err := ProvisionUser(t.Context(), database, account.name, fixturePassword, "tenant-a", "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(database, AuthOptions{}).Routes()
	editor := authLogin(t, handler, "wb-editor")
	viewer := authLogin(t, handler, "wb-viewer")
	const payload = `{"kind":"exit_checklist","project_id":"project-a","version_id":"v-a","title":"准出","context":"验证列表页"}`
	authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-a", "", payload, 401)
	authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-a", viewer, payload, 403)
	authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-a", editor, payload, 422)
	responses := make(chan string, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case completion := <-responses:
			writeCaseGenerationCompletion(w, completion)
		default:
			t.Error("invalid request reached the model")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(upstream.Close)
	setCaseGenerationTestModel(t, database, upstream.URL)
	authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-b", editor, payload, 403)
	authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-a", editor, strings.Replace(payload, "v-a", "v-b", 1), 404)
	authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-a", editor, strings.Replace(payload, "验证列表页", " ", 1), 400)
	authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-a", editor, strings.Replace(payload, "验证列表页", generationTestKey, 1), 422)
	for _, completion := range []string{
		`{"fields":{},"status":"approved"}`,
		`{"fields":{},"checks":{"execution_archived":{"result":"pass","evidence":"已归档"}}}`,
		`{"fields":{},"checks":{"unknown":{"guidance":"核对记录"}}}`,
		`{"fields":{},"checks":{"execution_archived":{"guidance":"核对记录"},"coverage_met":null}}`,
		`{"fields":{"unknown":"未知字段"}}`,
		`{"fields":{}}`,
		`{"fields":{},"checks":{"execution_archived":{"guidance":"` + generationTestKey + `"}}}`,
		`{"fields":{},"checks":{"execution_archived":{"guidance":"核对记录"}}} {}`,
	} {
		responses <- completion
		response := authRequest(t, handler, "POST", workbenchGenerationPath, "tenant-a", editor, payload, 502)
		if strings.Contains(response.Body.String(), generationTestKey) {
			t.Fatal("model key leaked into error response")
		}
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM workbench_documents`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid generation saved %d drafts (%v)", count, err)
	}
}
