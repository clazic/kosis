package output

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CSVFormatter formats data as CSV with UTF-8 BOM for Excel compatibility.
type CSVFormatter struct{}

// Format formats the data as CSV and writes it to the specified writer.
func (cf *CSVFormatter) Format(data []map[string]interface{}, opts FormatOptions) error {
	// BOM 은 파일(-o)에만 — stdout 은 파이프·파서(AI)가 읽으므로 첫 열 이름이 깨지지 않게
	toStdout := opts.Writer == nil || opts.Writer == os.Stdout
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}

	columns := getColumns(data, opts)
	if len(columns) == 0 {
		return nil
	}

	// Write UTF-8 BOM for Excel compatibility with Korean characters
	if !toStdout {
		if _, err := opts.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return fmt.Errorf("failed to write UTF-8 BOM: %w", err)
		}
	}

	// Create CSV writer
	w := csv.NewWriter(opts.Writer)

	// Write header row with Korean column names (default behavior)
	headers := make([]string, len(columns))
	for i, col := range columns {
		// By default, use Korean names for CSV headers for better readability
		headers[i] = convertColumnName(col, !opts.RawKeys)
	}

	if err := w.Write(headers); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	rowsToWrite := len(data)
	if opts.MaxRows > 0 && rowsToWrite > opts.MaxRows {
		rowsToWrite = opts.MaxRows
	}

	for i := 0; i < rowsToWrite; i++ {
		row := make([]string, len(columns))
		for j, col := range columns {
			row[j] = sanitizeCSVValue(formatValue(data[i][col]))
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row %d: %w", i+1, err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("CSV writer error: %w", err)
	}

	return nil
}

// sanitizeCSVValue prevents CSV injection by prefixing values that start with
// formula-triggering characters (=, +, -, @, \t, \r) with a single quote.
// This prevents formula injection when CSV files are opened in Excel.
func sanitizeCSVValue(s string) string {
	if s == "" {
		return s
	}
	if _, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64); err == nil { // -108.7 같은 숫자는 수식이 아니다
		return s
	}
	if strings.HasPrefix(s, "=") || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") ||
		strings.HasPrefix(s, "@") || strings.HasPrefix(s, "\t") || strings.HasPrefix(s, "\r") {
		return "'" + s
	}
	return s
}
