package chart

import (
	"fmt"
	"os"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// renderHTML generates an HTML chart file using the go-echarts library.
// Note: XSS safety — go-echarts renders data via JSON serialization into JavaScript,
// which inherently escapes HTML special characters. No additional escaping is needed
// as long as data values are not inserted as raw HTML.
func renderHTML(seriesList []Series, chartOpts Options) error {
	if chartOpts.Output == "" {
		return fmt.Errorf("HTML 차트는 -o/--output 파일 경로가 필요합니다")
	}

	title := chartOpts.Title
	if title == "" && len(seriesList) > 0 {
		title = seriesList[0].Name
	}

	width := fmt.Sprintf("%dpx", chartOpts.Width)
	if chartOpts.Width <= 0 {
		width = "1200px"
	}
	height := fmt.Sprintf("%dpx", chartOpts.Height)
	if chartOpts.Height <= 0 {
		height = "600px"
	}

	f, err := os.Create(chartOpts.Output)
	if err != nil {
		return fmt.Errorf("파일 생성 실패: %w", err)
	}
	defer f.Close()

	xLabel := chartOpts.XLabel
	yLabel := chartOpts.YLabel

	switch chartOpts.Type {
	case Line:
		err = renderHTMLLine(f, seriesList, title, width, height, xLabel, yLabel)
	case Bar:
		err = renderHTMLBar(f, seriesList, title, width, height, xLabel, yLabel)
	case Pie:
		err = renderHTMLPie(f, seriesList, title, width, height, xLabel, yLabel)
	default:
		err = renderHTMLLine(f, seriesList, title, width, height, xLabel, yLabel)
	}

	if err != nil {
		// 렌더링 실패 시 불완전한 파일 제거
		f.Close()
		os.Remove(chartOpts.Output)
		return err
	}

	if chartOpts.Open {
		return openFile(chartOpts.Output)
	}
	return nil
}

func renderHTMLLine(f *os.File, seriesList []Series, title, width, height, xLabel, yLabel string) error {
	line := charts.NewLine()
	globalOpts := []charts.GlobalOpts{
		charts.WithTitleOpts(opts.Title{Title: title, Left: "center"}),
		charts.WithInitializationOpts(opts.Initialization{Width: width, Height: height}),
		charts.WithLegendOpts(opts.Legend{Right: "10%", Top: "5%"}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
		charts.WithToolboxOpts(opts.Toolbox{Show: opts.Bool(true)}),
		charts.WithDataZoomOpts(opts.DataZoom{Type: "slider"}),
	}
	if xLabel != "" {
		globalOpts = append(globalOpts, charts.WithXAxisOpts(opts.XAxis{Name: xLabel}))
	}
	if yLabel != "" {
		globalOpts = append(globalOpts, charts.WithYAxisOpts(opts.YAxis{Name: yLabel}))
	}
	line.SetGlobalOptions(globalOpts...)

	// Use first series' labels as x-axis
	if len(seriesList) > 0 {
		line.SetXAxis(seriesList[0].Labels)
	}

	for _, s := range seriesList {
		items := make([]opts.LineData, len(s.Values))
		for i, v := range s.Values {
			items[i] = opts.LineData{Value: v}
		}
		name := s.Name
		if name == "" {
			name = "데이터"
		}
		line.AddSeries(name, items)
	}

	return line.Render(f)
}

func renderHTMLBar(f *os.File, seriesList []Series, title, width, height, xLabel, yLabel string) error {
	bar := charts.NewBar()
	globalOpts := []charts.GlobalOpts{
		charts.WithTitleOpts(opts.Title{Title: title, Left: "center"}),
		charts.WithInitializationOpts(opts.Initialization{Width: width, Height: height}),
		charts.WithLegendOpts(opts.Legend{Right: "10%", Top: "5%"}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
		charts.WithToolboxOpts(opts.Toolbox{Show: opts.Bool(true)}),
	}
	if xLabel != "" {
		globalOpts = append(globalOpts, charts.WithXAxisOpts(opts.XAxis{Name: xLabel}))
	}
	if yLabel != "" {
		globalOpts = append(globalOpts, charts.WithYAxisOpts(opts.YAxis{Name: yLabel}))
	}
	bar.SetGlobalOptions(globalOpts...)

	if len(seriesList) > 0 {
		bar.SetXAxis(seriesList[0].Labels)
	}

	for _, s := range seriesList {
		items := make([]opts.BarData, len(s.Values))
		for i, v := range s.Values {
			items[i] = opts.BarData{Value: v}
		}
		name := s.Name
		if name == "" {
			name = "데이터"
		}
		bar.AddSeries(name, items)
	}

	return bar.Render(f)
}

func renderHTMLPie(f *os.File, seriesList []Series, title, width, height, xLabel, yLabel string) error {
	pie := charts.NewPie()
	pie.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: title, Left: "center"}),
		charts.WithInitializationOpts(opts.Initialization{Width: width, Height: height}),
		charts.WithLegendOpts(opts.Legend{Right: "10%", Top: "5%", Orient: "vertical"}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "item"}),
		charts.WithToolboxOpts(opts.Toolbox{Show: opts.Bool(true)}),
	)

	// Use first series for pie chart
	if len(seriesList) > 0 {
		s := seriesList[0]
		items := make([]opts.PieData, len(s.Values))
		for i, v := range s.Values {
			label := ""
			if i < len(s.Labels) {
				label = s.Labels[i]
			}
			items[i] = opts.PieData{Name: label, Value: v}
		}
		pie.AddSeries(s.Name, items)
	}

	return pie.Render(f)
}
