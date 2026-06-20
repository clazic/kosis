package output

import (
	"bytes"
	"fmt"
	"os"
)

// WriteToFile formats data and writes it to a file based on the file extension.
// The format is automatically detected from the file extension.
func WriteToFile(data []map[string]interface{}, outputPath string, opts FormatOptions) error {
	// Detect format from file extension
	format := DetectFormat(outputPath)

	// Get the appropriate formatter
	formatter, err := NewFormatter(format)
	if err != nil {
		return fmt.Errorf("unsupported format '%s': %w", format, err)
	}

	// Formats that manage their own file I/O (xlsx, sqlite, parquet) receive
	// the file path instead of an open file handle to avoid file descriptor conflicts.
	switch format {
	case "xlsx", "sqlite", "parquet":
		opts.FilePath = outputPath
	default:
		// Stream-based formats (csv, json, table) write to an io.Writer
		file, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", outputPath, err)
		}
		defer file.Close()
		opts.Writer = file
	}

	// Format and write data
	if err := formatter.Format(data, opts); err != nil {
		return fmt.Errorf("failed to format data as %s: %w", format, err)
	}

	return nil
}

// FormatToString formats data to a string and returns it.
// Useful for testing and debugging.
func FormatToString(data []map[string]interface{}, format string, opts FormatOptions) (string, error) {
	buf := new(bytes.Buffer)
	opts.Writer = buf

	formatter, err := NewFormatter(format)
	if err != nil {
		return "", err
	}

	if err := formatter.Format(data, opts); err != nil {
		return "", err
	}

	return buf.String(), nil
}
