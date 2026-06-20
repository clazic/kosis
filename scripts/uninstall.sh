#!/bin/sh
# KOSIS 제거 스크립트 (macOS / Linux)
# 사용법:
#   curl -fsSL https://raw.githubusercontent.com/clazic/kosis/master/scripts/uninstall.sh | sh
#   sh uninstall.sh [--purge] [--yes]
#
# 옵션:
#   --purge   설정·데이터(~/.kosis)까지 삭제
#   --yes     확인 프롬프트 없이 바로 실행
#
# 환경변수:
#   KOSIS_PURGE=1   --purge와 동일
#   KOSIS_YES=1     --yes와 동일

# tty 감지 (curl|sh, CI 등 비대화형 대응)
if { : < /dev/tty; } 2>/dev/null; then HAVE_TTY=1; else HAVE_TTY=0; fi

# 인자 파싱
PURGE=0
YES=0
for arg in "$@"; do
  case "$arg" in
    --purge) PURGE=1 ;;
    --yes)   YES=1   ;;
  esac
done
[ "${KOSIS_PURGE:-0}" = "1" ] && PURGE=1
[ "${KOSIS_YES:-0}"   = "1" ] && YES=1

# ── 제거 대상 수집 (존재하는 것만) ──
TARGETS=""
add_target() {
  [ -e "$1" ] && TARGETS="${TARGETS}  $1\n"
}

add_target "$HOME/.local/bin/kosis"
add_target "$HOME/.claude/skills/kosis"
add_target "$HOME/.codex/skills/kosis"
add_target "$(pwd)/.claude/skills/kosis"
add_target "$(pwd)/.codex/skills/kosis"

if [ -z "$TARGETS" ]; then
  echo "제거할 항목이 없습니다."
  exit 0
fi

# ── 미리보기 출력 ──
echo "다음 항목을 제거합니다:"
printf "%b" "$TARGETS"

# ── 대화형: 확인 질문 ──
if [ "$HAVE_TTY" = 1 ] && [ "$YES" = 0 ]; then
  printf "\n계속하시겠습니까? (y/N) " > /dev/tty
  read CONFIRM < /dev/tty
  case "$CONFIRM" in
    y|Y) ;;
    *) echo "취소됨."; exit 0 ;;
  esac
fi

# ── 대화형 + purge 아닐 때: config 삭제 여부 추가 질문 ──
if [ "$HAVE_TTY" = 1 ] && [ "$PURGE" = 0 ]; then
  printf "\n설정·데이터(~/.kosis)도 삭제할까요? (y/N) " > /dev/tty
  read PURGE_ANSWER < /dev/tty
  case "$PURGE_ANSWER" in
    y|Y) PURGE=1 ;;
  esac
fi

# ── 삭제 실행 ──
remove() {
  if [ -e "$1" ]; then
    rm -rf "$1"
    echo "  ✓ 제거: $1"
  fi
}

remove "$HOME/.local/bin/kosis"
remove "$HOME/.claude/skills/kosis"
remove "$HOME/.codex/skills/kosis"
remove "$(pwd)/.claude/skills/kosis"
remove "$(pwd)/.codex/skills/kosis"

# ── config 처리 ──
if [ "$PURGE" = 1 ]; then
  if [ -d "$HOME/.kosis" ]; then
    echo ""
    echo "⚠  설정·데이터 삭제: $HOME/.kosis (API 키/북마크/이력/캐시 포함)"
    rm -rf "$HOME/.kosis"
    echo "  ✓ 제거: $HOME/.kosis"
  fi
  CONFIG_NOTE="(설정·데이터 삭제됨)"
else
  CONFIG_NOTE="(설정·데이터 보존: ~/.kosis)"
fi

# ── Unix는 rc 파일 PATH 라인 건드리지 않음 (공유 디렉토리) — 안내만 ──
echo ""
echo "PATH 안내: ~/.local/bin이 $HOME/.bashrc 또는 $HOME/.zshrc에 등록된 경우"
echo "  직접 해당 라인을 제거해 주세요 (다른 도구와 공유될 수 있어 자동 수정하지 않습니다)."

echo ""
echo "✓ kosis 제거 완료 $CONFIG_NOTE"
