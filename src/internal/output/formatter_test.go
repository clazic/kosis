package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"fmt"
	"github.com/xuri/excelize/v2"
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
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
			"ID":   i,
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

// 파이프(비TTY)로 나가는 표는 값을 자르지 않는다 — AI·스크립트가 코드를 읽어야 한다.
func TestTableNoTruncateWhenPiped(t *testing.T) {
	var buf bytes.Buffer
	long := "15118ZONE2012_262601-아주-긴-분류-이름-" + strings.Repeat("가", 40)
	rows := []map[string]interface{}{{"플래그": "--class1", "코드": long}}
	if err := (&TableFormatter{}).Format(rows, FormatOptions{Writer: &buf, Columns: []string{"플래그", "코드"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), long) || !strings.Contains(buf.String(), "--class1") {
		t.Errorf("잘림:\n%s", buf.String())
	}
}

// RawKeys(kosis d --api-keys)는 모든 형식에서 열 이름을 그대로 둔다. 기본은 기존 한국어 헤더 유지.
func TestRawKeysAllFormats(t *testing.T) {
	rows := []map[string]interface{}{{"C1": "26010", "C1_NM": "중구", "DT": "1"}}
	cols := []string{"C1", "C1_NM", "DT"}
	for _, f := range []string{"csv", "json", "md", "table"} {
		raw, err := FormatToString(rows, f, FormatOptions{Columns: cols, Korean: false, RawKeys: true})
		if err != nil {
			t.Fatal(f, err)
		}
		if !strings.Contains(raw, "C1_NM") || strings.Contains(raw, ColKorean["C1_NM"]) {
			t.Errorf("%s RawKeys 헤더:\n%s", f, raw)
		}
	}
	def, _ := FormatToString(rows, "csv", FormatOptions{Columns: cols})
	if !strings.Contains(def, ColKorean["C1_NM"]) {
		t.Errorf("csv 기본 헤더는 한국어 유지:\n%s", def)
	}

	dir := t.TempDir()
	for _, raw := range []bool{true, false} {
		p := filepath.Join(dir, fmt.Sprintf("x-%v.xlsx", raw))
		if err := WriteToFile(rows, p, FormatOptions{Columns: cols, RawKeys: raw}); err != nil {
			t.Fatal(err)
		}
		x, err := excelize.OpenFile(p)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := x.GetRows(x.GetSheetName(0))
		x.Close()
		want := "C1_NM"
		if !raw {
			want = ColKorean["C1_NM"]
		}
		if len(got) == 0 || !strings.Contains(strings.Join(got[0], ","), want) {
			t.Errorf("xlsx RawKeys=%v 헤더 %v, want %s", raw, got, want)
		}
	}
}

// 같은 데이터·옵션이면 JSON stdout 과 -o *.json 파일 내용이 같다.
func TestJSONStdoutEqualsFile(t *testing.T) {
	rows := []map[string]interface{}{{"C1": "26010", "C1_NM": "중구", "DT": "1"}}
	for _, opts := range []FormatOptions{{Columns: []string{"C1", "C1_NM", "DT"}, Korean: true}, {Columns: []string{"C1", "C1_NM", "DT"}, RawKeys: true}} {
		stdout, err := FormatToString(rows, "json", opts)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(t.TempDir(), "r.json")
		if err := WriteToFile(rows, p, opts); err != nil {
			t.Fatal(err)
		}
		file, _ := os.ReadFile(p)
		if strings.TrimSpace(stdout) != strings.TrimSpace(string(file)) {
			t.Errorf("stdout:\n%s\nfile:\n%s", stdout, file)
		}
	}
}

// parquet 은 API 키 열(C1_NM·ITM_NM·DT …)을 값으로 저장한다 — 한국어 라벨 키는 담지 못한다.
func TestParquetStoresAPIKeyRows(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.parquet")
	rows := []map[string]interface{}{{"C1_NM": "중구", "ITM_NM": "종사자수", "DT": "705"}}
	if err := WriteToFile(rows, p, FormatOptions{Columns: []string{"C1_NM", "ITM_NM", "DT"}}); err != nil {
		t.Fatal(err)
	}
	got, err := parquet.ReadFile[FlatRecord](p)
	if err != nil || len(got) != 1 || got[0].C1_NM == nil || *got[0].C1_NM != "중구" || got[0].DT == nil || *got[0].DT != 705 {
		t.Fatalf("read back: %+v %v", got, err)
	}
}

// 숫자로 표현할 수 없는 DT(X 비공개, - 등)는 DT=null 이지만 DT_RAW 에 원문이 남는다.
func TestParquetKeepsNonNumericDT(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.parquet")
	rows := []map[string]interface{}{
		{"C1_NM": "a", "DT": "705"}, {"C1_NM": "b", "DT": "X"}, {"C1_NM": "c", "DT": "-"}, {"C1_NM": "d", "DT": "1,234"}, {"C1_NM": "e"}, {"C1_NM": "f", "DT": ""},
	}
	if err := WriteToFile(rows, p, FormatOptions{Columns: []string{"C1_NM", "DT"}}); err != nil {
		t.Fatal(err)
	}
	got, err := parquet.ReadFile[FlatRecord](p)
	if err != nil || len(got) != 6 {
		t.Fatalf("%v %d", err, len(got))
	}
	str := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}
	if got[0].DT == nil || *got[0].DT != 705 || str(got[0].DT_RAW) != "705" {
		t.Errorf("숫자: %v %s", got[0].DT, str(got[0].DT_RAW))
	}
	for i, want := range []string{"X", "-", "1,234"} {
		r := got[i+1]
		if r.DT != nil || str(r.DT_RAW) != want {
			t.Errorf("%s: DT=%v DT_RAW=%s", want, r.DT, str(r.DT_RAW))
		}
	}
	if got[4].DT != nil || got[4].DT_RAW != nil {
		t.Errorf("DT 없는 행: %v %v", got[4].DT, got[4].DT_RAW)
	}
	if got[5].DT != nil || got[5].DT_RAW == nil || *got[5].DT_RAW != "" {
		t.Errorf("빈 문자열 DT: DT=%v DT_RAW=%s (null 과 구별돼야 함)", got[5].DT, str(got[5].DT_RAW))
	}
}
