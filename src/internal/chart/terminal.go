package chart

import (
	"fmt"
	"os"
	"strings"

	"github.com/guptarohit/asciigraph"
)

// ANSI colors for multi-series
var seriesColors = []asciigraph.AnsiColor{
	asciigraph.Red,
	asciigraph.Green,
	asciigraph.Yellow,
	asciigraph.Blue,
	asciigraph.Cyan,
	asciigraph.White,
}

func renderTerminal(seriesList []Series, opts Options) error {
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	height := opts.Height
	if height <= 0 {
		height = 15
	}

	// Filter out empty series
	var validSeries []Series
	for _, s := range seriesList {
		if len(s.Values) > 0 {
			validSeries = append(validSeries, s)
		}
	}
	if len(validSeries) == 0 {
		return nil
	}

	// Downsample if needed
	for i, s := range validSeries {
		if len(s.Values) > 200 {
			fmt.Fprintf(os.Stderr, "참고: 데이터가 %d건으로 많아 200건으로 다운샘플링하여 표시합니다.\n", len(s.Values))
			validSeries[i] = downsample(s, 200)
		}
	}

	// Single series: use Plot
	if len(validSeries) == 1 {
		return renderSingleSeries(validSeries[0], opts, width, height)
	}

	// Multiple series: use PlotMany for overlay chart
	return renderMultiSeries(validSeries, opts, width, height)
}

func renderSingleSeries(s Series, opts Options, width, height int) error {
	title := opts.Title
	if title == "" && s.Name != "" {
		title = s.Name
	}

	caption := buildCaption(s.Labels)

	// Print title at top
	if title != "" {
		fmt.Println()
		fmt.Printf("  %s\n\n", title)
	}

	plotOpts := []asciigraph.Option{
		asciigraph.Height(height),
		asciigraph.Width(width),
	}

	graph := asciigraph.Plot(s.Values, plotOpts...)
	fmt.Println(graph)

	if caption != "" {
		fmt.Println(caption)
	}
	return nil
}

func renderMultiSeries(seriesList []Series, opts Options, width, height int) error {
	// Build data for PlotMany
	data := make([][]float64, len(seriesList))
	for i, s := range seriesList {
		data[i] = s.Values
	}

	// Build legend names
	legends := make([]string, len(seriesList))
	for i, s := range seriesList {
		if s.Name != "" {
			legends[i] = s.Name
		} else {
			legends[i] = fmt.Sprintf("시리즈 %d", i+1)
		}
	}

	// Build color list
	colors := make([]asciigraph.AnsiColor, len(seriesList))
	for i := range seriesList {
		colors[i] = seriesColors[i%len(seriesColors)]
	}

	title := opts.Title
	caption := buildCaption(seriesList[0].Labels)

	// Print title at top
	if title != "" {
		fmt.Println()
		fmt.Printf("  %s\n\n", title)
	}

	plotOpts := []asciigraph.Option{
		asciigraph.Height(height),
		asciigraph.Width(width),
		asciigraph.SeriesColors(colors...),
		asciigraph.SeriesLegends(legends...),
	}

	graph := asciigraph.PlotMany(data, plotOpts...)
	fmt.Println(graph)

	if caption != "" {
		fmt.Println(caption)
	}

	return nil
}

// buildCaption creates an x-axis label line from labels.
func buildCaption(labels []string) string {
	if len(labels) == 0 {
		return ""
	}

	// Show up to 10 labels evenly spaced
	maxLabels := 10
	if len(labels) <= maxLabels {
		return "          " + strings.Join(labels, "  ")
	}

	step := len(labels) / (maxLabels - 1)
	var selected []string
	for i := 0; i < len(labels); i += step {
		selected = append(selected, labels[i])
	}
	// Always include the last label
	if selected[len(selected)-1] != labels[len(labels)-1] {
		selected = append(selected, labels[len(labels)-1])
	}
	return "          " + strings.Join(selected, "  ")
}

// downsample reduces a series to the target number of points by evenly sampling.
func downsample(s Series, target int) Series {
	n := len(s.Values)
	if n <= target {
		return s
	}
	result := Series{Name: s.Name}
	step := float64(n-1) / float64(target-1)
	for i := 0; i < target; i++ {
		idx := int(float64(i)*step + 0.5)
		if idx >= n {
			idx = n - 1
		}
		result.Values = append(result.Values, s.Values[idx])
		if idx < len(s.Labels) {
			result.Labels = append(result.Labels, s.Labels[idx])
		}
	}
	return result
}
