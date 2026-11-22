package api

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type workbenchField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type workbenchCheckItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type workbenchTemplate struct {
	Kind   string               `json:"kind"`
	Label  string               `json:"label"`
	Fields []workbenchField     `json:"fields"`
	Checks []workbenchCheckItem `json:"checks"`
}

var workbenchCatalog = []workbenchTemplate{
	{"test_application", "测试申请", []workbenchField{
		{"applicant", "申请人", "text"},
		{"scope", "测试范围", "textarea"},
		{"change_summary", "需求或变更说明", "textarea"},
		{"start_date", "期望开始日期", "date"},
		{"end_date", "期望完成日期", "date"},
	}, []workbenchCheckItem{}},
	{"test_plan", "测试计划", []workbenchField{
		{"scope", "目标与范围", "textarea"},
		{"environment", "测试环境", "text"},
		{"owner", "负责人", "text"},
		{"start_date", "计划开始日期", "date"},
		{"end_date", "计划完成日期", "date"},
		{"milestones", "策略与里程碑", "textarea"},
	}, []workbenchCheckItem{}},
	{"entrance_checklist", "准入检查单", []workbenchField{}, []workbenchCheckItem{
		{"application_approved", "测试申请已批准"},
		{"requirements_reviewed", "需求资料经评审"},
		{"developer_tests_passed", "开发自测通过"},
		{"environment_data_ready", "环境及数据就绪"},
		{"resources_ready", "测试资源就绪"},
		{"cases_reviewed", "用例评审通过"},
		{"smoke_passed", "冒烟通过"},
	}},
	{"peer_review", "同行评审记录", []workbenchField{
		{"subject", "评审对象", "text"},
		{"participants", "参会人", "text"},
		{"review_date", "评审日期", "date"},
		{"conclusion", "评审结论", "textarea"},
		{"follow_up", "问题与整改安排", "textarea"},
	}, []workbenchCheckItem{}},
	{"daily_report", "测试日报", []workbenchField{
		{"report_date", "报告日期", "date"},
		{"progress", "进展", "textarea"},
		{"blockers", "阻塞问题", "textarea"},
		{"next_steps", "下一步安排", "textarea"},
	}, []workbenchCheckItem{}},
	{"exit_checklist", "准出检查单", []workbenchField{}, []workbenchCheckItem{
		{"execution_archived", "执行记录已归档"},
		{"coverage_met", "执行和覆盖已达标"},
		{"major_defects_resolved", "高严重度缺陷已处理"},
		{"regression_complete", "复测与回归完成"},
		{"residuals_confirmed", "遗留项已确认"},
		{"report_ready", "报告材料齐备"},
	}},
	{"test_report", "测试总结报告", []workbenchField{
		{"test_scope_environment", "测试范围与环境", "textarea"},
		{"residual_risks", "遗留缺陷与风险", "textarea"},
		{"conclusion", "测试结论", "textarea"},
	}, []workbenchCheckItem{}},
}

func findWorkbenchTemplate(kind string) *workbenchTemplate {
	for index := range workbenchCatalog {
		if workbenchCatalog[index].Kind == kind {
			return &workbenchCatalog[index]
		}
	}
	return nil
}

type workbenchCheck struct {
	Result   string `json:"result"`
	Evidence string `json:"evidence"`
}

type workbenchContent struct {
	Title     string                     `json:"title"`
	RiskLevel string                     `json:"risk_level"`
	Fields    map[string]string          `json:"fields"`
	Checks    map[string]*workbenchCheck `json:"checks,omitempty"`
}

func (content *workbenchContent) validate(tmpl *workbenchTemplate) error {
	content.Title = strings.TrimSpace(content.Title)
	content.RiskLevel = strings.ToUpper(strings.TrimSpace(content.RiskLevel))
	if content.Title == "" || content.Fields == nil {
		return fmt.Errorf("title 与 fields 必填")
	}
	switch content.RiskLevel {
	case "", "P0", "P1", "P2", "P3":
	default:
		return fmt.Errorf("risk_level 无效")
	}
	for key, value := range content.Fields {
		index := slices.IndexFunc(tmpl.Fields, func(field workbenchField) bool { return field.Key == key })
		if index < 0 {
			return fmt.Errorf("未知模板字段 %q", key)
		}
		value = strings.TrimSpace(value)
		if value != "" && tmpl.Fields[index].Type == "date" {
			if date, err := time.Parse(time.DateOnly, value); err != nil || date.Format(time.DateOnly) != value {
				return fmt.Errorf("%s 应使用 YYYY-MM-DD 日期格式", tmpl.Fields[index].Label)
			}
		}
		content.Fields[key] = value
	}
	for key, check := range content.Checks {
		if !slices.ContainsFunc(tmpl.Checks, func(item workbenchCheckItem) bool { return item.Key == key }) {
			return fmt.Errorf("未知检查项 %q", key)
		}
		if check == nil || (check.Result != "" && check.Result != "pass" && check.Result != "fail") {
			return fmt.Errorf("检查项 %q 的结果无效", key)
		}
		check.Evidence = strings.TrimSpace(check.Evidence)
	}
	return nil
}

func (content workbenchContent) missingSections(tmpl *workbenchTemplate) []string {
	var missing []string
	for _, field := range tmpl.Fields {
		if content.Fields[field.Key] == "" {
			missing = append(missing, field.Key)
		}
	}
	if start, end := content.Fields["start_date"], content.Fields["end_date"]; start != "" && end != "" && end < start {
		missing = append(missing, "end_date")
	}
	for _, item := range tmpl.Checks {
		if check := content.Checks[item.Key]; check == nil || check.Result != "pass" || check.Evidence == "" {
			missing = append(missing, item.Key)
		}
	}
	return missing
}
