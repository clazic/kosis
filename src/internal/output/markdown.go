package output

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
)

// MarkdownFormatter formats data as a GitHub-flavored Markdown table.
type MarkdownFormatter struct{}

// Format formats the data as a Markdown table.
func (mf *MarkdownFormatter) Format(data []map[string]interface{}, opts FormatOptions) error {
	if len(data) == 0 {
		return nil
	}

	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}

	columns := getColumns(data, opts)
	if len(columns) == 0 {
		return nil
	}

	// Build header names
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = convertColumnName(col, opts.Korean)
	}

	// Calculate column widths
	widths := make([]int, len(columns))
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
		if widths[i] < 3 {
			widths[i] = 3 // minimum width for "---"
		}
	}

	rowsToWrite := len(data)
	if opts.MaxRows > 0 && rowsToWrite > opts.MaxRows {
		rowsToWrite = opts.MaxRows
	}

	for i := 0; i < rowsToWrite; i++ {
		for j, col := range columns {
			val := formatValue(data[i][col])
			w := runewidth.StringWidth(val)
			if w > widths[j] {
				widths[j] = w
			}
		}
	}

	// Write header row
	writeMarkdownRow(writer, headers, widths)

	// Write separator row
	seps := make([]string, len(columns))
	for i, w := range widths {
		seps[i] = strings.Repeat("-", w)
	}
	fmt.Fprintf(writer, "| %s |\n", strings.Join(seps, " | "))

	// Write data rows
	for i := 0; i < rowsToWrite; i++ {
		vals := make([]string, len(columns))
		for j, col := range columns {
			vals[j] = formatValue(data[i][col])
		}
		writeMarkdownRow(writer, vals, widths)
	}

	return nil
}

func writeMarkdownRow(w io.Writer, values []string, widths []int) {
	parts := make([]string, len(values))
	for i, v := range values {
		pad := widths[i] - runewidth.StringWidth(v)
		if pad < 0 {
			pad = 0
		}
		parts[i] = v + strings.Repeat(" ", pad)
	}
	fmt.Fprintf(w, "| %s |\n", strings.Join(parts, " | "))
}
