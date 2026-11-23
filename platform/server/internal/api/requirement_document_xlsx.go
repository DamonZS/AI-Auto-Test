package api

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"platform/server/internal/store"
)

const requirementXLSXMaxXML = 32 << 20
const requirementXLSXColumns = 16384

type requirementXLSXWorkbook struct {
	XMLName xml.Name `xml:"workbook"`
	Sheets  []struct {
		Name  string `xml:"name,attr"`
		ID    string `xml:"id,attr"`
		State string `xml:"state,attr"`
	} `xml:"sheets>sheet"`
}

type requirementXLSXRelationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
	Mode   string `xml:"TargetMode,attr"`
}

type requirementXLSXText struct {
	Plain string   `xml:"t"`
	Runs  []string `xml:"r>t"`
}

func (text requirementXLSXText) value() string {
	return text.Plain + strings.Join(text.Runs, "")
}

type requirementXLSXCell struct {
	Reference string              `xml:"r,attr"`
	Type      string              `xml:"t,attr"`
	Value     string              `xml:"v"`
	Inline    requirementXLSXText `xml:"is"`
}

type requirementXLSXSheet struct {
	XMLName xml.Name `xml:"worksheet"`
	Columns []struct {
		First  int  `xml:"min,attr"`
		Last   int  `xml:"max,attr"`
		Hidden bool `xml:"hidden,attr"`
	} `xml:"cols>col"`
	Rows []struct {
		Number int                   `xml:"r,attr"`
		Hidden bool                  `xml:"hidden,attr"`
		Cells  []requirementXLSXCell `xml:"c"`
	} `xml:"sheetData>row"`
}

type requirementXLSXRow struct {
	number int
	cells  map[int]string
	last   int
}

type requirementXLSXTable struct {
	name string
	rows []requirementXLSXRow
}

type requirementXLSXPackage struct {
	files     map[string]*zip.File
	remaining uint64
}

func parseRequirementXLSX(data []byte) ([]store.RequirementContent, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("需要有效的 XLSX 工作簿")
	}
	pkg := requirementXLSXPackage{files: make(map[string]*zip.File), remaining: requirementXLSXMaxXML}
	for _, file := range archive.File {
		pkg.files[file.Name] = file
	}
	tables, err := pkg.visibleTables()
	if err != nil {
		return nil, err
	}
	return requirementXLSXContents(tables)
}

func (pkg *requirementXLSXPackage) readXML(name string, target any) error {
	file := pkg.files[name]
	if file == nil {
		return errors.New("XLSX 工作簿缺少必要部件")
	}
	if file.UncompressedSize64 > pkg.remaining {
		return errors.New("XLSX 工作簿解压后超过大小限制")
	}
	pkg.remaining -= file.UncompressedSize64
	reader, err := file.Open()
	if err != nil {
		return fmt.Errorf("无法打开 XLSX 部件：%w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("无法读取 XLSX 部件：%w", err)
	}
	if err := xml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("XLSX XML 格式错误：%w", err)
	}
	return nil
}

func (pkg *requirementXLSXPackage) visibleTables() ([]requirementXLSXTable, error) {
	var workbook requirementXLSXWorkbook
	if err := pkg.readXML("xl/workbook.xml", &workbook); err != nil {
		return nil, err
	}
	var relationships struct {
		XMLName xml.Name                      `xml:"Relationships"`
		Entries []requirementXLSXRelationship `xml:"Relationship"`
	}
	if err := pkg.readXML("xl/_rels/workbook.xml.rels", &relationships); err != nil {
		return nil, err
	}
	byID := make(map[string]requirementXLSXRelationship)
	var shared []string
	for _, relation := range relationships.Entries {
		if relation.Mode == "External" {
			continue
		}
		byID[relation.ID] = relation
		if strings.HasSuffix(relation.Type, "/sharedStrings") {
			var stringsPart struct {
				XMLName xml.Name              `xml:"sst"`
				Values  []requirementXLSXText `xml:"si"`
			}
			if err := pkg.readXML(relation.partName(), &stringsPart); err != nil {
				return nil, err
			}
			for _, value := range stringsPart.Values {
				shared = append(shared, value.value())
			}
		}
	}
	var tables []requirementXLSXTable
	for _, sheet := range workbook.Sheets {
		if sheet.State == "hidden" || sheet.State == "veryHidden" {
			continue
		}
		relation, ok := byID[sheet.ID]
		if !ok {
			return nil, errors.New("XLSX 工作簿缺少工作表关系")
		}
		if !strings.HasSuffix(relation.Type, "/worksheet") {
			continue
		}
		var worksheet requirementXLSXSheet
		if err := pkg.readXML(relation.partName(), &worksheet); err != nil {
			return nil, fmt.Errorf("工作表 %q：%w", sheet.Name, err)
		}
		rows, err := worksheet.visibleRows(shared)
		if err != nil {
			return nil, fmt.Errorf("工作表 %q：%w", sheet.Name, err)
		}
		if len(rows) > 0 {
			tables = append(tables, requirementXLSXTable{name: sheet.Name, rows: rows})
		}
	}
	return tables, nil
}

func (relation requirementXLSXRelationship) partName() string {
	if strings.HasPrefix(relation.Target, "/") {
		return strings.TrimPrefix(relation.Target, "/")
	}
	return path.Join("xl", relation.Target)
}

func (sheet requirementXLSXSheet) visibleRows(shared []string) ([]requirementXLSXRow, error) {
	hidden := make([]bool, requirementXLSXColumns+1)
	for _, column := range sheet.Columns {
		if column.Hidden && column.First >= 1 && column.Last <= requirementXLSXColumns {
			for index := column.First; index <= column.Last; index++ {
				hidden[index] = true
			}
		}
	}
	positions := make([]int, len(hidden))
	position := 0
	for column := 1; column < len(hidden); column++ {
		if !hidden[column] {
			position++
			positions[column] = position
		}
	}
	var rows []requirementXLSXRow
	rowNumber := 0
	for _, row := range sheet.Rows {
		rowNumber++
		if row.Number > 0 {
			rowNumber = row.Number
		}
		if row.Hidden {
			continue
		}
		visible := requirementXLSXRow{number: rowNumber, cells: make(map[int]string)}
		column := 0
		for _, cell := range row.Cells {
			column++
			if cell.Reference != "" {
				column = requirementXLSXColumn(cell.Reference)
			}
			if column < 1 || column > requirementXLSXColumns {
				return nil, fmt.Errorf("第 %d 行单元格列无效", rowNumber)
			}
			position := positions[column]
			if position == 0 {
				continue
			}
			value, err := cell.text(shared)
			if err != nil {
				return nil, fmt.Errorf("第 %d 行：%w", rowNumber, err)
			}
			if strings.TrimSpace(value) != "" {
				visible.cells[position] = value
				visible.last = max(visible.last, position)
			}
		}
		if visible.last > 0 {
			rows = append(rows, visible)
		}
	}
	return rows, nil
}

func requirementXLSXColumn(reference string) int {
	column := 0
	for _, letter := range reference {
		if letter < 'A' || letter > 'Z' {
			break
		}
		column = column*26 + int(letter-'A'+1)
		if column > requirementXLSXColumns {
			break
		}
	}
	return column
}

func (cell requirementXLSXCell) text(shared []string) (string, error) {
	switch cell.Type {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err != nil || index < 0 || index >= len(shared) {
			return "", errors.New("共享字符串索引无效")
		}
		return shared[index], nil
	case "inlineStr":
		return cell.Inline.value(), nil
	case "b":
		if cell.Value == "1" {
			return "TRUE", nil
		}
		if cell.Value == "0" {
			return "FALSE", nil
		}
	}
	return cell.Value, nil
}

func requirementXLSXHeader(row requirementXLSXRow) (code, title, content int) {
	for column := 1; column <= row.last; column++ {
		switch strings.ToLower(strings.TrimSpace(row.cells[column])) {
		case "code", "编号", "编码", "需求编号", "需求编码":
			if code == 0 {
				code = column
			}
		case "title", "标题", "名称", "需求标题", "需求名称":
			if title == 0 {
				title = column
			}
		case "content", "内容", "正文", "描述", "需求内容", "需求描述", "需求正文":
			if content == 0 {
				content = column
			}
		}
	}
	return code, title, content
}

func requirementXLSXContents(tables []requirementXLSXTable) ([]store.RequirementContent, error) {
	var items []store.RequirementContent
	structured := false
	size := 0
	for _, table := range tables {
		code, title, content := requirementXLSXHeader(table.rows[0])
		if code == 0 || title == 0 || content == 0 {
			continue
		}
		structured = true
		for _, row := range table.rows[1:] {
			item := store.RequirementContent{
				Code: strings.TrimSpace(row.cells[code]), Title: strings.TrimSpace(row.cells[title]), Content: row.cells[content],
			}
			if item.Code == "" && item.Title == "" && item.Content == "" {
				continue
			}
			if item.Title == "" {
				return nil, fmt.Errorf("工作表 %q 第 %d 行标题不能为空", table.name, row.number)
			}
			size += len(item.Code) + len(item.Title) + len(item.Content)
			if size > requirementImportMaxBytes {
				return nil, errors.New("XLSX 导入文本不能超过 1 MiB")
			}
			items = append(items, item)
		}
	}
	if len(items) > 0 {
		return items, nil
	}
	if structured || len(tables) == 0 {
		return nil, errors.New("XLSX 工作簿没有可导入的需求内容")
	}
	return requirementXLSXDocument(tables)
}

func requirementXLSXDocument(tables []requirementXLSXTable) ([]store.RequirementContent, error) {
	var document strings.Builder
	for _, table := range tables {
		if document.Len() > 0 {
			document.WriteString("\n\n")
		}
		document.WriteString(table.name)
		for _, row := range table.rows {
			document.WriteByte('\n')
			for column := 1; column <= row.last; column++ {
				if column > 1 {
					document.WriteByte('\t')
				}
				value := row.cells[column]
				if document.Len()+len(value) > requirementImportMaxBytes {
					return nil, errors.New("XLSX 导入文本不能超过 1 MiB")
				}
				document.WriteString(value)
			}
		}
	}
	return []store.RequirementContent{{Content: document.String()}}, nil
}
