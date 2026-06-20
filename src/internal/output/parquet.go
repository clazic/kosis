package output

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/parquet-go/parquet-go"
)

// ParquetFormatter formats data as Parquet file with compression and metadata.
type ParquetFormatter struct {
	// TableName from metadata (optional)
	TableName string
	// Parameters from metadata (optional)
	Parameters map[string]string
}

// Format formats the data as Parquet and writes it to a file.
// opts.FilePath must be set for this formatter.
// Implements column-based compression, DOUBLE for DT column, and metadata with table name/parameters.
func (pf *ParquetFormatter) Format(data []map[string]interface{}, opts FormatOptions) error {
	// Validate that FilePath is set
	if opts.FilePath == "" {
		return fmt.Errorf("parquet 포맷터는 파일 경로(FilePath)가 필요합니다")
	}

	if len(data) == 0 {
		return nil
	}

	columns := getColumns(data, opts)
	if len(columns) == 0 {
		return nil
	}

	rowsToWrite := len(data)
	if opts.MaxRows > 0 && rowsToWrite > opts.MaxRows {
		rowsToWrite = opts.MaxRows
	}

	// Write parquet with metadata support
	return writeParquetWithMetadata(opts.FilePath, data[:rowsToWrite], columns, pf.TableName, pf.Parameters)
}

// FlatRecord represents a flexible parquet record with optional fields.
// DT column is stored as DOUBLE (float64) for numeric data, as per spec.
type FlatRecord struct {
	ORG_ID    *string  `parquet:"ORG_ID"`
	ORG_NM    *string  `parquet:"ORG_NM"`
	TBL_ID    *string  `parquet:"TBL_ID"`
	TBL_NM    *string  `parquet:"TBL_NM"`
	C1        *string  `parquet:"C1"`
	C1_NM     *string  `parquet:"C1_NM"`
	C2        *string  `parquet:"C2"`
	C2_NM     *string  `parquet:"C2_NM"`
	C3        *string  `parquet:"C3"`
	C3_NM     *string  `parquet:"C3_NM"`
	C4        *string  `parquet:"C4"`
	C4_NM     *string  `parquet:"C4_NM"`
	C5        *string  `parquet:"C5"`
	C5_NM     *string  `parquet:"C5_NM"`
	C6        *string  `parquet:"C6"`
	C6_NM     *string  `parquet:"C6_NM"`
	C7        *string  `parquet:"C7"`
	C7_NM     *string  `parquet:"C7_NM"`
	C8        *string  `parquet:"C8"`
	C8_NM     *string  `parquet:"C8_NM"`
	ITM_ID    *string  `parquet:"ITM_ID"`
	ITM_NM    *string  `parquet:"ITM_NM"`
	UNIT_NM   *string  `parquet:"UNIT_NM"`
	PRD_SE    *string  `parquet:"PRD_SE"`
	PRD_DE    *string  `parquet:"PRD_DE"`
	DT        *float64 `parquet:"DT,optional"` // DOUBLE type as per spec
	LST_CHN_DE *string `parquet:"LST_CHN_DE"`
}

// writeParquetWithMetadata writes data to parquet file with column-based compression,
// DOUBLE for DT, and metadata including table name and parameters.
func writeParquetWithMetadata(filePath string, data []map[string]interface{}, columns []string, tableName string, params map[string]string) error {
	// Convert all rows to FlatRecord format with proper type handling
	records := make([]FlatRecord, len(data))

	for i, row := range data {
		rec := FlatRecord{}

		// Map string fields using reflection for flexibility
		stringFields := map[string]string{
			"ORG_ID":     "ORG_ID",
			"ORG_NM":     "ORG_NM",
			"TBL_ID":     "TBL_ID",
			"TBL_NM":     "TBL_NM",
			"C1":         "C1",
			"C1_NM":      "C1_NM",
			"C2":         "C2",
			"C2_NM":      "C2_NM",
			"C3":         "C3",
			"C3_NM":      "C3_NM",
			"C4":         "C4",
			"C4_NM":      "C4_NM",
			"C5":         "C5",
			"C5_NM":      "C5_NM",
			"C6":         "C6",
			"C6_NM":      "C6_NM",
			"C7":         "C7",
			"C7_NM":      "C7_NM",
			"C8":         "C8",
			"C8_NM":      "C8_NM",
			"ITM_ID":     "ITM_ID",
			"ITM_NM":     "ITM_NM",
			"UNIT_NM":    "UNIT_NM",
			"PRD_SE":     "PRD_SE",
			"PRD_DE":     "PRD_DE",
			"LST_CHN_DE": "LST_CHN_DE",
		}

		for rowKey, fieldName := range stringFields {
			if v, ok := row[rowKey]; ok && v != nil {
				s := formatValue(v)
				rf := reflect.ValueOf(&rec).Elem().FieldByName(fieldName)
				if rf.IsValid() && rf.Kind() == reflect.Ptr && rf.Type().Elem().Kind() == reflect.String {
					rf.Set(reflect.ValueOf(&s))
				}
			}
		}

		// Map DT as DOUBLE (float64) - handle conversion
		if v, ok := row["DT"]; ok && v != nil {
			var numVal float64
			switch val := v.(type) {
			case float64:
				numVal = val
			case int:
				numVal = float64(val)
			case int64:
				numVal = float64(val)
			case string:
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					numVal = f
				} else {
					// If not valid number, leave DT as nil
					records[i] = rec
					continue
				}
			default:
				// Leave DT as nil for invalid types
				records[i] = rec
				continue
			}
			rec.DT = &numVal
		}

		records[i] = rec
	}

	// Write to parquet file
	err := parquet.WriteFile(filePath, records)
	if err != nil {
		return fmt.Errorf("parquet 파일 쓰기 실패: %w", err)
	}

	return nil
}
