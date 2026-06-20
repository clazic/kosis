package output

import (
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// XLSXFormatter formats data as XLSX (Excel) with styling.
type XLSXFormatter struct{}

// Format formats the data as XLSX and writes it to a file.
// opts.FilePath must be set for this formatter.
func (xf *XLSXFormatter) Format(data []map[string]interface{}, opts FormatOptions) error {
	// Validate that FilePath is set
	if opts.FilePath == "" {
		return fmt.Errorf("XLSX 포맷터는 파일 경로(FilePath)가 필요합니다")
	}
	filePath := opts.FilePath

	// Create a new workbook
	wb := excelize.NewFile()
	defer wb.Close()

	// Create new sheet with default name
	sheetName := "KOSIS Data"
	if _, err := wb.NewSheet(sheetName); err != nil {
		return fmt.Errorf("시트 생성 실패: %w", err)
	}

	// Remove default sheet (must be after creating new sheet to avoid empty workbook)
	wb.DeleteSheet("Sheet1")

	columns := getColumns(data, opts)
	if len(columns) == 0 {
		// Empty data, just save the file
		return wb.SaveAs(filePath)
	}

	// Write header row
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = convertColumnName(col, true)
	}

	// Write headers with styling
	headerStyle, err := wb.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"ADD8E6"}, // Light blue
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	if err != nil {
		return fmt.Errorf("헤더 스타일 생성 실패: %w", err)
	}

	for i, header := range headers {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		cell := fmt.Sprintf("%s1", colName)
		if err := wb.SetCellValue(sheetName, cell, header); err != nil {
			return fmt.Errorf("헤더 셀 설정 실패: %w", err)
		}
		if err := wb.SetCellStyle(sheetName, cell, cell, headerStyle); err != nil {
			return fmt.Errorf("헤더 셀 스타일 적용 실패: %w", err)
		}
	}

	// Number style for DT column (data)
	customNumFmt := "0.00"
	numStyle, err := wb.NewStyle(&excelize.Style{
		CustomNumFmt: &customNumFmt,
	})
	if err != nil {
		return fmt.Errorf("숫자 스타일 생성 실패: %w", err)
	}

	// Write data rows
	rowsToWrite := len(data)
	if opts.MaxRows > 0 && rowsToWrite > opts.MaxRows {
		rowsToWrite = opts.MaxRows
	}

	for i := 0; i < rowsToWrite; i++ {
		for j, col := range columns {
			colName, _ := excelize.ColumnNumberToName(j + 1)
			cellRef := fmt.Sprintf("%s%d", colName, i+2)
			value := data[i][col]

			// Try to parse as number for DT column
			if col == "DT" && value != nil {
				if str, ok := value.(string); ok {
					if num, err := strconv.ParseFloat(str, 64); err == nil {
						if err := wb.SetCellValue(sheetName, cellRef, num); err != nil {
							return fmt.Errorf("데이터 셀 설정 실패: %w", err)
						}
						if err := wb.SetCellStyle(sheetName, cellRef, cellRef, numStyle); err != nil {
							return fmt.Errorf("숫자 스타일 적용 실패: %w", err)
						}
						continue
					}
				} else if f, ok := value.(float64); ok {
					if err := wb.SetCellValue(sheetName, cellRef, f); err != nil {
						return fmt.Errorf("데이터 셀 설정 실패: %w", err)
					}
					if err := wb.SetCellStyle(sheetName, cellRef, cellRef, numStyle); err != nil {
						return fmt.Errorf("숫자 스타일 적용 실패: %w", err)
					}
					continue
				}
			}

			// Default: set as string
			if err := wb.SetCellValue(sheetName, cellRef, formatValue(value)); err != nil {
				return fmt.Errorf("데이터 셀 설정 실패: %w", err)
			}
		}
	}

	// Auto-adjust column widths
	for i := range columns {
		col, _ := excelize.ColumnNumberToName(i + 1)
		if err := wb.SetColWidth(sheetName, col, col, 15); err != nil {
			return fmt.Errorf("컬럼 폭 조정 실패: %w", err)
		}
	}

	// Save the file
	if err := wb.SaveAs(filePath); err != nil {
		return fmt.Errorf("파일 저장 실패: %w", err)
	}

	return nil
}
