package chart

import (
	"fmt"
	"image/color"
	"math"
	"os"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

func renderImage(seriesList []Series, opts Options) error {
	if opts.Output == "" {
		return fmt.Errorf("이미지 차트는 -o/--output 파일 경로가 필요합니다")
	}

	p := plot.New()
	p.Title.Text = opts.Title
	if p.Title.Text == "" && len(seriesList) > 0 {
		p.Title.Text = seriesList[0].Name
	}

	// 축 이름 설정
	if opts.XLabel != "" {
		p.X.Label.Text = opts.XLabel
	}
	if opts.YLabel != "" {
		p.Y.Label.Text = opts.YLabel
	}

	// 범례를 우측 상단으로
	p.Legend.Top = true
	p.Legend.Left = true
	p.Title.Padding = vg.Points(10)

	var width vg.Length
	if opts.Width <= 0 {
		width = 10 * vg.Inch
	} else {
		width = vg.Length(opts.Width) * vg.Centimeter
	}
	var height vg.Length
	if opts.Height <= 0 {
		height = 6 * vg.Inch
	} else {
		height = vg.Length(opts.Height) * vg.Centimeter
	}

	switch opts.Type {
	case Line:
		if err := addLinePlot(p, seriesList); err != nil {
			return err
		}
	case Bar:
		if err := addBarPlot(p, seriesList); err != nil {
			return err
		}
	case Pie:
		// Pie chart: gonum/plot does not support pie charts natively
		fmt.Fprintf(os.Stderr, "참고: gonum/plot은 파이 차트를 지원하지 않아 막대 차트로 대체합니다.\n")
		if err := addBarPlot(p, seriesList[:1]); err != nil {
			return err
		}
	}

	if err := p.Save(width, height, opts.Output); err != nil {
		return fmt.Errorf("차트 저장 실패: %w", err)
	}

	if opts.Open {
		return openFile(opts.Output)
	}
	return nil
}

func addLinePlot(p *plot.Plot, seriesList []Series) error {
	// Set x-axis labels from the first series
	if len(seriesList) > 0 && len(seriesList[0].Labels) > 0 {
		labels := seriesList[0].Labels
		p.NominalX(labels...)
	}

	for i, s := range seriesList {
		if len(s.Values) == 0 {
			continue
		}

		pts := make(plotter.XYs, len(s.Values))
		for j, v := range s.Values {
			pts[j].X = float64(j)
			pts[j].Y = v
		}

		name := s.Name
		if name == "" {
			name = fmt.Sprintf("시리즈 %d", i+1)
		}

		if len(s.Values) == 1 {
			// Single data point: use scatter instead of line
			scatter, err := plotter.NewScatter(pts)
			if err != nil {
				return fmt.Errorf("스캐터 생성 실패: %w", err)
			}
			scatter.GlyphStyle.Color = plotutil.Color(i)
			scatter.GlyphStyle.Radius = vg.Points(4)
			scatter.GlyphStyle.Shape = draw.CircleGlyph{}
			p.Add(scatter)
			p.Legend.Add(name, scatter)
		} else {
			line, err := plotter.NewLine(pts)
			if err != nil {
				return fmt.Errorf("라인 생성 실패: %w", err)
			}
			line.Color = plotutil.Color(i)
			line.Width = vg.Points(2)
			p.Add(line)
			p.Legend.Add(name, line)
		}
	}

	return nil
}

func addBarPlot(p *plot.Plot, seriesList []Series) error {
	if len(seriesList) == 0 {
		return nil
	}

	// Set x-axis labels
	if len(seriesList[0].Labels) > 0 {
		p.NominalX(seriesList[0].Labels...)
	}

	barWidth := vg.Points(20)
	if len(seriesList) > 1 {
		barWidth = vg.Points(float64(40) / float64(len(seriesList)))
	}

	for i, s := range seriesList {
		if len(s.Values) == 0 {
			continue
		}
		values := make(plotter.Values, len(s.Values))
		for j, v := range s.Values {
			values[j] = v
		}

		bar, err := plotter.NewBarChart(values, barWidth)
		if err != nil {
			return fmt.Errorf("바 차트 생성 실패: %w", err)
		}

		bar.Color = getBarColor(i)
		if len(seriesList) > 1 {
			bar.Offset = vg.Points(float64(i-(len(seriesList)-1)/2) * float64(barWidth)/float64(vg.Points(1)))
		}

		name := s.Name
		if name == "" {
			name = fmt.Sprintf("시리즈 %d", i+1)
		}
		p.Add(bar)
		p.Legend.Add(name, bar)
	}

	return nil
}

var barColors = []color.RGBA{
	{R: 66, G: 133, B: 244, A: 255},  // Blue
	{R: 234, G: 67, B: 53, A: 255},   // Red
	{R: 52, G: 168, B: 83, A: 255},   // Green
	{R: 251, G: 188, B: 4, A: 255},   // Yellow
	{R: 153, G: 102, B: 204, A: 255}, // Purple
	{R: 255, G: 152, B: 0, A: 255},   // Orange
}

func getBarColor(i int) color.RGBA {
	return barColors[int(math.Abs(float64(i)))%len(barColors)]
}
