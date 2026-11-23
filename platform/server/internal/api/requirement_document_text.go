package api

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

func extractRequirementDocumentText(ext string, data []byte) (string, error) {
	switch strings.ToLower(ext) {
	case ".pdf":
		return extractRequirementPDFText(data)
	case ".docx":
		return extractRequirementDOCXText(data)
	default:
		return "", errors.New("仅支持 PDF 和 DOCX 文档")
	}
}

func extractRequirementPDFText(data []byte) (text string, err error) {
	defer func() {
		if recover() != nil {
			text = ""
			err = errors.New("PDF 结构损坏，无法提取文字")
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if errors.Is(err, pdf.ErrInvalidPassword) {
		return "", errors.New("PDF 受密码保护，请上传解密后的文件")
	}
	if err != nil {
		return "", fmt.Errorf("无法读取 PDF：%w", err)
	}
	var output strings.Builder
	pageCount := reader.NumPage()
	for pageNumber := 1; pageNumber <= pageCount; pageNumber++ {
		page := reader.Page(pageNumber)
		if page.V.IsNull() {
			return "", fmt.Errorf("PDF 缺少第 %d 页", pageNumber)
		}
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("无法提取 PDF 第 %d 页文字：%w", pageNumber, err)
		}
		pageText = strings.TrimSpace(strings.ToValidUTF8(pageText, "�"))
		if pageText == "" {
			continue
		}
		if output.Len() > 0 {
			output.WriteString("\n\n")
		}
		if output.Len()+len(pageText) > requirementImportMaxBytes {
			return "", errors.New("提取的需求文本超过 1 MiB，请拆分文件")
		}
		output.WriteString(pageText)
	}
	if output.Len() == 0 {
		return "", errors.New("PDF 没有可提取的文字层，扫描件请先进行 OCR")
	}
	return output.String(), nil
}

func extractRequirementDOCXText(data []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("无法打开 DOCX，请上传有效且未加密的 Word 文档")
	}
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		if part.UncompressedSize64 > 16<<20 {
			return "", errors.New("DOCX 正文 XML 超过 16 MiB，请拆分文件")
		}
		body, err := part.Open()
		if err != nil {
			return "", fmt.Errorf("无法读取 DOCX 正文：%w", err)
		}
		defer body.Close()
		return readRequirementWordBody(body)
	}
	return "", errors.New("DOCX 缺少正文文件 word/document.xml")
}

func readRequirementWordBody(body io.Reader) (string, error) {
	const transitional = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	const strict = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	decoder := xml.NewDecoder(body)
	var output bytes.Buffer
	var namespace string
	var depth, runDepth int
	var inBody, sawBody bool
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("DOCX 正文 XML 无法读取：%w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 && element.Name.Local == "document" && (element.Name.Space == transitional || element.Name.Space == strict) {
				namespace = element.Name.Space
			}
			if namespace == "" || element.Name.Space != namespace {
				continue
			}
			if depth == 2 && element.Name.Local == "body" {
				inBody, sawBody = true, true
			}
			if !inBody {
				continue
			}
			switch element.Name.Local {
			case "r":
				runDepth++
			case "t", "tab", "br", "cr":
				if runDepth == 0 {
					continue
				}
				switch element.Name.Local {
				case "t":
					var text string
					if err := decoder.DecodeElement(&text, &element); err != nil {
						return "", fmt.Errorf("DOCX 正文 XML 无法读取：%w", err)
					}
					depth--
					output.WriteString(text)
				case "tab":
					output.WriteByte('\t')
				default:
					output.WriteByte('\n')
				}
			}
		case xml.EndElement:
			if inBody && element.Name.Space == namespace {
				switch element.Name.Local {
				case "body":
					inBody = depth != 2
				case "r":
					runDepth--
				case "p":
					output.WriteByte('\n')
				case "tc":
					replaceRequirementTextSeparator(&output, '\n', '\t')
				case "tr":
					replaceRequirementTextSeparator(&output, '\t', '\n')
				}
			}
			depth--
		}
		if output.Len() > requirementImportMaxBytes {
			return "", errors.New("提取的需求文本超过 1 MiB，请拆分文件")
		}
	}
	if !sawBody {
		return "", errors.New("DOCX 正文 XML 缺少有效的文档正文")
	}
	text := strings.TrimSpace(output.String())
	if text == "" {
		return "", errors.New("DOCX 正文没有可提取的文字")
	}
	return text, nil
}

func replaceRequirementTextSeparator(output *bytes.Buffer, trailing, separator byte) {
	if length := output.Len(); length > 0 && output.Bytes()[length-1] == trailing {
		output.Truncate(length - 1)
	}
	output.WriteByte(separator)
}
