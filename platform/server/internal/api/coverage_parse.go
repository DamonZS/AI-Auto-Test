package api

import (
	"encoding/xml"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"platform/server/internal/store"
)

var (
	errCoverageFormat = errors.New("invalid coverage report")
	goCoverageBlock   = regexp.MustCompile(`^([^\r\n]+:[1-9][0-9]*\.[1-9][0-9]*,[1-9][0-9]*\.[1-9][0-9]*) ([0-9]+) ([0-9]+)$`)
	coberturaBranch   = regexp.MustCompile(`^\s*[0-9]+(?:\.[0-9]+)?%\s*\(([0-9]+)/([0-9]+)\)\s*$`)
)

func parseGoCoverage(content string) (store.CoverageCount, error) {
	var result store.CoverageCount
	header, body, _ := strings.Cut(content, "\n")
	switch strings.TrimSuffix(header, "\r") {
	case "mode: set", "mode: count", "mode: atomic":
	default:
		return result, errCoverageFormat
	}
	type block struct {
		statements int64
		hit        bool
	}
	blocks := map[string]block{}
	for line := range strings.SplitSeq(strings.TrimSuffix(body, "\n"), "\n") {
		parts := goCoverageBlock.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		if parts == nil {
			return result, errCoverageFormat
		}
		statements, statementErr := strconv.ParseInt(parts[2], 10, 64)
		hits, hitErr := strconv.ParseInt(parts[3], 10, 64)
		previous, duplicate := blocks[parts[1]]
		if statementErr != nil || hitErr != nil || (duplicate && previous.statements != statements) {
			return result, errCoverageFormat
		}
		blocks[parts[1]] = block{statements: statements, hit: previous.hit || hits > 0}
	}
	for _, block := range blocks {
		if block.statements > math.MaxInt64-result.Total {
			return result, errCoverageFormat
		}
		result.Total += block.statements
		if block.hit {
			result.Covered += block.statements
		}
	}
	return result, nil
}

type coberturaReport struct {
	XMLName xml.Name         `xml:"coverage"`
	Classes []coberturaClass `xml:"packages>package>classes>class"`
}

type coberturaClass struct {
	File  string          `xml:"filename,attr"`
	Lines []coberturaLine `xml:"lines>line"`
}

type coberturaLine struct {
	Number    int64  `xml:"number,attr"`
	Hits      *int64 `xml:"hits,attr"`
	Branch    string `xml:"branch,attr"`
	Condition string `xml:"condition-coverage,attr"`
}

func parseCoberturaCoverage(content string) (store.CoverageCount, store.CoverageCount, error) {
	var lines, branches store.CoverageCount
	report, err := decodeCoberturaCoverage(content)
	if err != nil {
		return lines, branches, err
	}
	type location struct {
		file   string
		number int64
	}
	type lineCounts struct {
		hit      bool
		branches store.CoverageCount
	}
	unique := map[location]lineCounts{}
	for _, class := range report.Classes {
		for _, line := range class.Lines {
			if strings.TrimSpace(class.File) == "" || line.Number < 1 || line.Hits == nil || *line.Hits < 0 {
				return lines, branches, errCoverageFormat
			}
			counts, err := line.branchCounts()
			key := location{class.File, line.Number}
			previous, duplicate := unique[key]
			if err != nil || (duplicate && previous.branches != counts) {
				return lines, branches, errCoverageFormat
			}
			unique[key] = lineCounts{hit: previous.hit || *line.Hits > 0, branches: counts}
		}
	}
	if len(unique) == 0 {
		return lines, branches, errCoverageFormat
	}
	lines.Total = int64(len(unique))
	for _, line := range unique {
		if line.branches.Total > math.MaxInt64-branches.Total {
			return lines, branches, errCoverageFormat
		}
		branches.Total += line.branches.Total
		branches.Covered += line.branches.Covered
		if line.hit {
			lines.Covered++
		}
	}
	return lines, branches, nil
}

func decodeCoberturaCoverage(content string) (coberturaReport, error) {
	var report coberturaReport
	decoder := xml.NewDecoder(strings.NewReader(strings.TrimPrefix(content, "\ufeff")))
	for {
		token, err := decoder.Token()
		if err == io.EOF && report.XMLName.Local != "" {
			return report, nil
		}
		if err != nil {
			return report, errCoverageFormat
		}
		switch token := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return report, errCoverageFormat
			}
		case xml.StartElement:
			if report.XMLName.Local != "" || decoder.DecodeElement(&report, &token) != nil {
				return report, errCoverageFormat
			}
		case xml.Directive:
			if report.XMLName.Local != "" {
				return report, errCoverageFormat
			}
		}
	}
}

func (line coberturaLine) branchCounts() (store.CoverageCount, error) {
	var counts store.CoverageCount
	switch line.Branch {
	case "", "false":
		return counts, nil
	case "true":
		parts := coberturaBranch.FindStringSubmatch(line.Condition)
		if parts != nil {
			covered, coveredErr := strconv.ParseInt(parts[1], 10, 64)
			total, totalErr := strconv.ParseInt(parts[2], 10, 64)
			if coveredErr == nil && totalErr == nil && total > 0 && covered <= total {
				return store.CoverageCount{Covered: covered, Total: total}, nil
			}
		}
	}
	return counts, errCoverageFormat
}
