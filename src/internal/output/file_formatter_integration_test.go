package output

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestWriteToFileXLSX(t *testing.T) {
	data := []map[string]interface{}{
		{
			"ORG_ID": "101",
			"ORG_NM": "통계청",
			"TBL_ID": "DT_1IN1502",
			"TBL_NM": "인구통계",
			"C1": "11",
			"C1_NM": "서울",
			"ITM_NM": "총인구",
			"PRD_DE": "2024",
			"DT": "9500000",
		},
		{
			"ORG_ID": "101",
			"ORG_NM": "통계청",
			"TBL_ID": "DT_1IN1502",
			"TBL_NM": "인구통계",
			"C1": "26",
			"C1_NM": "부산",
			"ITM_NM": "총인구",
			"PRD_DE": "2024",
			"DT": "3200000",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test_output.xlsx")

	err := WriteToFile(data, outputPath, FormatOptions{})
	if err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}

	if info.Size() == 0 {
		t.Fatalf("File is empty")
	}

	t.Logf("XLSX file created successfully: %d bytes", info.Size())
}

func TestWriteToFileSQLite(t *testing.T) {
	data := []map[string]interface{}{
		{
			"ORG_ID": "101",
			"ORG_NM": "통계청",
			"TBL_ID": "DT_1IN1502",
			"C1": "11",
			"C1_NM": "서울",
			"PRD_DE": "2024",
			"DT": "9500000",
		},
		{
			"ORG_ID": "101",
			"ORG_NM": "통계청",
			"TBL_ID": "DT_1IN1502",
			"C1": "26",
			"C1_NM": "부산",
			"PRD_DE": "2024",
			"DT": "3200000",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test_output.db")

	err := WriteToFile(data, outputPath, FormatOptions{})
	if err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}

	if info.Size() == 0 {
		t.Fatalf("File is empty")
	}

	// Verify database contents
	db, err := sql.Open("sqlite3", outputPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	var rowCount int
	err = db.QueryRow("SELECT COUNT(*) FROM DT_1IN1502").Scan(&rowCount)
	if err != nil {
		t.Fatalf("Failed to query data: %v", err)
	}

	if rowCount != 2 {
		t.Errorf("expected 2 rows, got %d", rowCount)
	}

	t.Logf("SQLite database created successfully with %d rows", rowCount)
}

func TestWriteToFileParquet(t *testing.T) {
	data := []map[string]interface{}{
		{
			"ORG_ID": "101",
			"TBL_ID": "DT_1IN1502",
			"C1": "11",
			"PRD_DE": "2024",
			"DT": "9500000",
		},
		{
			"ORG_ID": "101",
			"TBL_ID": "DT_1IN1502",
			"C1": "26",
			"PRD_DE": "2024",
			"DT": "3200000",
		},
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test_output.parquet")

	err := WriteToFile(data, outputPath, FormatOptions{})
	if err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}

	if info.Size() == 0 {
		t.Fatalf("File is empty")
	}

	t.Logf("Parquet file created successfully: %d bytes", info.Size())
}
