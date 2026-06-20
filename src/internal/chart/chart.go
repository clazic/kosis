// Package chart provides chart rendering for KOSIS data.
package chart

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// ChartType represents the type of chart.
type ChartType string

const (
	Line ChartType = "line"
	Bar  ChartType = "bar"
	Pie  ChartType = "pie"
)

// ChartFormat represents the output format.
type ChartFormat string

const (
	FormatTerminal ChartFormat = "terminal"
	FormatPNG      ChartFormat = "png"
	FormatSVG      ChartFormat = "svg"
	FormatPDF      ChartFormat = "pdf"
	FormatHTML     ChartFormat = "html"
	FormatExcel    ChartFormat = "excel"
	FormatMermaid   ChartFormat = "mermaid"
	FormatTemplate  ChartFormat = "template"
)

// Options contains chart rendering options.
type Options struct {
	Type   ChartType
	Format ChartFormat
	Title  string
	Width  int
	Height int
	Output   string // output file path
	Open     bool   // open after creation
	XLabel   string // x-axis label
	YLabel   string // y-axis label
	Template string // template file path (for FormatTemplate)
	Subtitle string // subtitle text
	Source   string // source text
	Note     string // note text
}

// Series represents a data series for charting.
type Series struct {
	Name   string
	Labels []string  // x-axis labels (e.g., time periods)
	Values []float64 // y-axis values
}

// ParseChartType parses a string to ChartType.
func ParseChartType(s string) (ChartType, error) {
	switch strings.ToLower(s) {
	case "line":
		return Line, nil
	case "bar":
		return Bar, nil
	case "pie":
		return Pie, nil
	default:
		return "", fmt.Errorf("알 수 없는 차트 타입: %s (line, bar, pie 중 선택)", s)
	}
}

// ParseChartFormat parses a string to ChartFormat.
func ParseChartFormat(s string) (ChartFormat, error) {
	switch strings.ToLower(s) {
	case "terminal", "term":
		return FormatTerminal, nil
	case "png":
		return FormatPNG, nil
	case "svg":
		return FormatSVG, nil
	case "pdf":
		return FormatPDF, nil
	case "html":
		return FormatHTML, nil
	case "excel", "xlsx":
		return FormatExcel, nil
	case "mermaid":
		return FormatMermaid, nil
	case "template", "tpl":
		return FormatTemplate, nil
	default:
		return "", fmt.Errorf("알 수 없는 차트 포맷: %s (terminal, png, svg, pdf, html, excel, mermaid, template 중 선택)", s)
	}
}

// AxisInfo contains auto-detected axis label information.
type AxisInfo struct {
	XLabel string
	YLabel string
}

// ExtractSeriesWithAxis extracts chart series and axis info from KOSIS data maps.
func ExtractSeriesWithAxis(data []map[string]interface{}) ([]Series, AxisInfo) {
	series := ExtractSeries(data)
	info := AxisInfo{}
	if len(data) > 0 {
		row := data[0]
		// X축: 시간 컬럼명
		timeCol := detectColumn(row, []string{"수록시점", "시점", "PRD_DE"})
		if timeCol != "" {
			info.XLabel = timeCol
		}
		// Y축: 단위 정보가 있으면 사용, 없으면 "수치값"
		unitCol := detectColumn(row, []string{"단위", "UNIT_NM", "단위명"})
		if unitCol != "" {
			if v, ok := row[unitCol]; ok {
				if s, ok := v.(string); ok && s != "" {
					info.YLabel = s
				}
			}
		}
		if info.YLabel == "" {
			info.YLabel = "수치값"
		}
	}
	return series, info
}

// ExtractSeries extracts chart series from KOSIS data maps.
// It auto-detects the time column (수록시점/시점/PRD_DE) and value column (수치값/DT).
// If multiple classification values exist, it creates multiple series.
func ExtractSeries(data []map[string]interface{}) []Series {
	if len(data) == 0 {
		return nil
	}

	// Detect column names
	timeCol := detectColumn(data[0], []string{"수록시점", "시점", "PRD_DE"})
	valueCol := detectColumn(data[0], []string{"수치값", "DT"})
	classCols := detectClassColumns(data[0])

	if timeCol == "" || valueCol == "" {
		return nil
	}

	// Group data by classification value
	type entry struct {
		label string
		value float64
	}
	groups := make(map[string][]entry)
	var groupOrder []string
	skippedCount := 0

	for _, row := range data {
		groupName := ""
		if len(classCols) > 0 {
			var parts []string
			for _, col := range classCols {
				if v, ok := row[col]; ok {
					s := fmt.Sprintf("%v", v)
					if s != "" {
						parts = append(parts, s)
					}
				}
			}
			groupName = strings.Join(parts, " > ")
		}

		label := fmt.Sprintf("%v", row[timeCol])
		val, skipped := parseFloatWithWarn(row[valueCol])
		if skipped {
			skippedCount++
		}

		if _, exists := groups[groupName]; !exists {
			groupOrder = append(groupOrder, groupName)
		}
		groups[groupName] = append(groups[groupName], entry{label: label, value: val})
	}

	if skippedCount > 0 {
		fmt.Fprintf(os.Stderr, "경고: 숫자로 변환할 수 없는 값 %d건을 0으로 처리했습니다.\n", skippedCount)
	}

	// Build series
	var seriesList []Series
	for _, name := range groupOrder {
		entries := groups[name]
		s := Series{Name: name}
		for _, e := range entries {
			s.Labels = append(s.Labels, e.label)
			s.Values = append(s.Values, e.value)
		}
		seriesList = append(seriesList, s)
	}

	return seriesList
}

// Render renders chart data to the specified format.
func Render(seriesList []Series, opts Options) error {
	if len(seriesList) == 0 {
		return fmt.Errorf("차트를 생성할 데이터가 없습니다")
	}

	// Pie chart: check for negative values
	if opts.Type == Pie {
		hasNegative := false
		for _, s := range seriesList {
			for _, v := range s.Values {
				if v < 0 {
					hasNegative = true
					break
				}
			}
			if hasNegative {
				break
			}
		}
		if hasNegative {
			fmt.Fprintf(os.Stderr, "경고: 파이 차트에 음수 값이 포함되어 있어 막대 차트로 대체합니다.\n")
			opts.Type = Bar
		}
	}

	switch opts.Format {
	case FormatTerminal:
		return renderTerminal(seriesList, opts)
	case FormatPNG, FormatSVG, FormatPDF:
		return renderImage(seriesList, opts)
	case FormatHTML:
		return renderHTML(seriesList, opts)
	case FormatExcel:
		return renderExcel(seriesList, opts)
	case FormatMermaid:
		return renderMermaid(seriesList, opts)
	case FormatTemplate:
		return renderTemplate(seriesList, opts)
	default:
		return fmt.Errorf("지원하지 않는 차트 포맷: %s", opts.Format)
	}
}

func detectColumn(row map[string]interface{}, candidates []string) string {
	for _, c := range candidates {
		if _, ok := row[c]; ok {
			return c
		}
	}
	return ""
}

// detectClassColumns detects all classification columns present in the row.
// It returns them in order (C1_NM, C2_NM, C3_NM etc.) so they can be combined.
func detectClassColumns(row map[string]interface{}) []string {
	// Check classification column pairs in order
	pairs := []struct{ ko, en string }{
		{"분류값명1", "C1_NM"},
		{"분류값명2", "C2_NM"},
		{"분류값명3", "C3_NM"},
	}
	var found []string
	for _, p := range pairs {
		for _, c := range []string{p.ko, p.en} {
			if v, ok := row[c]; ok {
				if s, ok := v.(string); ok && s != "" {
					found = append(found, c)
					break // found one from this pair, skip the other
				}
			}
		}
	}
	if len(found) > 0 {
		return found
	}

	// Fallback: check any Korean label columns that look like classification
	// 시점, 수치값, 단위, 항목 등 비분류 컬럼을 제외하고 남은 문자열 컬럼이 분류
	excluded := map[string]bool{
		"수록시점": true, "시점": true, "수치값": true, "단위": true,
		"항목명": true, "항목": true, "수록주기": true, "비고": true,
		"PRD_DE": true, "DT": true, "UNIT_NM": true, "ITM_NM": true, "PRD_SE": true,
		"LST_CHN_DE": true, "CMMT": true, "ORG_ID": true, "TBL_ID": true, "TBL_NM": true,
	}
	// 분류 컬럼 후보: excluded에 없는 문자열 값 컬럼 모두 수집
	var keys []string
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var classCols []string
	for _, k := range keys {
		if excluded[k] {
			continue
		}
		if v, ok := row[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				classCols = append(classCols, k)
			}
		}
	}
	if len(classCols) > 0 {
		return classCols
	}
	return nil
}

func parseFloat(v interface{}) float64 {
	f, _ := parseFloatWithWarn(v)
	return f
}

// parseFloatWithWarn parses a value to float64 and returns whether a non-numeric value was skipped.
func parseFloatWithWarn(v interface{}) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, false
	case int:
		return float64(val), false
	case int64:
		return float64(val), false
	case string:
		val = strings.ReplaceAll(val, ",", "")
		val = strings.TrimSpace(val)
		if val == "" || val == "-" {
			return 0, false
		}
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, true
		}
		return f, false
	default:
		return 0, false
	}
}
