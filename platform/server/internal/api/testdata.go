package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type dataField struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Sensitive bool   `json:"sensitive"`
	Example   string `json:"example"`
}

type dataSetInput struct {
	ProjectID     string      `json:"project_id"`
	EnvironmentID string      `json:"environment_id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Fields        []dataField `json:"fields"`
}

func (s *Server) listDataSets(w http.ResponseWriter, r *http.Request, tid string) {
	rows, err := s.st.DB.Query(`SELECT d.*, p.name AS project_name
		FROM test_data_sets d JOIN projects p ON p.id = d.project_id
		WHERE d.tenant_id = ? ORDER BY d.updated_at DESC`, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, _ := rowsToMaps(rows)
	for _, item := range out {
		item["fields"] = parseFields(item["fields_json"])
		delete(item, "fields_json")
	}
	write(w, http.StatusOK, out)
}

func (s *Server) createDataSet(w http.ResponseWriter, r *http.Request, tid string) {
	var b dataSetInput
	if err := decode(r, &b); err != nil || b.ProjectID == "" || b.Name == "" || len(b.Fields) == 0 {
		fail(w, http.StatusBadRequest, "project_id、name、fields 必填")
		return
	}
	if err := validateFields(b.Fields); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	var project int
	if err := s.st.DB.QueryRow("SELECT 1 FROM projects WHERE id = ? AND tenant_id = ?", b.ProjectID, tid).Scan(&project); err != nil {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	if b.EnvironmentID != "" {
		if err := s.st.DB.QueryRow("SELECT 1 FROM environments WHERE id = ? AND tenant_id = ?", b.EnvironmentID, tid).Scan(&project); err != nil {
			fail(w, http.StatusNotFound, "环境不存在")
			return
		}
	}
	fields := mustJSON(b.Fields)
	did := id()
	_, err := s.st.DB.Exec(`INSERT INTO test_data_sets
		(id, tenant_id, project_id, environment_id, name, description, fields_json)
		VALUES (?,?,?,?,?,?,?)`, did, tid, b.ProjectID, b.EnvironmentID, b.Name, b.Description, fields)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"id": did})
}

func (s *Server) generateData(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		Count int `json:"count"`
	}
	if err := decode(r, &b); err != nil || b.Count < 1 || b.Count > 10000 {
		fail(w, http.StatusBadRequest, "count 必须在 1 到 10000 之间")
		return
	}
	did := r.PathValue("id")
	var fieldsJSON string
	err := s.st.DB.QueryRow("SELECT fields_json FROM test_data_sets WHERE id = ? AND tenant_id = ?", did, tid).Scan(&fieldsJSON)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "测试数据集不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	fields := parseFields(fieldsJSON)
	tx, err := s.st.DB.Begin()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err = tx.Exec("DELETE FROM test_data_records WHERE dataset_id = ? AND tenant_id = ?", did, tid); err != nil {
		tx.Rollback()
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for row := 1; row <= b.Count; row++ {
		values := make(map[string]string, len(fields))
		for _, field := range fields {
			values[field.Name] = generatedValue(field, row)
		}
		if _, err = tx.Exec(`INSERT INTO test_data_records
			(id, tenant_id, dataset_id, row_no, values_json) VALUES (?,?,?,?,?)`,
			id(), tid, did, row, mustJSON(values)); err != nil {
			tx.Rollback()
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if _, err = tx.Exec("UPDATE test_data_sets SET record_count = ?, updated_at = datetime('now') WHERE id = ? AND tenant_id = ?", b.Count, did, tid); err != nil {
		tx.Rollback()
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"dataset_id": did, "record_count": b.Count})
}

func (s *Server) listDataRecords(w http.ResponseWriter, r *http.Request, tid string) {
	did := r.PathValue("id")
	var fieldsJSON string
	err := s.st.DB.QueryRow("SELECT fields_json FROM test_data_sets WHERE id = ? AND tenant_id = ?", did, tid).Scan(&fieldsJSON)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "测试数据集不存在")
		return
	}
	rows, err := s.st.DB.Query(`SELECT id, row_no, values_json, created_at
		FROM test_data_records WHERE dataset_id = ? AND tenant_id = ? ORDER BY row_no`, did, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, _ := rowsToMaps(rows)
	fields := parseFields(fieldsJSON)
	for _, item := range out {
		values := parseValues(item["values_json"])
		item["values"] = maskedValues(values, fields)
		delete(item, "values_json")
	}
	write(w, http.StatusOK, out)
}

func (s *Server) rawDataRecords(w http.ResponseWriter, r *http.Request, tid string) {
	did := r.PathValue("id")
	var exists int
	if err := s.st.DB.QueryRow("SELECT 1 FROM test_data_sets WHERE id = ? AND tenant_id = ?", did, tid).Scan(&exists); err != nil {
		fail(w, http.StatusNotFound, "测试数据集不存在")
		return
	}
	rows, err := s.st.DB.Query(`SELECT id, row_no, values_json
		FROM test_data_records WHERE dataset_id = ? AND tenant_id = ? ORDER BY row_no`, did, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, _ := rowsToMaps(rows)
	for _, item := range out {
		item["values"] = parseValues(item["values_json"])
		delete(item, "values_json")
	}
	write(w, http.StatusOK, out)
}

func validateFields(fields []dataField) error {
	seen := map[string]bool{}
	valid := map[string]bool{"name": true, "email": true, "phone": true, "number": true, "date": true, "uuid": true, "text": true, "password": true}
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" || seen[field.Name] {
			return fmt.Errorf("字段名不能为空且不能重复")
		}
		if !valid[field.Type] {
			return fmt.Errorf("不支持的数据类型: %s", field.Type)
		}
		seen[field.Name] = true
	}
	return nil
}

func parseFields(v any) []dataField {
	var fields []dataField
	_ = json.Unmarshal([]byte(fmt.Sprint(v)), &fields)
	return fields
}

func parseValues(v any) map[string]string {
	values := map[string]string{}
	_ = json.Unmarshal([]byte(fmt.Sprint(v)), &values)
	return values
}

func maskedValues(values map[string]string, fields []dataField) map[string]string {
	masked := make(map[string]string, len(values))
	for _, field := range fields {
		value := values[field.Name]
		if field.Sensitive {
			masked[field.Name] = "••••••"
		} else {
			masked[field.Name] = value
		}
	}
	return masked
}

func generatedValue(field dataField, row int) string {
	if field.Example != "" {
		return field.Example + "-" + strconv.Itoa(row)
	}
	suffix := fmt.Sprintf("%04d", row)
	switch field.Type {
	case "name":
		return "测试用户" + suffix
	case "email":
		return "test" + suffix + "@example.test"
	case "phone":
		return "1380000" + suffix
	case "number":
		return strconv.Itoa(row)
	case "date":
		return time.Now().AddDate(0, 0, row).Format("2006-01-02")
	case "uuid":
		return id() + "-" + strconv.Itoa(row)
	case "password":
		return "Test@" + suffix + "!"
	default:
		return "测试数据" + suffix
	}
}
