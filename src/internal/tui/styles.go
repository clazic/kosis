package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// isColorDisabled는 NO_COLOR 환경변수가 설정되어 있는지 확인합니다.
func isColorDisabled() bool {
	_, exists := os.LookupEnv("NO_COLOR")
	return exists
}

// 색상 팔레트
var (
	ColorPrimary   = lipgloss.Color("#2563EB") // 파란색
	ColorSecondary = lipgloss.Color("#3B82F6") // 밝은 파란
	ColorAccent    = lipgloss.Color("#10B981") // 초록 강조
	ColorWarning   = lipgloss.Color("#F59E0B") // 노란 경고
	ColorError     = lipgloss.Color("#EF4444") // 빨간 에러
	ColorMuted     = lipgloss.Color("#6B7280") // 회색
	ColorBg        = lipgloss.Color("#1E293B") // 어두운 배경
	ColorBgPanel   = lipgloss.Color("#0F172A") // 패널 배경
	ColorBorder    = lipgloss.Color("#334155") // 테두리
	ColorHighlight = lipgloss.Color("#DBEAFE") // 강조 텍스트
)

// 타이틀바
var TitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(ColorPrimary).
	Padding(0, 2).
	Align(lipgloss.Center)

// 패널 스타일 (활성/비활성)
var PanelActiveStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorPrimary).
	Padding(0, 1)

var PanelInactiveStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorBorder).
	Padding(0, 1)

// 선택된 항목
var SelectedItemStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(ColorHighlight).
	Background(ColorPrimary)

// 일반 항목
var NormalItemStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#E2E8F0"))

// 검색 입력
var SearchInputStyle = lipgloss.NewStyle().
	Foreground(ColorAccent).
	Bold(true)

// 상태바
var StatusBarStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#1E293B")).
	Foreground(lipgloss.Color("#94A3B8")).
	Padding(0, 1)

// 키 힌트
var KeyHintStyle = lipgloss.NewStyle().
	Foreground(ColorAccent).
	Bold(true)

var KeyDescStyle = lipgloss.NewStyle().
	Foreground(ColorMuted)

// 섹션 헤더
var SectionHeaderStyle = lipgloss.NewStyle().
	Foreground(ColorWarning).
	Bold(true)

// 에러
var ErrorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(ColorError).
	Padding(0, 1)

// 로딩
var LoadingStyle = lipgloss.NewStyle().
	Foreground(ColorAccent).
	Italic(true)

// 데이터 테이블 헤더
var TableHeaderStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(ColorPrimary).
	BorderBottom(true).
	BorderStyle(lipgloss.NormalBorder())

// 데이터 셀
var TableCellStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#E2E8F0"))

// 즐겨찾기 아이콘
var BookmarkStyle = lipgloss.NewStyle().
	Foreground(ColorWarning)

// 이력 아이콘
var HistoryStyle = lipgloss.NewStyle().
	Foreground(ColorMuted)

// 타이틀 스타일
var TitleBarStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("255")).
	Background(ColorPrimary).
	Padding(0, 1)

// 패널 스타일 (공통)
var PanelCommonStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(0, 1)

// 섹션 스타일 (공통 - 보더 컬러는 동적으로 설정)
var SectionCommonStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(0, 1)
