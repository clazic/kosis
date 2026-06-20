// Package output provides data formatting and export functionality for KOSIS data.
package output

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Formatter defines the interface for formatting data output.
type Formatter interface {
	Format(data []map[string]interface{}, opts FormatOptions) error
}

// FormatOptions contains formatting options for all formatters.
type FormatOptions struct {
	// Columns is the list of columns to display. If empty, all columns are shown.
	Columns []string

	// Korean indicates whether to convert column names to Korean.
	Korean bool

	// MaxRows is the maximum number of rows to display. 0 means unlimited.
	MaxRows int

	// Writer is the output destination. Defaults to os.Stdout if not specified.
	// Used by stream-based formatters (csv, json, table).
	Writer io.Writer

	// FilePath is the output file path for formatters that manage their own file I/O
	// (xlsx, sqlite, parquet). These formatters open the file themselves and should
	// not share a file handle with the caller.
	FilePath string
}

// StandardColumnOrder defines the standard column order for consistent output.
var StandardColumnOrder = []string{
	"ORG_ID", "ORG_NM", "TBL_ID", "TBL_NM",
	"C1", "C1_NM", "C2", "C2_NM", "C3", "C3_NM",
	"C4", "C4_NM", "C5", "C5_NM", "C6", "C6_NM",
	"C7", "C7_NM", "C8", "C8_NM",
	"ITM_ID", "ITM_NM", "UNIT_NM", "PRD_SE", "PRD_DE", "DT", "LST_CHN_DE",
}

// ColKorean maps English column names to Korean names.
var ColKorean = map[string]string{
	// Organization and table info
	"ORG_ID":      "기관ID",
	"ORG_NM":      "기관명",
	"TBL_ID":      "통계표ID",
	"TBL_NM":      "통계표명",
	"STAT_NM":     "조사명",
	"STAT_ID":     "통계ID",

	// Classification columns
	"C1":      "분류값ID1",
	"C1_NM":   "분류값명1",
	"C2":      "분류값ID2",
	"C2_NM":   "분류값명2",
	"C3":      "분류값ID3",
	"C3_NM":   "분류값명3",
	"C4":      "분류값ID4",
	"C4_NM":   "분류값명4",
	"C5":      "분류값ID5",
	"C5_NM":   "분류값명5",
	"C6":      "분류값ID6",
	"C6_NM":   "분류값명6",
	"C7":      "분류값ID7",
	"C7_NM":   "분류값명7",
	"C8":      "분류값ID8",
	"C8_NM":   "분류값명8",

	// Item and period info
	"ITM_ID":      "항목ID",
	"ITM_NM":      "항목명",
	"UNIT_NM":     "단위명",
	"PRD_SE":      "수록주기",
	"PRD_DE":      "수록시점",

	// Data and metadata
	"DT":           "수치값",
	"CMMT":         "비고",
	"LST_CHN_DE":   "최종수정일",
	"JIPYO_ID":     "지표ID",
	"JIPYO_NM":     "지표명",
	"DATA_DE":      "통계시점",
	"UNIT":         "단위",
	"LAST_UPDATE":  "최종갱신",
	"WGT":          "가중치",
	"NCD":          "갱신일",
	"SOURCE":       "출처",
}

// NewFormatter returns a formatter based on the specified format string.
func NewFormatter(format string) (Formatter, error) {
	switch strings.ToLower(format) {
	case "table":
		return &TableFormatter{}, nil
	case "json":
		return &JSONFormatter{}, nil
	case "csv":
		return &CSVFormatter{}, nil
	case "markdown", "md":
		return &MarkdownFormatter{}, nil
	case "xlsx":
		return &XLSXFormatter{}, nil
	case "sqlite":
		return &SQLiteFormatter{}, nil
	case "parquet":
		return &ParquetFormatter{}, nil
	default:
		return nil, fmt.Errorf("알 수 없는 출력 형식: %s", format)
	}
}

// DetectFormat determines the output format from a file extension.
func DetectFormat(outputPath string) string {
	if outputPath == "" {
		return "table"
	}

	lower := strings.ToLower(outputPath)
	if strings.HasSuffix(lower, ".json") {
		return "json"
	}
	if strings.HasSuffix(lower, ".csv") {
		return "csv"
	}
	if strings.HasSuffix(lower, ".xlsx") {
		return "xlsx"
	}
	if strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite") {
		return "sqlite"
	}
	if strings.HasSuffix(lower, ".parquet") {
		return "parquet"
	}
	return "table"
}

// IsTTY checks whether the given writer is connected to a terminal.
func IsTTY(w io.Writer) bool {
	if w == nil {
		w = os.Stdout
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// getColumns returns the list of columns to display.
// If opts.Columns is not empty, returns opts.Columns.
// Otherwise, returns all keys from the first data row in standard order.
func getColumns(data []map[string]interface{}, opts FormatOptions) []string {
	if len(opts.Columns) > 0 {
		return opts.Columns
	}

	if len(data) == 0 {
		return []string{}
	}

	// Extract all unique keys from the first row
	allKeys := make(map[string]bool)
	for k := range data[0] {
		allKeys[k] = true
	}

	// Start with standard order
	columns := make([]string, 0, len(allKeys))
	for _, k := range StandardColumnOrder {
		if allKeys[k] {
			columns = append(columns, k)
			delete(allKeys, k)
		}
	}

	// Add remaining keys in sorted order
	remaining := make([]string, 0, len(allKeys))
	for k := range allKeys {
		remaining = append(remaining, k)
	}
	sort.Strings(remaining)
	columns = append(columns, remaining...)

	return columns
}

// convertColumnName converts a column name to Korean if requested.
func convertColumnName(col string, korean bool) string {
	if !korean {
		return col
	}
	if kr, ok := ColKorean[col]; ok {
		return kr
	}
	return col
}

// formatValue formats a value for display.
func formatValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Format numbers without unnecessary decimals
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%g", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}
