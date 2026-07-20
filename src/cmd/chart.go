package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/clazic/kosis/internal/chart"
	"github.com/spf13/cobra"
)

var chartCmd = &cobra.Command{
	Use:   "chart [flags]",
	Short: "데이터를 차트로 시각화",
	Long: `데이터를 차트로 시각화

stdin(파이프)이나 파일에서 JSON/CSV 데이터를 읽어
터미널, 이미지(PNG/SVG/PDF), HTML, Excel, Mermaid 차트로 출력합니다.

참고: kosis data ... --chart line 으로도 직접 차트를 생성할 수 있습니다.

사용법:
  kosis chart [flags]
  kosis data ... -f json | kosis chart --type line
  kosis chart --input data.json --type bar --format png -o chart.png

입력 플래그:
  -i, --input <파일>       입력 파일 (JSON/CSV). 미지정시 stdin

차트 설정:
  -t, --type <타입>        차트 타입: line(기본), bar, pie
  --format <포맷>          출력 포맷: terminal(기본), png, svg, pdf, html, excel, mermaid

출력 플래그:
  -o, --output <파일>      출력 파일 경로 (이미지/HTML/Excel 필수)
  --title <제목>           차트 제목
  --subtitle <부제>        차트 부제목
  --source <출처>          출처 표시
  --note <주석>            주석 표시
  --template <이름>        HTML 템플릿 (line-chart, comparison, bar-rank, ...)
  --width <너비>           차트 너비 (px)
  --height <높이>          차트 높이 (px)
  --open                   생성 후 브라우저/뷰어로 자동 열기`,

	Example: `  # 파이프: 데이터 조회 후 터미널 차트
  kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 -f json | kosis chart

  # 파이프: HTML 인터랙티브 차트
  kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 -f json | kosis chart --format html -o pop.html --open

  # 파일 입력: PNG 이미지
  kosis chart -i data.json -t bar --format png -o chart.png

  # 파일 입력: Excel 차트
  kosis chart -i data.json --format excel -o chart.xlsx

  # Mermaid 출력 (Markdown 삽입용)
  kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 5 -f json | kosis chart --format mermaid

HTML 템플릿:
  line-chart       시계열 추이
  comparison       비교 분석 (차트+테이블+비중)
  bar-rank         항목별 순위 (가로 막대)
  bar-vertical     세로 막대 비교
  stacked-bar      누적 막대 (구성비)
  pie-share        구성비 (도넛 차트)
  area             영역 차트
  dual-axis        이중 Y축
  heatmap          히트맵
  treemap          트리맵
  dashboard        대시보드 (여러 차트 한 페이지)

관련 명령어:
  kosis data ... --chart line    데이터 조회와 차트를 한 번에`,

	Run: runChart,
}

var (
	chartInputFlag    string
	chartTypeFlag     string
	chartFormatFlag   string
	chartTitleFlag    string
	chartWidthFlag    int
	chartHeightFlag   int
	chartOutputFlag   string
	chartOpenFlag     bool
	chartTemplateFlag string
	chartSubtitleFlag string
	chartSourceFlag   string
	chartNoteFlag     string
)

func runChart(cmd *cobra.Command, args []string) {
	// Parse chart type
	chartType, err := chart.ParseChartType(chartTypeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	// Parse chart format
	chartFormat, err := chart.ParseChartFormat(chartFormatFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	// Read input data
	data, err := readChartInput(chartInputFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	if len(data) == 0 {
		fmt.Fprintln(os.Stderr, "오류: 입력 데이터가 비어 있습니다")
		return
	}

	// Extract series and axis info from data
	seriesList, axisInfo := chart.ExtractSeriesWithAxis(data)
	if len(seriesList) == 0 {
		fmt.Fprintln(os.Stderr, "오류: 차트를 생성할 수 있는 데이터가 없습니다")
		return
	}

	// Use output flag from chart or global
	output := chartOutputFlag
	if output == "" {
		output = outputFlag
	}

	opts := chart.Options{
		Type:     chartType,
		Format:   chartFormat,
		Title:    chartTitleFlag,
		Width:    chartWidthFlag,
		Height:   chartHeightFlag,
		Output:   output,
		Open:     chartOpenFlag,
		XLabel:   axisInfo.XLabel,
		YLabel:   axisInfo.YLabel,
		Template: chartTemplateFlag,
		Subtitle: chartSubtitleFlag,
		Source:   chartSourceFlag,
		Note:     chartNoteFlag,
	}

	if err := chart.Render(seriesList, opts); err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	if output != "" && chartFormat != chart.FormatTerminal {
		fmt.Fprintf(os.Stderr, "✓ 차트가 %s로 저장되었습니다.\n", output)
	}
}

// readChartInput reads JSON or CSV data from file or stdin.
func readChartInput(inputPath string) ([]map[string]interface{}, error) {
	var reader io.Reader

	if inputPath != "" {
		f, err := os.Open(inputPath)
		if err != nil {
			return nil, fmt.Errorf("파일 열기 실패: %w", err)
		}
		defer f.Close()
		reader = f

		// Detect format from extension
		if strings.HasSuffix(strings.ToLower(inputPath), ".csv") {
			return readCSV(reader)
		}
		return readJSON(reader)
	}

	// Read from stdin
	reader = os.Stdin
	return readJSON(reader)
}

func readJSON(r io.Reader) ([]map[string]interface{}, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("데이터 읽기 실패: %w", err)
	}

	// Trim whitespace
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, fmt.Errorf("입력 데이터가 비어 있습니다")
	}

	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		// Try single object
		var single map[string]interface{}
		if err2 := json.Unmarshal([]byte(trimmed), &single); err2 == nil {
			return []map[string]interface{}{single}, nil
		}
		return nil, fmt.Errorf("JSON 파싱 실패: %w", err)
	}

	return result, nil
}

func readCSV(r io.Reader) ([]map[string]interface{}, error) {
	csvReader := csv.NewReader(r)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 파싱 실패: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV 데이터가 부족합니다 (헤더 + 최소 1행)")
	}

	headers := records[0]
	var result []map[string]interface{}

	for _, row := range records[1:] {
		m := make(map[string]interface{})
		for j, header := range headers {
			if j < len(row) {
				m[header] = row[j]
			}
		}
		result = append(result, m)
	}

	return result, nil
}

func init() {
	rootCmd.AddCommand(chartCmd)

	chartCmd.Flags().StringVarP(&chartInputFlag, "input", "i", "", "[입력] 읽어들일 데이터 파일 (JSON/CSV). 미지정 시 stdin(파이프)에서 읽음")
	chartCmd.Flags().StringVarP(&chartTypeFlag, "type", "t", "line", "[차트] 차트 종류: line=시계열 추이, bar=항목 비교, pie=구성비")
	chartCmd.Flags().StringVar(&chartFormatFlag, "format", "terminal", "[출력] 렌더링 포맷: terminal=터미널 텍스트, png/svg/pdf=이미지 파일, html=인터랙티브, excel=차트 포함 xlsx, mermaid=Markdown 코드")
	chartCmd.Flags().StringVarP(&chartOutputFlag, "output", "o", "", "[출력] 저장 파일 경로. terminal/mermaid 외 포맷에서는 필수")
	chartCmd.Flags().StringVar(&chartTitleFlag, "title", "", "[꾸미기] 차트 제목 (상단 중앙)")
	chartCmd.Flags().IntVar(&chartWidthFlag, "width", 0, "[크기] 차트 너비(px). 0=포맷별 기본값. 이미지/HTML에만 적용")
	chartCmd.Flags().IntVar(&chartHeightFlag, "height", 0, "[크기] 차트 높이(px). 0=포맷별 기본값. 이미지/HTML에만 적용")
	chartCmd.Flags().BoolVar(&chartOpenFlag, "open", false, "[출력] 생성 후 기본 브라우저/뷰어로 자동 열기")
	chartCmd.Flags().StringVar(&chartTemplateFlag, "template", "", "[차트] html 포맷 전용 템플릿: line-chart, comparison, bar-rank, bar-vertical, stacked-bar, pie-share, area, dual-axis, heatmap, treemap, dashboard")
	chartCmd.Flags().StringVar(&chartSubtitleFlag, "subtitle", "", "[꾸미기] 부제목 (제목 아래)")
	chartCmd.Flags().StringVar(&chartSourceFlag, "source", "", "[꾸미기] 하단 출처 문구 (예: \"자료: KOSIS\")")
	chartCmd.Flags().StringVar(&chartNoteFlag, "note", "", "[꾸미기] 하단 주석 문구")
}
