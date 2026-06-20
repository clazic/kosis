package output

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteFormatter formats data as SQLite database.
type SQLiteFormatter struct {
	TableName string // Optional: custom table name
}

// Format formats the data as SQLite and writes it to a database file.
// opts.FilePath must be set for this formatter.
func (sf *SQLiteFormatter) Format(data []map[string]interface{}, opts FormatOptions) error {
	// Validate that FilePath is set
	if opts.FilePath == "" {
		return fmt.Errorf("SQLite 포맷터는 파일 경로(FilePath)가 필요합니다")
	}

	dbPath := opts.FilePath

	// Open or create database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("데이터베이스 열기 실패: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("데이터베이스 연결 실패: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	columns := getColumns(data, opts)
	if len(columns) == 0 {
		return nil
	}

	// Determine table name
	tableName := sf.TableName
	if tableName == "" {
		// Try to get from TBL_ID if available
		if tblID, ok := data[0]["TBL_ID"].(string); ok && tblID != "" {
			tableName = tblID
		} else {
			tableName = "kosis_data"
		}
	}

	// Sanitize table name
	tableName = sanitizeTableName(tableName)

	// Create table if not exists
	if err := createTable(db, tableName, data, columns); err != nil {
		return fmt.Errorf("테이블 생성 실패: %w", err)
	}

	// Insert data
	rowsToInsert := len(data)
	if opts.MaxRows > 0 && rowsToInsert > opts.MaxRows {
		rowsToInsert = opts.MaxRows
	}

	if err := insertData(db, tableName, data[:rowsToInsert], columns); err != nil {
		return fmt.Errorf("데이터 삽입 실패: %w", err)
	}

	// Create indexes on PRD_DE and C1 if they exist
	if hasColumn(columns, "PRD_DE") {
		if err := createIndex(db, tableName, "PRD_DE"); err != nil {
			// Non-fatal error, continue
			fmt.Fprintf(os.Stderr, "경고: PRD_DE 인덱스 생성 실패: %v\n", err)
		}
	}

	if hasColumn(columns, "C1") {
		if err := createIndex(db, tableName, "C1"); err != nil {
			// Non-fatal error, continue
			fmt.Fprintf(os.Stderr, "경고: C1 인덱스 생성 실패: %v\n", err)
		}
	}

	// Create or update _kosis_meta table
	if err := updateMetaTable(db, tableName); err != nil {
		// Non-fatal error, continue
		fmt.Fprintf(os.Stderr, "경고: 메타 테이블 업데이트 실패: %v\n", err)
	}

	return nil
}

// createTable creates a table based on the data structure
func createTable(db *sql.DB, tableName string, data []map[string]interface{}, columns []string) error {
	// Check if table exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
	if err != nil {
		return fmt.Errorf("테이블 존재 여부 확인 실패: %w", err)
	}

	if count > 0 {
		// Table already exists, no need to create
		return nil
	}

	// Build CREATE TABLE statement
	createStmt := fmt.Sprintf("CREATE TABLE %s (", quoteIdentifier(tableName))
	colDefs := make([]string, len(columns))

	for i, col := range columns {
		colType := inferColumnType(data, col)
		colDefs[i] = fmt.Sprintf("%s %s", quoteIdentifier(col), colType)
	}

	createStmt += strings.Join(colDefs, ", ") + ")"

	if _, err := db.Exec(createStmt); err != nil {
		return fmt.Errorf("테이블 생성 SQL 실행 실패: %w", err)
	}

	return nil
}

// inferColumnType infers the SQL type for a column based on sample data
func inferColumnType(data []map[string]interface{}, col string) string {
	// For DT (data) column, try to detect if it's numeric
	if col == "DT" {
		for _, row := range data {
			if val, ok := row[col]; ok && val != nil {
				// Try to parse as float
				if str, ok := val.(string); ok {
					if _, err := strconv.ParseFloat(str, 64); err == nil {
						return "REAL"
					}
				} else if _, ok := val.(float64); ok {
					return "REAL"
				}
			}
		}
		return "TEXT"
	}

	// Default to TEXT for other columns
	return "TEXT"
}

// insertData inserts data rows into the table
func insertData(db *sql.DB, tableName string, data []map[string]interface{}, columns []string) error {
	// Build INSERT statement template with backtick-quoted identifiers
	placeholders := make([]string, len(columns))
	quotedColumns := make([]string, len(columns))
	for i, col := range columns {
		placeholders[i] = "?"
		quotedColumns[i] = quoteIdentifier(col)
	}

	insertStmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdentifier(tableName),
		strings.Join(quotedColumns, ", "),
		strings.Join(placeholders, ", "),
	)

	// Use transaction for batch insert performance and atomicity
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 실패: %w", err)
	}

	stmt, err := tx.Prepare(insertStmt)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("INSERT 문 준비 실패: %w", err)
	}
	defer stmt.Close()

	for _, row := range data {
		values := make([]interface{}, len(columns))
		for i, col := range columns {
			values[i] = row[col]
		}

		if _, err := stmt.Exec(values...); err != nil {
			tx.Rollback()
			return fmt.Errorf("INSERT 실행 실패: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("트랜잭션 커밋 실패: %w", err)
	}

	return nil
}

// createIndex creates an index on a column
func createIndex(db *sql.DB, tableName, columnName string) error {
	sanitizedTableName := sanitizeIdentifier(tableName)
	sanitizedColumnName := sanitizeIdentifier(columnName)
	indexName := fmt.Sprintf("idx_%s_%s", sanitizedTableName, sanitizedColumnName)
	// Limit index name length for SQLite
	if len(indexName) > 64 {
		indexName = indexName[:64]
	}

	createIndexStmt := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(%s)",
		quoteIdentifier(indexName), quoteIdentifier(tableName), quoteIdentifier(columnName))

	if _, err := db.Exec(createIndexStmt); err != nil {
		return fmt.Errorf("인덱스 생성 실패: %w", err)
	}

	return nil
}

// updateMetaTable updates the _kosis_meta table with query information
func updateMetaTable(db *sql.DB, tableName string) error {
	// Create _kosis_meta table if not exists
	createMetaStmt := `
	CREATE TABLE IF NOT EXISTS _kosis_meta (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tbl_id TEXT,
		table_name TEXT,
		queried_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		row_count INTEGER
	)
	`

	if _, err := db.Exec(createMetaStmt); err != nil {
		return fmt.Errorf("메타 테이블 생성 실패: %w", err)
	}

	// Count rows in the data table
	var rowCount int
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(tableName))).Scan(&rowCount)
	if err != nil {
		return fmt.Errorf("행 수 계산 실패: %w", err)
	}

	// Insert metadata
	insertMetaStmt := `
	INSERT INTO _kosis_meta (tbl_id, table_name, queried_at, row_count)
	VALUES (?, ?, ?, ?)
	`

	tblID := tableName
	_, err = db.Exec(insertMetaStmt, tblID, tableName, time.Now(), rowCount)
	if err != nil {
		return fmt.Errorf("메타 데이터 삽입 실패: %w", err)
	}

	return nil
}

// sanitizeIdentifier sanitizes a SQL identifier (table name or column name) for SQLite
func sanitizeIdentifier(name string) string {
	// Replace invalid characters with underscore
	replacer := strings.NewReplacer(
		"-", "_",
		".", "_",
		"/", "_",
		"\\", "_",
		" ", "_",
		"\t", "_",
		"\n", "_",
		"'", "",
		";", "",
		"`", "",
	)
	sanitized := replacer.Replace(name)

	// Remove SQL comment sequences
	sanitized = strings.ReplaceAll(sanitized, "--", "")

	// Remove leading digits if any (SQLite identifiers shouldn't start with digit)
	if len(sanitized) > 0 && sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "_" + sanitized
	}

	return sanitized
}

// sanitizeTableName sanitizes a table name for SQLite
func sanitizeTableName(name string) string {
	return sanitizeIdentifier(name)
}

// quoteIdentifier wraps an identifier with backticks for safe use in SQL
func quoteIdentifier(name string) string {
	return "`" + sanitizeIdentifier(name) + "`"
}

// hasColumn checks if a column exists in the columns list
func hasColumn(columns []string, colName string) bool {
	for _, col := range columns {
		if col == colName {
			return true
		}
	}
	return false
}
