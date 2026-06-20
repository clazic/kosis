package tui

// 키바인딩 상수
const (
	KeySearch    = "/"
	KeyQuit      = "q"
	KeyTab       = "tab"
	KeyEnter     = "enter"
	KeyExport    = "e"
	KeyBookmark  = "f"
	KeyIndicator = "i"
	KeyHelp      = "?"
	KeyUp        = "up"
	KeyDown      = "down"
	KeyLeft      = "left"
	KeyRight     = "right"
	KeyEscape    = "esc"
)

// KeyBindings 도움말 텍스트
func KeyBindings() string {
	return KeyHintStyle.Render("/") + KeyDescStyle.Render(" 검색  ") +
		KeyHintStyle.Render("Enter") + KeyDescStyle.Render(" 선택  ") +
		KeyHintStyle.Render("Tab") + KeyDescStyle.Render(" 패널전환  ") +
		KeyHintStyle.Render("e") + KeyDescStyle.Render(" 내보내기  ") +
		KeyHintStyle.Render("f") + KeyDescStyle.Render(" 즐겨찾기  ") +
		KeyHintStyle.Render("i") + KeyDescStyle.Render(" 지표모드  ") +
		KeyHintStyle.Render("?") + KeyDescStyle.Render(" 도움말  ") +
		KeyHintStyle.Render("q") + KeyDescStyle.Render(" 종료")
}
