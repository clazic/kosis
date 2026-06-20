package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		format      string
		expectError bool
	}{
		{"table", false},
		{"json", false},
		{"csv", false},
		{"xlsx", false},
		{"sqlite", false},
		{"parquet", false},
		{"unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			f, err := NewFormatter(tt.format)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for format %q, got nil", tt.format)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error for format %q, got %v", tt.format, err)
				}
				if f == nil {
					t.Fatalf("expected formatter, got nil")
				}
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"data.json", "json"},
		{"data.csv", "csv"},
		{"data.xlsx", "xlsx"},
		{"data.db", "sqlite"},
		{"data.sqlite", "sqlite"},
		{"data.parquet", "parquet"},
		{"data.txt", "table"},
		{"", "table"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			f := DetectFormat(tt.path)
			if f != tt.expected {
				t.Errorf("DetectFormat(%q) = %q, want %q", tt.path, f, tt.expected)
			}
		})
	}
}

func TestIsTTY(t *testing.T) {
	buf := &bytes.Buffer{}
	if IsTTY(buf) {
		t.Errorf("IsTTY(buffer) = true, want false")
	}
}

func TestConvertColumnName(t *testing.T) {
	tests := []struct {
		col      string
		korean   bool
		expected string
	}{
		{"ORG_ID", true, "기관ID"},
		{"ORG_ID", false, "ORG_ID"},
		{"TBL_NM", true, "통계표명"},
		{"UNKNOWN", true, "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.col, func(t *testing.T) {
			result := convertColumnName(tt.col, tt.korean)
			if result != tt.expected {
				t.Errorf("convertColumnName(%q, %v) = %q, want %q",
					tt.col, tt.korean, result, tt.expected)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		value    interface{}
		expected string
	}{
		{nil, ""},
		{"test", "test"},
		{123, "123"},
		{123.45, "123.45"},
		{123.0, "123"},
		{true, "true"},
		{false, "false"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := formatValue(tt.value)
			if result != tt.expected {
				t.Errorf("formatValue(%v) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}

func TestTableFormatter(t *testing.T) {
	data := []map[string]interface{}{
		{"ORG_ID": "101", "TBL_NM": "인구통계", "DT": "1234567"},
		{"ORG_ID": "102", "TBL_NM": "주택현황", "DT": "9876543"},
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer: buf,
		Korean: true,
	}

	tf := &TableFormatter{}
	err := tf.Format(data, opts)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "기관ID") {
		t.Errorf("output should contain Korean column name '기관ID'")
	}
	if !strings.Contains(output, "101") {
		t.Errorf("output should contain data value '101'")
	}
}

func TestTableFormatterMaxRows(t *testing.T) {
	data := make([]map[string]interface{}, 100)
	for i := 0; i < 100; i++ {
		data[i] = map[string]interface{}{
			"ID": i,
			"Name": "Item",
		}
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer:  buf,
		MaxRows: 50,
	}

	tf := &TableFormatter{}
	err := tf.Format(data, opts)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "총 100건 중 50건") {
		t.Errorf("output should contain truncation message")
	}
}

func TestJSONFormatter(t *testing.T) {
	data := []map[string]interface{}{
		{"ORG_ID": "101", "TBL_NM": "인구통계", "DT": "1234567"},
		{"ORG_ID": "102", "TBL_NM": "주택현황", "DT": "9876543"},
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer: buf,
		Korean: true,
	}

	jf := &JSONFormatter{}
	err := jf.Format(data, opts)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	output := buf.String()
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result))
	}
}

func TestJSONFormatterMaxRows(t *testing.T) {
	data := make([]map[string]interface{}, 10)
	for i := 0; i < 10; i++ {
		data[i] = map[string]interface{}{
			"ID": i,
		}
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer:  buf,
		MaxRows: 5,
	}

	jf := &JSONFormatter{}
	err := jf.Format(data, opts)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	output := buf.String()
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if len(result) != 5 {
		t.Errorf("expected 5 rows, got %d", len(result))
	}
}

func TestCSVFormatter(t *testing.T) {
	data := []map[string]interface{}{
		{"ORG_ID": "101", "TBL_NM": "인구통계", "DT": "1234567"},
		{"ORG_ID": "102", "TBL_NM": "주택현황", "DT": "9876543"},
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer: buf,
		Korean: true,
	}

	cf := &CSVFormatter{}
	err := cf.Format(data, opts)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}

	output := buf.String()
	// Check for UTF-8 BOM
	if !strings.HasPrefix(output, "\uFEFF") {
		t.Errorf("output should start with UTF-8 BOM")
	}
	if !strings.Contains(output, "기관ID") {
		t.Errorf("output should contain Korean header")
	}
	if !strings.Contains(output, "101") {
		t.Errorf("output should contain data value '101'")
	}
}

func TestGetColumns(t *testing.T) {
	data := []map[string]interface{}{
		{"A": 1, "B": 2},
		{"A": 3, "B": 4},
	}

	// Test with explicit columns
	opts := FormatOptions{
		Columns: []string{"A"},
	}
	cols := getColumns(data, opts)
	if len(cols) != 1 || cols[0] != "A" {
		t.Errorf("expected [A], got %v", cols)
	}

	// Test with empty columns (should infer from data)
	opts = FormatOptions{}
	cols = getColumns(data, opts)
	if len(cols) != 2 {
		t.Errorf("expected 2 columns, got %d", len(cols))
	}

	// Test with empty data
	opts = FormatOptions{}
	cols = getColumns([]map[string]interface{}{}, opts)
	if len(cols) != 0 {
		t.Errorf("expected 0 columns for empty data, got %d", len(cols))
	}
}

func TestXLSXFormatterNotFile(t *testing.T) {
	data := []map[string]interface{}{
		{"ORG_ID": "101", "DT": "1234567"},
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer: buf,
	}

	xf := &XLSXFormatter{}
	err := xf.Format(data, opts)
	if err == nil {
		t.Fatalf("expected error for non-file writer")
	}
	if !strings.Contains(err.Error(), "파일 경로(FilePath)가 필요합니다") {
		t.Fatalf("expected file path error message, got: %v", err)
	}
}

func TestSQLiteFormatterNotFile(t *testing.T) {
	data := []map[string]interface{}{
		{"ORG_ID": "101", "DT": "1234567"},
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer: buf,
	}

	sf := &SQLiteFormatter{}
	err := sf.Format(data, opts)
	if err == nil {
		t.Fatalf("expected error for non-file writer")
	}
	if !strings.Contains(err.Error(), "파일 경로(FilePath)가 필요합니다") {
		t.Fatalf("expected file path error message, got: %v", err)
	}
}

func TestParquetFormatterNotFile(t *testing.T) {
	data := []map[string]interface{}{
		{"ORG_ID": "101", "DT": "1234567"},
	}

	buf := &bytes.Buffer{}
	opts := FormatOptions{
		Writer: buf,
	}

	pf := &ParquetFormatter{}
	err := pf.Format(data, opts)
	if err == nil {
		t.Fatalf("expected error for non-file writer")
	}
	if !strings.Contains(err.Error(), "파일 경로(FilePath)가 필요합니다") {
		t.Fatalf("expected file path error message, got: %v", err)
	}
}
