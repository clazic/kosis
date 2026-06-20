package tui

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/bookmark"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/history"
	"github.com/muesli/termenv"
)

// Panel은 현재 포커스된 패널을 나타냅니다.
type Panel int

const (
	PanelSearch Panel = iota
	PanelMeta
	PanelParams
	PanelResult
)

// SearchItem은 검색 결과의 각 아이템을 나타냅니다.
type SearchItem struct {
	OrgID string
	OrgNM string
	TblID string
	TblNM string
}

// TableInfo는 선택된 통계표의 정보입니다.
type TableInfo struct {
	OrgID string
	TblID string
	TblNM string
}

// ClassInfo는 분류 정보입니다.
type ClassInfo struct {
	Code string
	Name string
}

// ItemInfo는 항목 정보입니다.
type ItemInfo struct {
	Code string
	Name string
}

// MetaInfo는 통계표의 메타정보입니다.
type MetaInfo struct {
	Classifications []ClassInfo
	Items           []ItemInfo
	PeriodInfo      string
	NumClassGroups  int               // 분류 그룹 수 (objL1, objL2... 몇 개인지)
	ClassGroupNames map[int]string    // 분류 그룹 번호(1~8) → 그룹명 (예: "시도별", "산업별")
	ColumnMeta      *api.ColumnMeta   // 컬럼 메타정보 (실제 컬럼명)
}

// 비동기 메시지 타입들
type searchResultMsg struct {
	results []SearchItem
	err     error
}

type metaResultMsg struct {
	meta *MetaInfo
	err  error
}

type dataResultMsg struct {
	data []api.DataRow
	err  error
}

// Model은 TUI 메인 모델입니다.
type Model struct {
	width, height int          // 터미널 크기
	activePanel   Panel        // 현재 포커스된 패널
	searchInput   string       // 검색어 입력
	textInput     textinput.Model // 텍스트 입력 컴포넌트
	searchResults []SearchItem // 검색 결과 목록
	selectedTable *TableInfo   // 선택된 통계표
	metaInfo      *MetaInfo    // 메타 정보
	resultData    []api.DataRow // 조회 결과 데이터
	statusMsg     string       // 상태바 메시지
	loading       bool         // 로딩 상태
	err           error        // 에러 메시지
	quitting      bool         // 종료 플래그
	typing        bool         // 입력 모드 여부
	cursor        int          // 현재 선택 인덱스
	client        *api.Client  // API 클라이언트
	bookmarks     []bookmark.Bookmark // 즐겨찾기 목록
	histories     []history.HistoryEntry // 최근 조회 이력
	paramClass1   string       // 분류1 파라미터
	paramItem     string       // 항목 파라미터
	paramPeriod   string       // 주기 파라미터
	paramLatest   string       // 최근 N개 파라미터
	resultScrollX int          // 결과 테이블 가로 스크롤 오프셋
	metaScrollY   int          // 메타 정보 세로 스크롤 오프셋
	lastCLICmd    string       // 마지막 데이터 조회 CLI 명령어
}

// New는 새로운 Model을 생성합니다.
func New() Model {
	// API 클라이언트 초기화
	keys, err := config.GetAPIKeys()
	if err != nil || len(keys) == 0 {
		// API 키 미설정 상태로 반환
		return Model{
			width:       80,
			height:      24,
			activePanel: PanelSearch,
			statusMsg:   "⚠ API 키 미설정. kosis config set-key <KEY> 로 설정하세요",
			err:         fmt.Errorf("API 키 없음"),
			cursor:      0,
			typing:      false,
		}
	}

	client, err := api.NewClient(keys)
	if err != nil {
		// 클라이언트 생성 실패
		return Model{
			width:       80,
			height:      24,
			activePanel: PanelSearch,
			statusMsg:   "❌ API 클라이언트 초기화 실패: " + err.Error(),
			err:         err,
			cursor:      0,
			typing:      false,
		}
	}

	if client != nil {
		// 캐시 디렉토리 초기화
		cacheDir := config.ConfigDir() + "/cache"
		if err := client.InitCache(cacheDir, 24); err != nil {
			// 캐시 초기화 실패는 경고만 표시하고 계속 진행
			fmt.Fprintf(os.Stderr, "⚠ 캐시 초기화 실패: %v\n", err)
		}
	}

	// 즐겨찾기와 이력 로드
	bookmarks, _ := bookmark.List()
	histories, _ := history.List(5)

	ti := textinput.New()
	ti.Placeholder = "검색어를 입력하세요..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40

	return Model{
		width:       80,
		height:      24,
		activePanel: PanelSearch,
		statusMsg:   "검색어를 입력하세요 (Enter: 검색, Esc: 취소)",
		client:      client,
		cursor:      0,
		typing:      true,
		textInput:   ti,
		bookmarks:   bookmarks,
		histories:   histories,
	}
}

// Init은 Bubble Tea 초기화 함수입니다.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// doSearch는 검색을 비동기로 실행하는 커맨드입니다.
func (m Model) doSearch(keyword string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return searchResultMsg{err: fmt.Errorf("API 클라이언트가 초기화되지 않았습니다")}
		}
		results, err := m.client.Search(keyword, api.SearchOptions{ResultCount: 20})
		if err != nil {
			return searchResultMsg{err: err}
		}
		var items []SearchItem
		for _, r := range results {
			items = append(items, SearchItem{
				OrgID: r.OrgID, OrgNM: r.OrgNM,
				TblID: r.TblID, TblNM: r.TblNM,
			})
		}
		return searchResultMsg{results: items}
	}
}

// doMeta는 메타 정보 조회를 비동기로 실행하는 커맨드입니다.
func (m Model) doMeta(orgID, tblID string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return metaResultMsg{err: fmt.Errorf("API 클라이언트가 초기화되지 않았습니다")}
		}
		summary, err := m.client.MetaSummary(orgID, tblID)
		if err != nil {
			return metaResultMsg{err: err}
		}
		meta := &MetaInfo{
			PeriodInfo:      "수록정보 확인",
			ClassGroupNames: map[int]string{},
		}
		// 분류 그룹(ObjID) 수 세기 + 그룹명 추출
		objIDSet := map[string]bool{}
		objIDOrder := []string{} // 등장 순서 보존
		objIDName := map[string]string{} // ObjID → ObjNM
		for _, c := range summary.Classifications {
			if c.ObjID != "" {
				if !objIDSet[c.ObjID] {
					objIDSet[c.ObjID] = true
					objIDOrder = append(objIDOrder, c.ObjID)
					if c.ObjNM != "" {
						objIDName[c.ObjID] = c.ObjNM
					}
				}
			}
		}
		meta.NumClassGroups = len(objIDSet)
		if meta.NumClassGroups == 0 {
			meta.NumClassGroups = 1
		}
		// 분류 그룹 번호(1~8)에 그룹명 매핑
		for i, objID := range objIDOrder {
			if name, ok := objIDName[objID]; ok && i < 8 {
				meta.ClassGroupNames[i+1] = name
			}
		}

		for _, c := range summary.Classifications {
			code := c.ItmID
			name := c.ItmNM
			if code == "" {
				code = c.Code
			}
			if name == "" {
				name = c.Label
			}
			meta.Classifications = append(meta.Classifications, ClassInfo{Code: code, Name: name})
		}
		for _, i := range summary.Items {
			code := i.ItmID
			name := i.ItmNM
			if code == "" {
				code = i.Code
			}
			if name == "" {
				name = i.Label
			}
			meta.Items = append(meta.Items, ItemInfo{Code: code, Name: name})
		}
		for _, p := range summary.Periods {
			prdCode := convertPrdSe(p.PrdSe)
			meta.PeriodInfo = "prdSe=" + prdCode + " " + p.StrtPrdDe + "~" + p.EndPrdDe
		}
		// ColumnMeta 생성: 메타 정보에서 실제 컬럼명 추출
		meta.ColumnMeta = summary.BuildColumnMeta()
		return metaResultMsg{meta: meta}
	}
}

// doData는 데이터 조회를 비동기로 실행하는 커맨드입니다.
// 4만 셀 초과 시 자동 분할 조회합니다.
func (m Model) doData(orgID, tblID string, opts api.DataOptions) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return dataResultMsg{err: fmt.Errorf("API 클라이언트가 초기화되지 않았습니다")}
		}
		splitOpts := api.SplitOptions{MaxCells: 40000}
		results, err := m.client.DataWithAutoSplit(orgID, tblID, opts, splitOpts, nil)
		if err != nil {
			return dataResultMsg{err: err}
		}
		return dataResultMsg{data: results}
	}
}

// Update는 메시지를 처리합니다.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// 입력 모드일 때 (검색어 타이핑)
		if m.typing {
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case KeyEnter:
				m.typing = false
				m.searchInput = m.textInput.Value()
				m.textInput.Blur()
				if m.searchInput == "" {
					m.statusMsg = "검색어를 입력하세요"
					return m, nil
				}
				m.loading = true
				m.statusMsg = "🔍 검색 중..."
				return m, m.doSearch(m.searchInput)
			case KeyEscape:
				m.typing = false
				m.textInput.Blur()
				m.statusMsg = m.getPanelStatusMsg()
				return m, nil
			default:
				// textinput 컴포넌트에 키 이벤트 위임
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				m.searchInput = m.textInput.Value()
				return m, cmd
			}
		}

		// 일반 모드에서의 키 처리
		switch msg.String() {
		case KeyQuit, "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case KeyTab:
			// 패널 순환: Search → Meta → Params → Result → Search
			m.activePanel = (m.activePanel + 1) % 4
			m.statusMsg = m.getPanelStatusMsg()
		case KeySearch:
			m.activePanel = PanelSearch
			m.typing = true
			m.textInput.SetValue("")
			m.textInput.Focus()
			m.searchInput = ""
			m.statusMsg = "검색어 입력 (Enter: 검색, Esc: 취소)"
		case KeyHelp:
			m.statusMsg = KeyBindings()
		case KeyUp, "k":
			if m.activePanel == PanelMeta {
				if m.metaScrollY > 0 {
					m.metaScrollY--
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
					if len(m.searchResults) > 0 {
						m.activePanel = PanelSearch
					}
				}
			}
		case KeyDown, "j":
			if m.activePanel == PanelMeta {
				m.metaScrollY++
			} else {
				if m.cursor < len(m.searchResults)-1 {
					m.cursor++
					if len(m.searchResults) > 0 {
						m.activePanel = PanelSearch
					}
				}
			}
		case "left", "h":
			if m.activePanel == PanelResult {
				if m.resultScrollX > 0 {
					m.resultScrollX -= 5
					if m.resultScrollX < 0 {
						m.resultScrollX = 0
					}
				}
			}
		case "right", "l":
			if m.activePanel == PanelResult {
				m.resultScrollX += 5
			}
		case KeyExport: // e 키: 내보내기
			if len(m.resultData) > 0 {
				m.statusMsg = "💾 내보내기: 기능 준비 중 (CLI로 사용: kosis d ... -o file.xlsx)"
			} else {
				m.statusMsg = "⚠ 내보낼 데이터가 없습니다. 먼저 데이터를 조회하세요."
			}
		case KeyBookmark: // f 키: 즐겨찾기 추가
			if m.selectedTable != nil {
				err := bookmark.Add(m.selectedTable.OrgID, m.selectedTable.TblID, m.selectedTable.TblNM)
				if err != nil {
					m.statusMsg = "❌ 즐겨찾기 추가 실패: " + err.Error()
				} else {
					m.statusMsg = "★ 즐겨찾기에 추가: " + m.selectedTable.TblNM
					// 즐겨찾기 새로고침
					if newBookmarks, err := bookmark.List(); err == nil {
						m.bookmarks = newBookmarks
					}
				}
			} else {
				m.statusMsg = "⚠ 즐겨찾기에 추가할 통계표를 먼저 선택하세요."
			}
		case KeyIndicator: // i 키: 주요지표 모드 전환
			m.statusMsg = "📊 주요지표 모드: CLI로 사용 → kosis ind s \"GDP\""
		case KeyEnter:
			// 검색 결과에서 통계표 선택 → 메타 조회
			if (m.activePanel == PanelSearch || m.activePanel == PanelMeta) && len(m.searchResults) > 0 && m.cursor < len(m.searchResults) {
				selected := m.searchResults[m.cursor]
				m.selectedTable = &TableInfo{OrgID: selected.OrgID, TblID: selected.TblID, TblNM: selected.TblNM}
				m.loading = true
				m.statusMsg = "📋 메타 로딩 중..."
				return m, m.doMeta(selected.OrgID, selected.TblID)
			} else if m.activePanel == PanelParams {
				// 파라미터 설정에서 Enter 키: 데이터 조회 실행
				if m.selectedTable != nil && m.metaInfo != nil {
					// 기본값 설정 (파라미터가 비어있으면 메타의 첫 번째값 사용)
					class1 := m.paramClass1
					item := m.paramItem
					if class1 == "" && len(m.metaInfo.Classifications) > 0 {
						// "ALL"로 전체 선택 (가장 안전)
						class1 = "ALL"
					}
					if item == "" && len(m.metaInfo.Items) > 0 {
						// "ALL"로 전체 선택
						item = "ALL"
					}

					// 주기: 비어있으면 메타 수록정보에서 추출
					period := m.paramPeriod
					if period == "" && m.metaInfo != nil {
						// PeriodInfo에서 prdSe 추출 (예: "prdSe=Y ...")
						if len(m.metaInfo.PeriodInfo) > 6 {
							parts := strings.Fields(m.metaInfo.PeriodInfo)
							for _, p := range parts {
								if strings.HasPrefix(p, "prdSe=") {
									period = strings.TrimPrefix(p, "prdSe=")
									break
								}
							}
						}
						if period == "" {
							period = "Y" // 최종 기본값
						}
					}

					opts := api.DataOptions{
						Class1: class1,
						Item:   item,
						PrdSe:  period,
					}
					// latest: 비어있으면 최근 5개 기본
					if m.paramLatest != "" {
						if n, err := strconv.Atoi(m.paramLatest); err == nil && n > 0 {
							opts.NewEstPrdCnt = m.paramLatest
						}
					} else {
						opts.NewEstPrdCnt = "5"
					}
					m.loading = true
					m.statusMsg = "📊 데이터 조회 중..."
					return m, m.doData(m.selectedTable.OrgID, m.selectedTable.TblID, opts)
				} else {
					m.statusMsg = "⚠ 먼저 통계표를 선택하고 메타 정보를 로드하세요."
				}
			}
		}

	// 비동기 결과 처리
	case searchResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.statusMsg = fmt.Sprintf("❌ 검색 오류: %v", msg.err)
		} else {
			m.searchResults = msg.results
			m.cursor = 0
			m.statusMsg = fmt.Sprintf("✓ %d건 검색 완료 (↑↓ 선택, Enter 확인)", len(msg.results))
		}

	case metaResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.statusMsg = fmt.Sprintf("❌ 메타 조회 오류: %v", msg.err)
		} else {
			m.metaInfo = msg.meta
			m.metaScrollY = 0

			// 메타 로드 완료 → 자동으로 기본 파라미터 설정 후 데이터 조회 시작
			if m.selectedTable != nil {
				// 기본 파라미터: ALL, ALL, 자동 주기, 최근 5개
				period := "Y"
				if m.metaInfo != nil && m.metaInfo.PeriodInfo != "" {
					parts := strings.Fields(m.metaInfo.PeriodInfo)
					for _, p := range parts {
						if strings.HasPrefix(p, "prdSe=") {
							period = strings.TrimPrefix(p, "prdSe=")
							break
						}
					}
				}
				m.paramPeriod = period
				m.paramLatest = "5"

				// 항목은 ALL로 전체 조회 (종사자수, 급여액, 출하액 등 모두 포함)
				itemValue := "ALL"

				opts := api.DataOptions{
					Item:         itemValue,
					PrdSe:        period,
					NewEstPrdCnt: "5",
				}

				n := m.metaInfo.NumClassGroups
				// 분류가 20개 이상이면 첫 번째 값만 조회 (4만 셀 제한 방지)
				classValue := "ALL"
				if len(m.metaInfo.Classifications) > 20 {
					classValue = m.metaInfo.Classifications[0].Code
				}

				if n >= 1 { opts.Class1 = classValue }
				if n >= 2 { opts.Class2 = "ALL" }
				if n >= 3 { opts.Class3 = "ALL" }
				if n >= 4 { opts.Class4 = "ALL" }
				if n >= 5 { opts.Class5 = "ALL" }
				if n >= 6 { opts.Class6 = "ALL" }
				if n >= 7 { opts.Class7 = "ALL" }
				if n >= 8 { opts.Class8 = "ALL" }

				m.paramClass1 = opts.Class1
				m.paramItem = itemValue

				// CLI 명령어 생성
				cmd := fmt.Sprintf("kosis d %s %s -i %s -p %s -l 5", m.selectedTable.OrgID, m.selectedTable.TblID, itemValue, period)
				if n >= 1 { cmd += fmt.Sprintf(" -c1 %s", opts.Class1) }
				if n >= 2 { cmd += " --class2 ALL" }
				if n >= 3 { cmd += " --class3 ALL" }
				if n >= 4 { cmd += " --class4 ALL" }
				if n >= 5 { cmd += " --class5 ALL" }
				if n >= 6 { cmd += " --class6 ALL" }
				if n >= 7 { cmd += " --class7 ALL" }
				if n >= 8 { cmd += " --class8 ALL" }
				m.lastCLICmd = cmd

				m.loading = true
				m.activePanel = PanelResult
				m.statusMsg = fmt.Sprintf("📊 데이터 조회 중... (c1=%s, 최근 5개)", opts.Class1)
				return m, m.doData(m.selectedTable.OrgID, m.selectedTable.TblID, opts)
			} else {
				m.statusMsg = "✓ 메타 로드 완료"
			}
		}

	case dataResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.statusMsg = fmt.Sprintf("❌ 데이터 조회 오류: %v", msg.err)
		} else {
			m.resultData = msg.data
			m.resultScrollX = 0
			colInfo := ""
			if m.metaInfo != nil && m.metaInfo.ColumnMeta != nil {
				for _, c := range m.metaInfo.ColumnMeta.Columns {
					colInfo += c.Key + ":" + c.Label + " "
				}
			}
			m.statusMsg = fmt.Sprintf("✓ %d건 조회 완료 [%s]", len(msg.data), colInfo)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width/3 - 12
	}
	return m, nil
}

// View는 현재 모델 상태를 렌더링합니다.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	// NO_COLOR 환경변수 확인
	if isColorDisabled() {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	// 레이아웃 비율: 왼쪽 42%, 오른쪽 58% (보더 여유 제외)
	leftWidth := m.width * 42 / 100
	if m.width < 80 {
		leftWidth = m.width * 35 / 100  // 작은 터미널에서는 35%로 축소
	}
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := m.width - leftWidth - 3
	if rightWidth < 20 {
		rightWidth = 20
	}

	// 타이틀 바 렌더링
	title := m.renderTitle()

	// 왼쪽 패널 렌더링 (검색 + 결과)
	leftPanel := m.renderLeftPanel(leftWidth)

	// 오른쪽 패널 렌더링 (메타 + 파라미터 + 결과테이블)
	rightPanel := m.renderRightPanel(rightWidth)

	// 왼쪽, 오른쪽 패널 합치기
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// 상태바 렌더링
	statusBar := m.renderStatusBar()

	// 전체 레이아웃 조립
	result := lipgloss.JoinVertical(lipgloss.Left, title, mainContent, statusBar)

	// 모든 라인이 m.width를 넘지 않도록 정규화
	result = normalizeLineWidth(result, m.width)

	return result
}

// renderTitle은 타이틀 바를 렌더링합니다.
func (m Model) renderTitle() string {
	title := "KOSIS 통계 데이터 탐색기"
	helpText := "[?] 도움말  [q] 종료"

	// 글로벌 스타일 재사용하되 너비만 동적으로 설정
	titleStyle := TitleBarStyle.Copy().Width(m.width)

	titleContent := title + " " + helpText
	return titleStyle.Render(titleContent)
}

// renderLeftPanel은 왼쪽 패널(검색 + 결과 + 즐겨찾기 + 이력)을 렌더링합니다.
func (m Model) renderLeftPanel(width int) string {
	panelStyle := PanelCommonStyle.Copy().
		BorderForeground(ColorPrimary).
		Width(width).
		Height(m.height-3)

	searchSection := m.renderSearchSection(width - 4)
	resultsSection := m.renderResultsSection(width - 4)
	bookmarkSection := m.renderBookmarkSection(width - 4)
	historySection := m.renderHistorySection(width - 4)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		searchSection,
		"",
		resultsSection,
		"",
		bookmarkSection,
		"",
		historySection,
	)

	return panelStyle.Render(content)
}

// renderSearchSection은 검색 입력 섹션을 렌더링합니다.
func (m Model) renderSearchSection(width int) string {
	var borderColor lipgloss.Color = ColorBorder
	if m.activePanel == PanelSearch {
		borderColor = ColorAccent
	}

	sectionStyle := SectionCommonStyle.Copy().
		BorderForeground(borderColor).
		Width(width)

	var content string
	if m.typing {
		content = "🔍 검색: " + m.textInput.View() + "\n(Enter: 검색, Esc: 취소)"
	} else {
		searchDisplay := truncateText(m.searchInput, width-12)
		content = "🔍 검색: " + searchDisplay + "\n(/ 키로 검색 시작)"
	}

	return sectionStyle.Render(content)
}

// renderBookmarkSection은 즐겨찾기 섹션을 렌더링합니다.
func (m Model) renderBookmarkSection(width int) string {
	sectionStyle := SectionCommonStyle.Copy().
		BorderForeground(ColorBorder).
		Width(width).
		Height(5)

	bookmarkLines := []string{"★ 즐겨찾기"}
	bookmarkLines = append(bookmarkLines, "─────────────")

	if len(m.bookmarks) == 0 {
		bookmarkLines = append(bookmarkLines, "  (f 키로 추가)")
	} else {
		for i, bm := range m.bookmarks {
			if i >= 3 { // 최대 3개만 표시
				bookmarkLines = append(bookmarkLines, "  ...")
				break
			}
			bookmarkLines = append(bookmarkLines, "  "+truncateText(bm.Name, width-6)+" ("+bm.TblID+")")
		}
	}

	return sectionStyle.Render(lipgloss.JoinVertical(lipgloss.Left, bookmarkLines...))
}

// renderHistorySection은 이력 섹션을 렌더링합니다.
func (m Model) renderHistorySection(width int) string {
	sectionStyle := SectionCommonStyle.Copy().
		BorderForeground(ColorBorder).
		Width(width).
		Height(5)

	historyLines := []string{"◷ 최근 조회"}
	historyLines = append(historyLines, "─────────────")

	if len(m.histories) == 0 {
		historyLines = append(historyLines, "  (조회 이력 없음)")
	} else {
		for i, h := range m.histories {
			if i >= 3 { // 최대 3개만 표시
				historyLines = append(historyLines, "  ...")
				break
			}
			historyLines = append(historyLines, "  "+truncateText(h.Command, width-6))
		}
	}

	return sectionStyle.Render(lipgloss.JoinVertical(lipgloss.Left, historyLines...))
}

// renderResultsSection은 검색 결과 섹션을 렌더링합니다.
func (m Model) renderResultsSection(width int) string {
	var borderColor lipgloss.Color = ColorBorder
	if m.activePanel == PanelMeta {
		borderColor = ColorAccent
	}

	// 검색/즐겨찾기/이력 섹션이 각각 ~7줄, 패널 보더+패딩 ~4줄 차지
	// 남은 공간을 결과 섹션에 최대한 할당
	resultHeight := m.height - 3 - 7 - 5 - 5 - 6 // 패널보더, 검색, 즐겨찾기, 이력, 여백
	if resultHeight < 10 {
		resultHeight = 10
	}

	sectionStyle := SectionCommonStyle.Copy().
		BorderForeground(borderColor).
		Width(width).
		Height(resultHeight)

	if len(m.searchResults) == 0 {
		return sectionStyle.Render("검색 결과 / 목록\n─────────────\n(검색어를 입력하세요)")
	}

	// 각 항목이 3줄이므로 보이는 항목 수 계산 (헤더 2줄 제외)
	maxVisible := (resultHeight - 2) / 3
	if maxVisible < 3 {
		maxVisible = 3
	}
	total := len(m.searchResults)

	// 스크롤 윈도우: cursor가 보이도록 start 위치 조정
	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > total {
		end = total
	}

	header := fmt.Sprintf("검색 결과 (%d건, ↑↓ 선택)", total)
	resultLines := []string{header, "─────────────"}

	if start > 0 {
		resultLines = append(resultLines, fmt.Sprintf("  ↑ %d개 더 있음", start))
	}

	for i := start; i < end; i++ {
		item := m.searchResults[i]
		marker := "  "
		if i == m.cursor {
			marker = "▸ "
		}
		// 1줄: 통계표명 (marker 2 + 여유 2 = 4)
		name := truncateText(item.TblNM, width-6)
		line1 := marker + name
		// 2줄: 기관명 + 통계표 ID (들여쓰기 4 + 여유 2 = 6)
		orgInfo := item.OrgNM + " | " + item.TblID
		line2 := "    " + truncateText(orgInfo, width-8)
		// 3줄: 구분선
		line3 := "    ───────────────────────"

		// 선택된 항목은 노란색으로 표시
		if i == m.cursor {
			selectedStyle := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
			line1 = selectedStyle.Render(line1)
			line2 = selectedStyle.Render(line2)
		}
		resultLines = append(resultLines, line1, line2, line3)
	}

	if end < total {
		resultLines = append(resultLines, fmt.Sprintf("  ↓ %d개 더 있음", total-end))
	}

	return sectionStyle.Render(lipgloss.JoinVertical(lipgloss.Left, resultLines...))
}

// renderRightPanel은 오른쪽 패널(메타 + 파라미터 + 결과)을 렌더링합니다.
func (m Model) renderRightPanel(width int) string {
	panelStyle := PanelCommonStyle.Copy().
		BorderForeground(ColorPrimary).
		Width(width).
		Height(m.height-3)

	metaSection := m.renderMetaSection(width - 4)
	paramsSection := m.renderParamsSection(width - 4)

	// 메타+파라미터 높이를 계산하여 결과 섹션에 나머지 공간 할당
	metaLines := strings.Count(metaSection, "\n") + 1
	paramsLines := strings.Count(paramsSection, "\n") + 1
	resultHeight := m.height - 3 - metaLines - paramsLines - 4 // 패널보더, 여백
	if resultHeight < 8 {
		resultHeight = 8
	}

	resultSection := m.renderResultSection(width-4, resultHeight)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		metaSection,
		"",
		paramsSection,
		"",
		resultSection,
	)

	return panelStyle.Render(content)
}

// renderMetaSection은 메타 정보 섹션을 렌더링합니다.
func (m Model) renderMetaSection(width int) string {
	var borderColor lipgloss.Color = ColorBorder
	if m.activePanel == PanelMeta {
		borderColor = ColorAccent
	}

	sectionStyle := SectionCommonStyle.Copy().
		BorderForeground(borderColor).
		Width(width)

	if m.selectedTable == nil {
		return sectionStyle.Render("📊 통계표 정보\n─────────────\n(통계표를 선택하세요)")
	}

	content := "📊 통계표 정보\n" +
		"─────────────\n" +
		"통계표명: " + m.selectedTable.TblNM + "\n" +
		"기관코드: " + m.selectedTable.OrgID + "\n" +
		"통계표ID: " + m.selectedTable.TblID + "\n"

	if m.metaInfo != nil {
		content += "\n[분류] 코드 목록\n"
		for _, cls := range m.metaInfo.Classifications {
			content += "  " + cls.Code + " " + cls.Name + "\n"
		}

		content += "\n[항목] 코드 목록\n"
		for _, item := range m.metaInfo.Items {
			content += "  " + item.Code + " " + item.Name + "\n"
		}

		content += "\n[수록정보]\n" +
			m.metaInfo.PeriodInfo
	}

	// 세로 스크롤 적용: 전체 라인을 생성 후 뷰포트만 표시
	allLines := strings.Split(content, "\n")
	metaHeight := 10 // 메타 섹션 뷰포트 높이
	totalLines := len(allLines)

	// 스크롤 범위 제한
	maxScrollY := totalLines - metaHeight
	if maxScrollY < 0 {
		maxScrollY = 0
	}
	scrollY := m.metaScrollY
	if scrollY > maxScrollY {
		scrollY = maxScrollY
	}

	endIdx := scrollY + metaHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}
	visibleLines := allLines[scrollY:endIdx]

	// 스크롤 인디케이터
	if scrollY > 0 {
		visibleLines = append([]string{fmt.Sprintf("  ↑ %d줄 더 있음", scrollY)}, visibleLines...)
	}
	if endIdx < totalLines {
		visibleLines = append(visibleLines, fmt.Sprintf("  ↓ %d줄 더 있음 (↑↓ 스크롤)", totalLines-endIdx))
	}

	return sectionStyle.Render(strings.Join(visibleLines, "\n"))
}

// renderParamsSection은 파라미터 설정 섹션을 렌더링합니다.
func (m Model) renderParamsSection(width int) string {
	var borderColor lipgloss.Color = ColorBorder
	if m.activePanel == PanelParams {
		borderColor = ColorAccent
	}

	sectionStyle := SectionCommonStyle.Copy().
		BorderForeground(borderColor).
		Width(width)

	// 파라미터 값 또는 기본값 표시
	class1Display := m.paramClass1
	itemDisplay := m.paramItem
	periodDisplay := m.paramPeriod
	latestDisplay := m.paramLatest

	// 메타 정보가 있고 파라미터가 비어있으면 첫 번째값 표시
	if m.metaInfo != nil && (m.metaInfo.Classifications != nil || m.metaInfo.Items != nil) {
		if class1Display == "" && len(m.metaInfo.Classifications) > 0 && m.metaInfo.Classifications[0].Code != "" {
			class1Display = m.metaInfo.Classifications[0].Code + " " + m.metaInfo.Classifications[0].Name
		}
		if itemDisplay == "" && len(m.metaInfo.Items) > 0 && m.metaInfo.Items[0].Code != "" {
			itemDisplay = m.metaInfo.Items[0].Code + " " + m.metaInfo.Items[0].Name
		}
	}

	if class1Display == "" {
		class1Display = "[선택 필요]"
	}
	if itemDisplay == "" {
		itemDisplay = "[선택 필요]"
	}
	if periodDisplay == "" {
		periodDisplay = "[자동]"
	}
	if latestDisplay == "" {
		latestDisplay = "[전체]"
	}

	textWidth := width - 10
	if textWidth < 5 {
		textWidth = 5
	}
	content := "⚙ 파라미터 설정\n" +
		"─────────────\n" +
		"분류: " + truncateText(class1Display, textWidth) + "\n" +
		"항목: " + truncateText(itemDisplay, textWidth) + "\n" +
		"주기: " + periodDisplay + "\n" +
		"최근: " + latestDisplay + "\n" +
		"\n(Enter로 조회)"

	return sectionStyle.Render(content)
}

// renderResultSection은 결과 테이블 섹션을 렌더링합니다.
func (m Model) renderResultSection(width int, viewHeight int) string {
	var borderColor lipgloss.Color = ColorBorder
	if m.activePanel == PanelResult {
		borderColor = ColorAccent
	}

	sectionStyle := SectionCommonStyle.Copy().
		BorderForeground(borderColor).
		Width(width).
		Height(viewHeight)

	if len(m.resultData) == 0 {
		return sectionStyle.Render("📋 결과 데이터\n───────────────────\n(조회를 실행하세요)")
	}

	// 동적 컬럼 결정: 데이터에 값이 있는 필드만 표시
	type colDef struct {
		key   string
		label string
	}
	// ColumnMeta에서 컬럼 정보를 가져와서 데이터에 값이 있는 것만 필터링
	var activeCols []colDef
	if m.metaInfo != nil && m.metaInfo.ColumnMeta != nil {
		filtered := m.metaInfo.ColumnMeta.FilterByData(m.resultData)
		for _, col := range filtered.Columns {
			activeCols = append(activeCols, colDef{key: col.Key, label: col.Label})
		}
	} else {
		// 메타 없을 때 폴백
		fallbackCols := []colDef{
			{"C1_NM", "분류1"}, {"C2_NM", "분류2"}, {"ITM_NM", "항목"},
			{"PRD_DE", "시점"}, {"DT", "수치값"}, {"UNIT_NM", "단위"},
		}
		for _, col := range fallbackCols {
			for _, row := range m.resultData {
				if row.GetField(col.key) != "" {
					activeCols = append(activeCols, col)
					break
				}
			}
		}
	}

	if len(activeCols) == 0 {
		return sectionStyle.Render("📋 결과 데이터\n───────────────────\n(데이터 없음)")
	}

	// 컬럼별 최적 폭 계산 (헤더와 데이터 중 가장 긴 값 기준)
	colWidths := make([]int, len(activeCols))
	for ci, col := range activeCols {
		maxW := stringDisplayWidth(col.label)
		for _, row := range m.resultData {
			w := stringDisplayWidth(row.GetField(col.key))
			if w > maxW {
				maxW = w
			}
		}
		if maxW < 4 {
			maxW = 4
		}
		// 가로 스크롤이 있으므로 컬럼 폭을 데이터에 맞춤 (최대 50)
		if maxW > 50 {
			maxW = 50
		}
		colWidths[ci] = maxW
	}

	// 전체 테이블 폭 계산
	totalTableWidth := 0
	for _, cw := range colWidths {
		totalTableWidth += cw
	}
	totalTableWidth += (len(activeCols) - 1) * 3 // 구분자 " │ "

	// 스크롤 범위 제한
	viewWidth := width - 4
	maxScrollX := totalTableWidth - viewWidth
	if maxScrollX < 0 {
		maxScrollX = 0
	}
	scrollX := m.resultScrollX
	if scrollX > maxScrollX {
		scrollX = maxScrollX
	}

	// 전체 행을 생성한 뒤 가로 스크롤 적용
	buildRow := func(cells []string) string {
		var parts []string
		for ci, cell := range cells {
			parts = append(parts, truncateText(cell, colWidths[ci]))
		}
		return strings.Join(parts, " │ ")
	}

	// 가로 스크롤 뷰포트 적용 함수
	applyHScroll := func(line string) string {
		runes := []rune(line)
		// scrollX만큼 건너뛰기 (표시 폭 기준)
		consumed := 0
		startIdx := 0
		for startIdx < len(runes) && consumed < scrollX {
			consumed += runeWidth(runes[startIdx])
			startIdx++
		}
		// viewWidth만큼만 표시
		displayed := 0
		endIdx := startIdx
		for endIdx < len(runes) && displayed < viewWidth {
			displayed += runeWidth(runes[endIdx])
			endIdx++
		}
		return string(runes[startIdx:endIdx])
	}

	// 헤더 생성
	var headerCells []string
	for _, col := range activeCols {
		headerCells = append(headerCells, col.label)
	}
	headerLine := buildRow(headerCells)

	var sepCells []string
	for _, cw := range colWidths {
		sep := ""
		for j := 0; j < cw; j++ {
			sep += "─"
		}
		sepCells = append(sepCells, sep)
	}
	sepLine := strings.Join(sepCells, "─┼─")

	// 스크롤 인디케이터
	scrollInfo := ""
	if maxScrollX > 0 {
		scrollInfo = fmt.Sprintf(" (←→ 가로스크롤)", )
	}

	// 뷰포트 높이만큼만 행 표시 (헤더 4줄 제외)
	maxRows := viewHeight - 4
	if maxRows < 3 {
		maxRows = 3
	}

	totalRows := len(m.resultData)
	showRows := totalRows
	if showRows > maxRows {
		showRows = maxRows
	}

	lines := []string{fmt.Sprintf("📋 결과 데이터 (%d건)%s", totalRows, scrollInfo), "───────────────────"}
	lines = append(lines, applyHScroll(headerLine))
	lines = append(lines, applyHScroll(sepLine))

	for i := 0; i < showRows; i++ {
		row := m.resultData[i]
		var cells []string
		for _, col := range activeCols {
			cells = append(cells, row.GetField(col.key))
		}
		lines = append(lines, applyHScroll(buildRow(cells)))
	}
	if totalRows > showRows {
		lines = append(lines, fmt.Sprintf("  ... 외 %d건", totalRows-showRows))
	}

	// CLI 명령어 표시
	if m.lastCLICmd != "" {
		lines = append(lines, "───────────────────")
		lines = append(lines, "💻 CLI: "+m.lastCLICmd)
	}

	return sectionStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// runeWidth는 한글 등 전각 문자의 표시 폭을 계산합니다.
func runeWidth(r rune) int {
	if r >= 0x1100 && (r <= 0x115F || r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6)) {
		return 2
	}
	return 1
}

// stringDisplayWidth는 문자열의 터미널 표시 폭을 계산합니다.
func stringDisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// truncateText는 텍스트를 지정된 표시 폭으로 자르고 패딩합니다.
func truncateText(text string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(text)
	displayW := 0
	cutIdx := len(runes)

	for i, r := range runes {
		rw := runeWidth(r)
		if displayW+rw > width-3 && width > 3 && stringDisplayWidth(text) > width {
			cutIdx = i
			break
		}
		displayW += rw
	}

	result := text
	if cutIdx < len(runes) {
		result = string(runes[:cutIdx]) + "..."
	}

	// 패딩: 고정 폭에 맞추기
	dw := stringDisplayWidth(result)
	for dw < width {
		result += " "
		dw++
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// renderStatusBar는 상태바를 렌더링합니다.
func (m Model) renderStatusBar() string {
	// StatusBarStyle의 Padding(0, 1)을 고려하여 width 조정 (양쪽 1칸씩 = 총 2칸)
	effectiveWidth := m.width - 2
	if effectiveWidth < 20 {
		effectiveWidth = 20
	}

	statusStyle := StatusBarStyle.
		Width(m.width)

	var status string
	if m.loading {
		status = "⏳ 로딩중..."
	} else if m.err != nil {
		errMsg := m.err.Error()
		maxLen := effectiveWidth - 4 // "❌ " 부분 고려
		if maxLen < 20 {
			maxLen = 20
		}
		errMsg = truncateText(errMsg, maxLen)
		status = "❌ " + errMsg
	} else {
		status = m.statusMsg
	}

	// 상태 메시지를 터미널 너비에 맞게 제한 (padding 포함)
	maxStatusLen := effectiveWidth
	if maxStatusLen < 20 {
		maxStatusLen = 20
	}
	status = truncateText(status, maxStatusLen)

	return statusStyle.Render(status)
}

// normalizeLineWidth는 모든 라인을 지정된 너비 이하로 정규화합니다.
func normalizeLineWidth(text string, maxWidth int) string {
	lines := strings.Split(text, "\n")
	ansiRe := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	for i, line := range lines {
		// ANSI 코드의 위치를 추적하며 문자 길이 계산
		cleanLine := ansiRe.ReplaceAllString(line, "")
		cleanRunes := []rune(cleanLine)

		if len(cleanRunes) > maxWidth {
			// 명시적으로 maxWidth개 문자까지만 유지하기 위해 truncate 수행
			// ANSI 코드는 제거됨
			lines[i] = string(cleanRunes[:maxWidth])
		}
	}
	return strings.Join(lines, "\n")
}

// getPanelStatusMsg는 현재 활성 패널에 따른 상태 메시지를 반환합니다.
func (m Model) getPanelStatusMsg() string {
	switch m.activePanel {
	case PanelSearch:
		return "/ 검색어 입력  Enter 검색  Tab 다음 패널"
	case PanelMeta:
		return "↑↓ 목록 탐색  Enter 선택  Tab 다음 패널"
	case PanelParams:
		return "파라미터 설정  Enter 조회 실행  Tab 다음 패널"
	case PanelResult:
		return "e 내보내기  f 즐겨찾기  i 지표모드  Tab 다음 패널"
	default:
		return "/"
	}
}

// convertPrdSe 한글 주기명을 API 코드로 변환
func convertPrdSe(prdSe string) string {
	switch prdSe {
	case "년", "연", "연간":
		return "Y"
	case "월", "월간":
		return "M"
	case "분기", "분기별":
		return "Q"
	case "반기", "반기별":
		return "H"
	case "Y", "M", "Q", "H":
		return prdSe
	default:
		if prdSe != "" {
			return prdSe
		}
		return "Y"
	}
}

// StartTUI는 TUI를 시작합니다.
func StartTUI() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
