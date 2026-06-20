package chart

import (
	"fmt"
	"math"
	"os"
	"strings"
)

func renderMermaid(seriesList []Series, opts Options) error {
	if len(seriesList) == 0 {
		return fmt.Errorf("차트를 생성할 데이터가 없습니다")
	}

	writer := os.Stdout
	if opts.Output != "" {
		f, err := os.Create(opts.Output)
		if err != nil {
			return fmt.Errorf("파일 생성 실패: %w", err)
		}
		defer f.Close()
		writer = f
	}

	switch opts.Type {
	case Pie:
		return renderMermaidPie(writer, seriesList, opts)
	default:
		return renderMermaidXY(writer, seriesList, opts)
	}
}

// renderMermaidXY renders line or bar chart as Mermaid xychart-beta
func renderMermaidXY(w *os.File, seriesList []Series, opts Options) error {
	title := opts.Title
	if title == "" && len(seriesList) == 1 {
		title = seriesList[0].Name
	}

	// X-axis labels from first series
	labels := seriesList[0].Labels
	if len(labels) == 0 {
		return fmt.Errorf("X축 라벨이 없습니다")
	}

	// Calculate Y-axis range across all series
	yMin, yMax := math.MaxFloat64, -math.MaxFloat64
	for _, s := range seriesList {
		for _, v := range s.Values {
			if v < yMin {
				yMin = v
			}
			if v > yMax {
				yMax = v
			}
		}
	}
	// Add 5% padding
	yRange := yMax - yMin
	if yRange == 0 {
		yRange = 1
	}
	yMin = yMin - yRange*0.05
	yMax = yMax + yRange*0.05

	fmt.Fprintln(w, "```mermaid")
	fmt.Fprintln(w, "xychart-beta")
	if title != "" {
		fmt.Fprintf(w, "    title \"%s\"\n", escapeMermaid(title))
	}

	// X-axis
	fmt.Fprintf(w, "    x-axis [%s]\n", joinQuoted(labels))

	// Y-axis with range
	yLabel := opts.YLabel
	if yLabel == "" {
		yLabel = "값"
	}
	fmt.Fprintf(w, "    y-axis \"%s\" %.0f --> %.0f\n", escapeMermaid(yLabel), yMin, yMax)

	// Series
	for _, s := range seriesList {
		chartType := "line"
		if opts.Type == Bar {
			chartType = "bar"
		}

		// Format values
		vals := make([]string, len(s.Values))
		for i, v := range s.Values {
			if v == float64(int64(v)) {
				vals[i] = fmt.Sprintf("%.0f", v)
			} else {
				vals[i] = fmt.Sprintf("%.2f", v)
			}
		}

		// Mermaid xychart doesn't support series names in the syntax,
		// so add a comment for multi-series
		if len(seriesList) > 1 && s.Name != "" {
			fmt.Fprintf(w, "    %% %s\n", s.Name)
		}
		fmt.Fprintf(w, "    %s [%s]\n", chartType, strings.Join(vals, ", "))
	}

	fmt.Fprintln(w, "```")

	// If multiple series, add a legend note since Mermaid xychart doesn't support legend
	if len(seriesList) > 1 {
		fmt.Fprintln(w)
		fmt.Fprint(w, "> 범례: ")
		names := make([]string, len(seriesList))
		for i, s := range seriesList {
			names[i] = s.Name
		}
		fmt.Fprintln(w, strings.Join(names, " / "))
	}

	if opts.Output != "" {
		fmt.Fprintf(os.Stderr, "✓ Mermaid 차트가 %s로 저장되었습니다.\n", opts.Output)
	}
	return nil
}

// renderMermaidPie renders pie chart as Mermaid pie
func renderMermaidPie(w *os.File, seriesList []Series, opts Options) error {
	s := seriesList[0]

	title := opts.Title
	if title == "" {
		title = s.Name
	}

	fmt.Fprintln(w, "```mermaid")
	if title != "" {
		fmt.Fprintf(w, "pie title %s\n", escapeMermaid(title))
	} else {
		fmt.Fprintln(w, "pie")
	}

	for i, v := range s.Values {
		label := ""
		if i < len(s.Labels) {
			label = s.Labels[i]
		} else {
			label = fmt.Sprintf("항목%d", i+1)
		}
		if v == float64(int64(v)) {
			fmt.Fprintf(w, "    \"%s\" : %.0f\n", escapeMermaid(label), v)
		} else {
			fmt.Fprintf(w, "    \"%s\" : %.2f\n", escapeMermaid(label), v)
		}
	}

	fmt.Fprintln(w, "```")

	if opts.Output != "" {
		fmt.Fprintf(os.Stderr, "✓ Mermaid 차트가 %s로 저장되었습니다.\n", opts.Output)
	}
	return nil
}

func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	return s
}

func joinQuoted(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("\"%s\"", escapeMermaid(item))
	}
	return strings.Join(quoted, ", ")
}
