package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractRequirementDocumentPDF(t *testing.T) {
	text, err := extractRequirementDocumentText(".PDF", requirementTextPDF(true))
	if want := "需求测试\n\nSecond page"; err != nil || text != want {
		t.Fatalf("PDF text = %q, error = %v, want %q", text, err, want)
	}
}

func TestExtractRequirementDocumentDOCX(t *testing.T) {
	for _, namespace := range []string{
		"http://schemas.openxmlformats.org/wordprocessingml/2006/main",
		"http://purl.oclc.org/ooxml/wordprocessingml/main",
	} {
		t.Run(namespace, func(t *testing.T) {
			document := `<w:document xmlns:w="` + namespace + `"><w:body>
			 <w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="720"/></w:tabs></w:pPr><w:r><w:t>需求</w:t></w:r><w:hyperlink><w:r><w:t>说明</w:t></w:r></w:hyperlink></w:p>
			 <w:tbl>
			   <w:tr><w:tc><w:p><w:r><w:t>编号</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>摘要</w:t></w:r></w:p></w:tc></w:tr>
			   <w:tr><w:tc><w:p><w:r><w:t>REQ-1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>登入</w:t></w:r></w:p><w:p><w:r><w:t>確認</w:t></w:r></w:p></w:tc></w:tr>
			 </w:tbl>
			 <w:p><w:r><w:t>テスト</w:t><w:tab/><w:t>요구</w:t><w:br/><w:t xml:space="preserve">A &amp; B</w:t></w:r></w:p>
			 </w:body></w:document>`
			text, err := extractRequirementDocumentText(".docx", requirementTextDOCX(t, document))
			if want := "需求说明\n编号\t摘要\nREQ-1\t登入\n確認\nテスト\t요구\nA & B"; err != nil || text != want {
				t.Fatalf("DOCX text = %q, error = %v, want %q", text, err, want)
			}
		})
	}
}

func TestExtractRequirementDocumentUpload(t *testing.T) {
	database := openTestStore(t)
	seedMCPData(t, database)
	handler := New(database).Routes()
	for _, test := range []struct {
		filename string
		data     []byte
		want     string
	}{
		{"需求.pdf", requirementTextPDF(true), "需求测试\n\nSecond page"},
		{"需求.docx", requirementTextDOCX(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>中文正文</w:t></w:r></w:p></w:body></w:document>`), "中文正文"},
	} {
		t.Run(test.filename, func(t *testing.T) {
			var upload bytes.Buffer
			writer := multipart.NewWriter(&upload)
			part, err := writer.CreateFormFile("file", test.filename)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(test.data); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/requirements/documents?project_id=project-a&preview=1", &upload)
			request.Header.Set(tenantHeader, "tenant-a")
			request.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var preview struct {
				Items []struct{ Title, Content string }
			}
			if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil || response.Code != http.StatusOK || len(preview.Items) != 1 || preview.Items[0].Title != "需求" || preview.Items[0].Content != test.want {
				t.Fatalf("preview = %d %s (%v)", response.Code, response.Body, err)
			}
		})
	}
}

func TestExtractRequirementDocumentErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		ext  string
		data []byte
		want string
	}{
		{"image only PDF", ".pdf", requirementTextPDF(false), "OCR"},
		{"broken PDF", ".pdf", []byte("%PDF-1.7\npartial"), "PDF"},
		{"broken Word package", ".docx", []byte("not a ZIP"), "DOCX"},
		{"missing Word body part", ".docx", requirementTextDOCX(t, ""), "word/document.xml"},
		{"broken Word XML", ".docx", requirementTextDOCX(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`), "正文 XML"},
		{"blank Word document", ".docx", requirementTextDOCX(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p/></w:body></w:document>`), "没有可提取"},
		{"oversized Word text", ".docx", requirementTextDOCX(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>`+strings.Repeat("文", requirementImportMaxBytes/3+1)+`</w:t></w:r></w:p></w:body></w:document>`), "文本超过 1 MiB"},
		{"unsupported extension", ".doc", nil, "DOCX"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if text, err := extractRequirementDocumentText(test.ext, test.data); text != "" || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("text = %q, error = %v, want error containing %q", text, err, test.want)
			}
		})
	}
}

func requirementTextDOCX(t *testing.T, document string) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
	}
	if document != "" {
		parts["word/document.xml"] = document
	}
	for name, content := range parts {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func requirementTextPDF(withText bool) []byte {
	stream := func(dictionary, content string) string {
		return fmt.Sprintf("<< /Length %d %s >>\nstream\n%s\nendstream", len(content), dictionary, content)
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 8 0 R] /Count 2 /MediaBox [0 0 300 200] >>",
		"<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 5 0 R >> /XObject << /Im1 11 0 R >> >> /Contents 4 0 R >>",
		stream("", "BT /F1 12 Tf 20 100 Td <0001000200030004> Tj ET"),
		"<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
		"<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 0 >> >>",
		stream("", `/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /FixtureUnicode def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
4 beginbfchar
<0001> <9700>
<0002> <6C42>
<0003> <6D4B>
<0004> <8BD5>
endbfchar
endcmap
CMapName currentdict /CMap defineresource pop
end
end`),
		"<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 10 0 R >> /XObject << /Im1 11 0 R >> >> /Contents 9 0 R >>",
		stream("", "BT /F1 12 Tf 20 100 Td (Second page) Tj ET"),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		stream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8", "\x80"),
	}
	if !withText {
		objects[3] = stream("", "q 300 0 0 200 0 0 cm /Im1 Do Q")
		objects[8] = objects[3]
	}
	var data bytes.Buffer
	data.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for index, object := range objects {
		offsets[index] = data.Len()
		fmt.Fprintf(&data, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := data.Len()
	fmt.Fprintf(&data, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&data, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&data, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return data.Bytes()
}
