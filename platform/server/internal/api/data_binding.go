package api

import (
	"database/sql"
	"fmt"
)

func (s *Server) validateDataBinding(tenantID, projectID, datasetID, rowID string) error {
	if datasetID == "" && rowID == "" {
		return nil
	}
	if datasetID == "" || rowID == "" {
		return fmt.Errorf("dataset_id 与 data_row_id 必须同时提供")
	}
	var datasetProject string
	if err := s.st.DB.QueryRow(`SELECT project_id FROM test_data_sets
		WHERE id = ? AND tenant_id = ?`, datasetID, tenantID).Scan(&datasetProject); err == sql.ErrNoRows {
		return fmt.Errorf("测试数据集不存在")
	} else if err != nil {
		return err
	}
	if datasetProject != projectID {
		return fmt.Errorf("测试数据集不属于当前项目")
	}
	var one int
	if err := s.st.DB.QueryRow(`SELECT 1 FROM test_data_records
		WHERE id = ? AND dataset_id = ? AND tenant_id = ?`, rowID, datasetID, tenantID).Scan(&one); err == sql.ErrNoRows {
		return fmt.Errorf("测试数据行不存在")
	} else if err != nil {
		return err
	}
	return nil
}
