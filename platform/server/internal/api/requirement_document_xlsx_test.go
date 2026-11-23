package api

import (
	"archive/zip"
	"bytes"
	"maps"
	"reflect"
	"strings"
	"testing"

	"platform/server/internal/store"
)

const requirementXLSXTestMainNS = ` xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`

func requirementXLSXFixture(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	packageParts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
		<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
		<Default Extension="xml" ContentType="application/xml"/>
		<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
		<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
		<Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
		<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
		</Types>`,
		"_rels/.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml": `<workbook` + requirementXLSXTestMainNS + ` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
		<sheets><sheet name="需求清单" sheetId="2" r:id="rId2"/><sheet name="隐藏" sheetId="1" state="hidden" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
		<Relationship Id="rId1" Target="worksheets/sheet1.xml" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"/>
		<Relationship Id="rId2" Target="/xl/worksheets/sheet2.xml" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"/>
		<Relationship Id="rId3" Target="sharedStrings.xml" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings"/>
		</Relationships>`,
		"xl/worksheets/sheet1.xml": `<worksheet` + requirementXLSXTestMainNS + `><sheetData><row><c r="A1" t="inlineStr"><is><t>不导入</t></is></c></row></sheetData></worksheet>`,
		"xl/sharedStrings.xml":     `<sst` + requirementXLSXTestMainNS + `><si><t>需求标题</t></si><si><r><t>第一行</t></r><r><t xml:space="preserve">&#10;第二行</t></r></si></sst>`,
	}
	maps.Copy(packageParts, parts)
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, content := range packageParts {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestParseRequirementXLSXRows(t *testing.T) {
	data := requirementXLSXFixture(t, map[string]string{
		"xl/worksheets/sheet2.xml": `<worksheet` + requirementXLSXTestMainNS + `><sheetData>
		<row r="1"/>
		<row r="2"><c r="B2" t="s"><v>0</v></c><c r="D2" t="inlineStr"><is><t>  CODE </t></is></c><c r="E2" t="inlineStr"><is><t>需求内容</t></is></c></row>
		<row r="3"><c r="B3" t="inlineStr"><is><r><t>支付</t></r><r><t>验收</t></r></is></c><c r="D3" t="inlineStr"><is><t> REQ-1 </t></is></c><c r="E3" t="s"><v>1</v></c></row>
		<row r="4"><c r="B4" t="inlineStr"><is><t> </t></is></c></row>
		<row r="5"><c r="B5" t="inlineStr"><is><t> 退出 </t></is></c></row>
		</sheetData></worksheet>`,
	})
	items, err := parseRequirementXLSX(data)
	want := []store.RequirementContent{{Code: "REQ-1", Title: "支付验收", Content: "第一行\n第二行"}, {Title: "退出"}}
	if err != nil || !reflect.DeepEqual(items, want) {
		t.Fatalf("parsed = %#v (%v), want %#v", items, err, want)
	}
}

func TestParseRequirementXLSXDocument(t *testing.T) {
	data := requirementXLSXFixture(t, map[string]string{
		"xl/worksheets/sheet2.xml": `<worksheet` + requirementXLSXTestMainNS + `><cols><col min="2" max="2" hidden="1"/></cols><sheetData>
		<row r="1"><c r="A1" t="inlineStr"><is><t>项目</t></is></c><c r="B1" t="inlineStr"><is><t>隐藏列</t></is></c><c r="D1" t="inlineStr"><is><t>结果</t></is></c></row>
		<row r="3" hidden="1"><c r="A3" t="inlineStr"><is><t>隐藏行</t></is></c></row>
		<row r="4"><c r="A4" t="inlineStr"><is><t>登录</t></is></c><c r="D4"><f>1+1</f><v>2</v></c></row>
		</sheetData></worksheet>`,
	})
	items, err := parseRequirementXLSX(data)
	want := []store.RequirementContent{{Content: "需求清单\n项目\t\t结果\n登录\t\t2"}}
	if err != nil || !reflect.DeepEqual(items, want) {
		t.Fatalf("parsed = %#v (%v), want %#v", items, err, want)
	}
}

func TestParseRequirementXLSXUsesVisibleWorkbookOrder(t *testing.T) {
	data := requirementXLSXFixture(t, map[string]string{
		"xl/workbook.xml": `<workbook` + requirementXLSXTestMainNS + ` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
		<sheets><sheet name="一" sheetId="2" r:id="rId2"/><sheet name="二" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet` + requirementXLSXTestMainNS + `><sheetData>
		<row><c t="inlineStr"><is><t>二表</t></is></c><c t="b"><v>1</v></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet` + requirementXLSXTestMainNS + `><sheetData>
		<row><c r="A1" t="inlineStr"><is><t>一表</t></is></c></row></sheetData></worksheet>`,
	})
	items, err := parseRequirementXLSX(data)
	want := []store.RequirementContent{{Content: "一\n一表\n\n二\n二表\tTRUE"}}
	if err != nil || !reflect.DeepEqual(items, want) {
		t.Fatalf("parsed = %#v (%v), want %#v", items, err, want)
	}
}

func TestParseRequirementXLSXRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name  string
		rows  string
		error string
	}{
		{"legacy XLS", "", "XLSX"},
		{"empty sheet", `<row/>`, "没有可导入"},
		{"bad shared string", `<row><c r="A1" t="s"><v>22</v></c></row>`, "共享字符串"},
		{"missing title", `<row><c r="A1" t="inlineStr"><is><t>编号</t></is></c><c r="B1" t="inlineStr"><is><t>标题</t></is></c><c r="C1" t="inlineStr"><is><t>正文</t></is></c></row>
		<row r="2"><c r="A2" t="inlineStr"><is><t>REQ-2</t></is></c></row>`, "第 2 行标题不能为空"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}
			if test.rows != "" {
				data = requirementXLSXFixture(t, map[string]string{
					"xl/worksheets/sheet2.xml": `<worksheet` + requirementXLSXTestMainNS + `><sheetData>` + test.rows + `</sheetData></worksheet>`,
				})
			}
			if items, err := parseRequirementXLSX(data); items != nil || err == nil || !strings.Contains(err.Error(), test.error) {
				t.Fatalf("parsed = %#v (%v), want %s", items, err, test.error)
			}
		})
	}
}
