package chart

import (
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"
)

func renderExcel(seriesList []Series, opts Options) error {
	if opts.Output == "" {
		return fmt.Errorf("Excel 차트는 -o/--output 파일 경로가 필요합니다")
	}

	wb := excelize.NewFile()
	defer wb.Close()

	sheetName := "Chart Data"
	wb.SetSheetName("Sheet1", sheetName)

	// Write data: header row
	wb.SetCellValue(sheetName, "A1", "시점")
	for i, s := range seriesList {
		col, _ := excelize.ColumnNumberToName(i + 2) // B=2, C=3, ...
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("시리즈 %d", i+1)
		}
		wb.SetCellValue(sheetName, fmt.Sprintf("%s1", col), name)
	}

	// Write data rows using the first series' labels
	maxRows := 0
	for _, s := range seriesList {
		if len(s.Values) > maxRows {
			maxRows = len(s.Values)
		}
	}

	for row := 0; row < maxRows; row++ {
		// Time label from first series
		label := ""
		if len(seriesList) > 0 && row < len(seriesList[0].Labels) {
			label = seriesList[0].Labels[row]
		}
		wb.SetCellValue(sheetName, fmt.Sprintf("A%d", row+2), label)

		for i, s := range seriesList {
			col, _ := excelize.ColumnNumberToName(i + 2)
			if row < len(s.Values) {
				wb.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, row+2), s.Values[row])
			}
		}
	}

	// Create chart
	chartType := excelize.Line
	switch opts.Type {
	case Bar:
		chartType = excelize.Col
	case Pie:
		chartType = excelize.Pie
	}

	// Build series definitions for the chart
	lastDataCol, _ := excelize.ColumnNumberToName(len(seriesList) + 1)
	lastDataRow := strconv.Itoa(maxRows + 1)

	var excelSeries []excelize.ChartSeries
	for i, s := range seriesList {
		col, _ := excelize.ColumnNumberToName(i + 2)
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("시리즈 %d", i+1)
		}
		excelSeries = append(excelSeries, excelize.ChartSeries{
			Name:       fmt.Sprintf("'%s'!$%s$1", sheetName, col),
			Categories: fmt.Sprintf("'%s'!$A$2:$A$%s", sheetName, lastDataRow),
			Values:     fmt.Sprintf("'%s'!$%s$2:$%s$%s", sheetName, col, col, lastDataRow),
		})
	}
	_ = lastDataCol

	title := opts.Title
	if title == "" && len(seriesList) > 0 {
		title = seriesList[0].Name
	}

	chartOpts := &excelize.Chart{
		Type: chartType,
		Series: excelSeries,
		Title: []excelize.RichTextRun{
			{Text: title},
		},
		Legend: excelize.ChartLegend{
			Position:      "top",
			ShowLegendKey: true,
		},
		PlotArea: excelize.ChartPlotArea{
			ShowVal: true,
		},
	}

	// Add chart to the sheet
	if err := wb.AddChart(sheetName, "A"+strconv.Itoa(maxRows+3), chartOpts); err != nil {
		return fmt.Errorf("Excel 차트 생성 실패: %w", err)
	}

	if err := wb.SaveAs(opts.Output); err != nil {
		return fmt.Errorf("Excel 파일 저장 실패: %w", err)
	}

	if opts.Open {
		return openFile(opts.Output)
	}
	return nil
}
