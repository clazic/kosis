package output

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// TableFormatter formats data as a colored table for terminal display.
// TODO: lipgloss로 교체 (TUI 단계에서 도입)
type TableFormatter struct{}

// Format formats the data as a table and writes it to the specified writer.
func (tf *TableFormatter) Format(data []map[string]interface{}, opts FormatOptions) error {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}

	columns := getColumns(data, opts)
	if len(columns) == 0 {
		return nil
	}

	// Determine if we should use Unicode and colors
	isTTY := IsTTY(opts.Writer)

	// Auto-apply MaxRows = 50 for TTY output
	if isTTY && opts.MaxRows == 0 {
		opts.MaxRows = 50
	}

	// Get terminal width
	width := getTerminalWidth()

	// Prepare column headers
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = convertColumnName(col, opts.Korean)
	}

	// Calculate column widths
	colWidths := calculateColumnWidths(columns, data, headers, width)

	// Print header
	if isTTY {
		printTableHeader(opts.Writer, headers, colWidths, true)
	} else {
		printTableHeader(opts.Writer, headers, colWidths, false)
	}

	// Print rows
	rowsToShow := len(data)
	if opts.MaxRows > 0 && rowsToShow > opts.MaxRows {
		rowsToShow = opts.MaxRows
	}

	for i := 0; i < rowsToShow; i++ {
		printTableRow(opts.Writer, columns, data[i], colWidths, isTTY)
	}

	// Print separator
	if isTTY {
		printTableSeparator(opts.Writer, colWidths, true, "bottom")
	} else {
		printTableSeparator(opts.Writer, colWidths, false, "bottom")
	}

	// Print summary if rows were truncated
	if opts.MaxRows > 0 && len(data) > opts.MaxRows {
		fmt.Fprintf(opts.Writer, "... (총 %d건 중 %d건 표시)\n", len(data), opts.MaxRows)
	}

	return nil
}

// getTerminalWidth returns the terminal width, with a default of 120.
func getTerminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 120 // 기본값
}

// calculateColumnWidths calculates the width for each column.
func calculateColumnWidths(columns []string, data []map[string]interface{}, headers []string, termWidth int) []int {
	widths := make([]int, len(columns))

	// Start with header widths
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
	}

	// Adjust based on data
	for _, row := range data {
		for i, col := range columns {
			val := formatValue(row[col])
			strLen := runewidth.StringWidth(val)
			if strLen > widths[i] {
				widths[i] = strLen
			}
		}
	}

	// Cap at 40 characters per column
	for i := range widths {
		if widths[i] > 40 {
			widths[i] = 40
		}
	}

	// Adjust widths to fit terminal width if necessary
	totalWidth := sum(widths) + len(widths)*3 // 3 chars per column for separators
	if totalWidth > termWidth && termWidth > 50 {
		// Proportionally reduce all columns
		scale := float64(termWidth-len(widths)*3) / float64(sum(widths))
		for i := range widths {
			widths[i] = int(float64(widths[i]) * scale)
			if widths[i] < 5 {
				widths[i] = 5
			}
		}
	}

	return widths
}

// sum calculates the sum of an integer slice.
func sum(nums []int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

// printTableHeader prints the table header.
func printTableHeader(w io.Writer, headers []string, widths []int, withColor bool) {
	// Top border
	printTableSeparator(w, widths, withColor, "top")

	// Header row
	fmt.Fprint(w, "│ ")
	for i, h := range headers {
		padded := padRight(h, widths[i])
		if withColor {
			fmt.Fprintf(w, "\033[1m%s\033[0m", padded) // Bold
		} else {
			fmt.Fprint(w, padded)
		}
		fmt.Fprint(w, " │ ")
	}
	fmt.Fprint(w, "\n")

	// Header separator
	printTableSeparator(w, widths, withColor, "header")
}

// printTableRow prints a single data row.
func printTableRow(w io.Writer, columns []string, row map[string]interface{}, widths []int, withColor bool) {
	fmt.Fprint(w, "│ ")
	for i, col := range columns {
		val := formatValue(row[col])
		// Truncate if too long
		if runewidth.StringWidth(val) > widths[i] {
			val = truncateString(val, widths[i]-2) + ".."
		}
		padded := padRight(val, widths[i])
		fmt.Fprint(w, padded)
		fmt.Fprint(w, " │ ")
	}
	fmt.Fprint(w, "\n")
}

// printTableSeparator prints a table separator line.
// position can be "top", "header", or "bottom" for different corner characters
func printTableSeparator(w io.Writer, widths []int, withUnicode bool, position string) {
	var sep, cornerLeft, cornerRight, cornerMiddle string

	if withUnicode {
		sep = "─"
		switch position {
		case "top":
			cornerLeft = "┌"
			cornerRight = "┐"
			cornerMiddle = "┬"
		case "header":
			cornerLeft = "├"
			cornerRight = "┤"
			cornerMiddle = "┼"
		case "bottom":
			cornerLeft = "└"
			cornerRight = "┘"
			cornerMiddle = "┴"
		default:
			cornerLeft = "┌"
			cornerRight = "┐"
			cornerMiddle = "┬"
		}
	} else {
		sep = "-"
		cornerLeft = "+"
		cornerRight = "+"
		cornerMiddle = "+"
	}

	fmt.Fprint(w, cornerLeft)
	for i, width := range widths {
		for j := 0; j < width+2; j++ {
			fmt.Fprint(w, sep)
		}
		if i < len(widths)-1 {
			fmt.Fprint(w, cornerMiddle)
		}
	}
	fmt.Fprint(w, cornerRight)
	fmt.Fprint(w, "\n")
}

// padRight pads a string to the right with spaces.
func padRight(s string, width int) string {
	strLen := runewidth.StringWidth(s)
	if strLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-strLen)
}

// truncateString truncates a string to the specified display width.
func truncateString(s string, maxWidth int) string {
	return runewidth.Truncate(s, maxWidth, "")
}
