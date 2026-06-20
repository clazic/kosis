package chart

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TemplateData is the JSON structure injected into HTML templates.
type TemplateData struct {
	Labels     []string         `json:"labels"`
	Unit       string           `json:"unit"`
	Series     []TemplateSeries `json:"series"`
	ChartType  string           `json:"chartType,omitempty"`
	ShowRatio  bool             `json:"showRatio,omitempty"`
	MarkLine   *MarkLine        `json:"markLine,omitempty"`
	Charts     []ChartItem      `json:"charts,omitempty"`
	Categories []string         `json:"categories,omitempty"`
	Unit2      string           `json:"unit2,omitempty"`
}

// ChartItem is for dashboard template (multiple charts in one page).
type ChartItem struct {
	Type   string           `json:"type"`
	Title  string           `json:"title"`
	Labels []string         `json:"labels"`
	Unit   string           `json:"unit"`
	Series []TemplateSeries `json:"series"`
}

type TemplateSeries struct {
	Name   string     `json:"name"`
	Values []*float64 `json:"values"`
}

type MarkLine struct {
	XAxis string `json:"xAxis"`
	Label string `json:"label"`
}

// findTemplate searches for a template file in known locations.
// Search order: absolute path → cwd → skill template dirs
func findTemplate(name string) (string, error) {
	// 1. Absolute path or relative to cwd
	if filepath.IsAbs(name) || strings.Contains(name, string(filepath.Separator)) {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
	}

	// 2. Add .html extension if not present
	if !strings.HasSuffix(name, ".html") {
		name = name + ".html"
	}

	// 3. Check current directory
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}

	// 4. Check known skill template directories
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".claude", "skills", "kosis-cli", "templates"),
		filepath.Join(home, ".gemini", "skills", "kosis-cli", "templates"),
		filepath.Join(home, ".codex", "skills", "kosis-cli", "templates"),
	}

	for _, dir := range dirs {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("템플릿 '%s'을 찾을 수 없습니다. 사용 가능: line-chart, comparison, bar-rank, pie-share", name)
}

// seriesToTemplateData converts chart Series to TemplateData.
func seriesToTemplateData(seriesList []Series, opts Options) TemplateData {
	data := TemplateData{
		Unit:      opts.YLabel,
		ShowRatio: len(seriesList) == 2,
	}

	// Chart type mapping
	switch opts.Type {
	case Bar:
		data.ChartType = "bar"
	default:
		data.ChartType = "line"
	}

	// Dashboard: each series becomes a separate chart
	if opts.Template == "dashboard" {
		labels := []string{}
		if len(seriesList) > 0 {
			labels = seriesList[0].Labels
		}
		data.Labels = labels
		for _, s := range seriesList {
			ts := TemplateSeries{Name: s.Name}
			for _, v := range s.Values {
				val := v
				ts.Values = append(ts.Values, &val)
			}
			data.Charts = append(data.Charts, ChartItem{
				Type:   data.ChartType,
				Title:  s.Name,
				Labels: labels,
				Unit:   data.Unit,
				Series: []TemplateSeries{ts},
			})
			data.Series = append(data.Series, ts)
		}
		return data
	}

	// Heatmap: convert series to [[rowIdx, colIdx, value]] format
	if opts.Template == "heatmap" && len(seriesList) > 1 {
		labels := []string{}
		if len(seriesList) > 0 {
			labels = seriesList[0].Labels
		}
		data.Labels = labels
		categories := make([]string, len(seriesList))
		var heatValues [][3]interface{}
		for i, s := range seriesList {
			categories[i] = s.Name
			ts := TemplateSeries{Name: s.Name}
			for j, v := range s.Values {
				val := v
				ts.Values = append(ts.Values, &val)
				heatValues = append(heatValues, [3]interface{}{i, j, v})
			}
			data.Series = append(data.Series, ts)
		}
		data.Categories = categories
		// Override series with heatmap format
		heatSeries := TemplateSeries{Name: ""}
		for _, hv := range heatValues {
			if f, ok := hv[2].(float64); ok {
				heatSeries.Values = append(heatSeries.Values, &f)
			}
		}
		// Store raw heatmap data as the series values field won't work directly.
		// Instead, keep the regular series for the table and let the template JS handle heatmap rendering.
		return data
	}

	// Pie chart: flatten multiple series into one series with labels=names, values=first value
	if opts.Type == Pie && len(seriesList) > 1 {
		labels := make([]string, len(seriesList))
		vals := make([]*float64, len(seriesList))
		for i, s := range seriesList {
			labels[i] = s.Name
			if len(s.Values) > 0 {
				v := s.Values[0]
				vals[i] = &v
			}
		}
		data.Labels = labels
		data.Series = []TemplateSeries{{Name: "", Values: vals}}
		return data
	}

	// Use labels from first series
	if len(seriesList) > 0 {
		data.Labels = seriesList[0].Labels
	}

	// Convert series
	for _, s := range seriesList {
		ts := TemplateSeries{Name: s.Name}
		for _, v := range s.Values {
			val := v
			ts.Values = append(ts.Values, &val)
		}
		data.Series = append(data.Series, ts)
	}

	return data
}

func renderTemplate(seriesList []Series, opts Options) error {
	if opts.Output == "" {
		return fmt.Errorf("템플릿 차트는 -o/--output 파일 경로가 필요합니다")
	}
	if opts.Template == "" {
		// Default template based on chart type
		switch opts.Type {
		case Pie:
			opts.Template = "pie-share"
		case Bar:
			opts.Template = "bar-rank"
		default:
			if len(seriesList) > 1 {
				opts.Template = "comparison"
			} else {
				opts.Template = "line-chart"
			}
		}
	}

	// Find template file
	tplPath, err := findTemplate(opts.Template)
	if err != nil {
		return err
	}

	// Read template
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		return fmt.Errorf("템플릿 읽기 실패: %w", err)
	}
	html := string(tplBytes)

	// Build template data JSON
	tplData := seriesToTemplateData(seriesList, opts)
	dataJSON, err := json.Marshal(tplData)
	if err != nil {
		return fmt.Errorf("데이터 JSON 변환 실패: %w", err)
	}

	// Replace placeholders
	title := opts.Title
	if title == "" && len(seriesList) > 0 {
		title = seriesList[0].Name
	}

	html = strings.ReplaceAll(html, "{{TITLE}}", title)
	html = strings.ReplaceAll(html, "{{SUBTITLE}}", opts.Subtitle)
	html = strings.ReplaceAll(html, "{{SOURCE}}", opts.Source)
	html = strings.ReplaceAll(html, "{{DATA}}", string(dataJSON))

	// Handle NOTE
	if opts.Note != "" {
		html = strings.ReplaceAll(html, "<!-- {{NOTE_START}}", "")
		html = strings.ReplaceAll(html, "{{NOTE_END}} -->", "")
		html = strings.ReplaceAll(html, "{{NOTE}}", opts.Note)
	}

	// Write output
	if err := os.WriteFile(opts.Output, []byte(html), 0644); err != nil {
		return fmt.Errorf("파일 저장 실패: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ 템플릿 차트가 %s로 저장되었습니다. (템플릿: %s)\n", opts.Output, filepath.Base(tplPath))

	if opts.Open {
		return openFile(opts.Output)
	}
	return nil
}
