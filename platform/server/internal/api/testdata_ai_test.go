package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"platform/server/internal/store"
)

const aiDataPath = "/api/test-data/generate"

func TestAIDataSetSavesSelectedFieldsAndProtectsRecords(t *testing.T) {
	database, handler, editor, _ := aiDataFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct{ Role, Content string }
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+generationTestKey ||
			json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Messages) != 2 || body.Model != "tenant-model" || body.MaxTokens != 400 {
			t.Error("unexpected model request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		messages := mustJSON(body.Messages)
		if !strings.Contains(messages, "证件号码") || !strings.Contains(messages, "document_number") {
			t.Error("selected field meanings are missing")
		}
		for _, forbidden := range []string{"project-a", "tenant-a", "env-ai", "a.test", "private-environment.test", generationTestKey} {
			if strings.Contains(messages, forbidden) {
				t.Errorf("unexpected metadata in model prompt: %q", forbidden)
			}
		}
		writeCaseGenerationCompletion(w, `{"name":"订单数据","rows":[{"username":"","password":"model-value","phone":"123","document_number":"arbitrary","amount":-12.50}]}`)
	}))
	t.Cleanup(upstream.Close)
	setCaseGenerationTestModel(t, database, upstream.URL)
	result := authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor,
		`{"project_id":"project-a","environment_id":"env-ai","count":1,"fields":["username","password","phone","document_number","amount"]}`, http.StatusCreated)
	var created struct {
		ID          string
		Name        string
		RecordCount int `json:"record_count"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &created); err != nil || created.ID == "" || created.Name != "订单数据" || created.RecordCount != 1 {
		t.Fatalf("create response = %s (%v)", result.Body, err)
	}
	path := "/api/test-data/datasets/" + created.ID
	list := authRequest(t, handler, "GET", "/api/test-data/datasets", "tenant-a", editor, "", http.StatusOK)
	if !strings.Contains(list.Body.String(), `"generation_source":"ai"`) {
		t.Fatalf("AI source missing: %s", list.Body)
	}
	authRequest(t, handler, "POST", path+"/generate", "tenant-a", editor, `{"count":2}`, http.StatusConflict)
	var raw string
	err := database.DB.QueryRow(`SELECT values_json FROM test_data_records WHERE dataset_id = ? AND tenant_id = 'tenant-a'`, created.ID).Scan(&raw)
	if err != nil || raw != `{"amount":"-12.50","document_number":"TEST-DOC-0001","password":"SyntheticQA-0001!","phone":"+1-202-555-0100","username":""}` {
		t.Fatalf("stored row = %s (%v)", raw, err)
	}
	masked := authRequest(t, handler, "GET", path+"/records", "tenant-a", editor, "", http.StatusOK)
	for _, field := range []string{"password", "phone", "document_number"} {
		if !strings.Contains(masked.Body.String(), `"`+field+`":"••••••"`) {
			t.Errorf("%s was not masked: %s", field, masked.Body)
		}
	}
}

func TestAIDataSetRejectsInvalidRequestsBeforeModelCall(t *testing.T) {
	database, handler, editor, viewer := aiDataFixture(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)
	const valid = `{"project_id":"project-a","count":1,"fields":["password"]}`
	authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor, valid, http.StatusUnprocessableEntity)
	setCaseGenerationTestModel(t, database, upstream.URL)
	authRequest(t, handler, "POST", aiDataPath, "tenant-a", "", valid, http.StatusUnauthorized)
	authRequest(t, handler, "POST", aiDataPath, "tenant-a", viewer, valid, http.StatusForbidden)
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"project_id":"project-b","count":1,"fields":["password"]}`, http.StatusNotFound},
		{`{"project_id":"project-a","environment_id":"foreign-env","count":1,"fields":["password"]}`, http.StatusNotFound},
		{`{"project_id":"project-a","environment_id":"other-project-env","count":1,"fields":["password"]}`, http.StatusNotFound},
		{`{"project_id":"project-a","count":31,"fields":["password"]}`, http.StatusBadRequest},
		{`{"project_id":"project-a","count":0,"fields":["password"]}`, http.StatusBadRequest},
		{`{"project_id":"project-a","count":1,"fields":["password","password"]}`, http.StatusBadRequest},
		{`{"project_id":"project-a","count":1,"fields":["unknown"]}`, http.StatusBadRequest},
		{`{"project_id":"project-a","count":1}`, http.StatusBadRequest},
		{`{"project_id":"project-a","count":1,"prompt":"password: \"fixture-secret!\""}`, http.StatusUnprocessableEntity},
		{`{"project_id":"project-a","count":1,"prompt":"密码为 private-fixture-value!"}`, http.StatusUnprocessableEntity},
		{`{"project_id":"project-a","count":1,"prompt":"api_key=fixture-key"}`, http.StatusUnprocessableEntity},
		{`{"project_id":"project-a","count":1,"prompt":"使用 https://example.test 的数据"}`, http.StatusUnprocessableEntity},
	} {
		authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor, test.body, test.status)
	}
	if calls.Load() != 0 {
		t.Fatal("invalid requests reached model")
	}
	authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor, valid, http.StatusBadGateway)
	assertNoAIData(t, database)
}

func TestAIDataSetRejectsInvalidModelOutputAndRollsBackWrites(t *testing.T) {
	database, handler, editor, _ := aiDataFixture(t)
	var completion atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeCaseGenerationCompletion(w, completion.Load().(string))
	}))
	t.Cleanup(upstream.Close)
	setCaseGenerationTestModel(t, database, upstream.URL)
	const selected = `{"project_id":"project-a","count":2,"fields":["quantity"]}`
	for _, output := range []string{
		`malformed`,
		`{"name":"订单","rows":[{"quantity":2}]}`,
		`{"name":"订单","rows":[{"quantity":2},{"extra":3}]}`,
		`{"name":"订单","rows":[{"quantity":2},{"quantity":3,"extra":3}]}`,
		`{"name":"订单","rows":[{"quantity":2},{"quantity":"词"}]}`,
		`{"name":"订单","rows":[{"quantity":2},{"quantity":null}]}`,
		`{"name":"订单","fields":[{"name":"quantity","type":"text"}],"rows":[{"quantity":2},{"quantity":3}]}`,
	} {
		completion.Store(output)
		authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor, selected, http.StatusBadGateway)
		assertNoAIData(t, database)
	}
	const freeform = `{"project_id":"project-a","count":2,"prompt":"密码: 至少 8 位，备注示例为普通订单。"}`
	completion.Store(`{"name":"自由模式","fields":[{"name":"备注","type":"unknown"}],"rows":[{"备注":"普通订单"},{"备注":""}]}`)
	authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor, freeform, http.StatusBadGateway)
	assertNoAIData(t, database)
	completion.Store(`{"name":"自由模式","fields":[{"name":"密码","type":"text","example":"discard"},{"name":"备注","type":"text"}],"rows":[{"密码":"model-generated","备注":"普通订单"},{"密码":"","备注":""}]}`)
	if _, err := database.DB.Exec(`CREATE TRIGGER reject_second_ai_record BEFORE INSERT ON test_data_records
		WHEN NEW.row_no = 2 BEGIN SELECT RAISE(ABORT, 'test write failure'); END`); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor, freeform, http.StatusInternalServerError)
	assertNoAIData(t, database)
	if _, err := database.DB.Exec("DROP TRIGGER reject_second_ai_record"); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "POST", aiDataPath, "tenant-a", editor, freeform, http.StatusCreated)
}

func TestAIDataSetMigrationPreservesRuleRegeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedMCPData(t, database)
	_, setupErr := database.DB.Exec(`ALTER TABLE test_data_sets DROP COLUMN generation_source;
		INSERT INTO test_data_sets (id, tenant_id, project_id, name, fields_json) VALUES
		('legacy-data','tenant-a','project-a','Legacy','[{"name":"quantity","type":"number"}]')`)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	database, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	handler := New(database).Routes()
	list := serveTenant(handler, "GET", "/api/test-data/datasets", "tenant-a", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"generation_source":"rule"`) {
		t.Fatalf("legacy dataset list = %d %s", list.Code, list.Body)
	}
	regenerate := serveTenant(handler, "POST", "/api/test-data/datasets/legacy-data/generate", "tenant-a", []byte(`{"count":2}`))
	if regenerate.Code != http.StatusOK || !strings.Contains(regenerate.Body.String(), `"record_count":2`) {
		t.Fatalf("legacy regeneration = %d %s", regenerate.Code, regenerate.Body)
	}
}

func aiDataFixture(t *testing.T) (*store.Store, http.Handler, string, string) {
	t.Helper()
	database := openTestStore(t)
	seedMCPData(t, database)
	if _, err := database.DB.Exec(`INSERT INTO projects (id, tenant_id, name, base_url) VALUES ('project-other','tenant-a','Other','https://other.test');
		INSERT INTO environments (id, tenant_id, project_id, name, base_url) VALUES
		('env-ai','tenant-a','project-a','Synthetic QA','https://private-environment.test'),
		('foreign-env','tenant-b','project-b','Foreign','https://foreign.test'),
		('other-project-env','tenant-a','project-other','Other','https://other.test')`); err != nil {
		t.Fatal(err)
	}
	for _, account := range []struct{ name, role string }{{"testdata-editor", "editor"}, {"testdata-viewer", "viewer"}} {
		if err := ProvisionUser(t.Context(), database, account.name, fixturePassword, "tenant-a", "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(database, AuthOptions{}).Routes()
	return database, handler, authLogin(t, handler, "testdata-editor"), authLogin(t, handler, "testdata-viewer")
}

func assertNoAIData(t *testing.T, database *store.Store) {
	t.Helper()
	var sets, records int
	if err := database.DB.QueryRow(`SELECT (SELECT count(*) FROM test_data_sets), (SELECT count(*) FROM test_data_records)`).Scan(&sets, &records); err != nil || sets != 0 || records != 0 {
		t.Fatalf("failed generation left %d datasets and %d records (%v)", sets, records, err)
	}
}
