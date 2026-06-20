#!/bin/sh
# KOSIS 설치 스크립트 (macOS / Linux)
# 사용법: curl -fsSL https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.sh | sh
#
# 비대화형(CI) 환경변수:
#   KOSIS_TARGET=claude|codex|both     (기본 claude)
#   KOSIS_CLAUDE_SCOPE=global|project  (기본 global)
#   KOSIS_CODEX_SCOPE=global|project   (기본 global)
#   KOSIS_VERSION=vX.Y.Z               (기본: 최신 릴리스)
set -e

REPO="clazic/kosis"

# ── 버전 확인 ──
if [ -n "$KOSIS_VERSION" ]; then
  VERSION="$KOSIS_VERSION"
else
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')"
fi
[ -n "$VERSION" ] || { echo "오류: 버전 정보를 가져올 수 없습니다." >&2; exit 1; }
echo "kosis $VERSION 설치 중..."

# ── OS / 아키텍처 감지 ──
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "지원하지 않는 아키텍처: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) echo "지원하지 않는 OS: $OS" >&2; exit 1 ;;
esac

BIN_ASSET="kosis-${OS}-${ARCH}"
SKILL_ASSET="kosis-skill-${VERSION}.tar.gz"
BIN_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BIN_ASSET}"
SKILL_URL="https://github.com/${REPO}/releases/download/${VERSION}/${SKILL_ASSET}"

# tty(대화형 입력) 실제 사용 가능 여부 — 존재만으로는 부족(curl|sh, CI 대응)
if { : < /dev/tty; } 2>/dev/null; then HAVE_TTY=1; else HAVE_TTY=0; fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
download() {
  if command -v curl >/dev/null 2>&1; then curl -fsSL "$1" -o "$2"; else wget -qO "$2" "$1"; fi
}

# ── 1. CLI 바이너리 → ~/.local/bin/kosis ──
echo "  CLI 바이너리 다운로드 중 ($BIN_ASSET)..."
download "$BIN_URL" "$TMP/kosis"
chmod +x "$TMP/kosis"
mkdir -p "$HOME/.local/bin"
cp "$TMP/kosis" "$HOME/.local/bin/kosis"
echo "  ✓ CLI: ~/.local/bin/kosis"

# ── 2. 스킬: 대상 선택 ──
if [ "$HAVE_TTY" = 1 ]; then
  printf "\n설치 대상을 선택하세요:\n  1) Claude\n  2) Codex\n  3) 둘 다\n> " > /dev/tty
  read TSEL < /dev/tty
  case "$TSEL" in 1) TARGET=claude ;; 2) TARGET=codex ;; 3) TARGET=both ;; *) TARGET=claude ;; esac
else
  TARGET="${KOSIS_TARGET:-claude}"
fi

# 범위 질문 (대상별로 개별) — $1: 표시명, $2: 비대화형 기본값
ask_scope() {
  if [ "$HAVE_TTY" = 1 ]; then
    printf "\n[%s] 설치 범위를 선택하세요:\n  1) 전역 (~/, 모든 프로젝트에서 사용)\n  2) 프로젝트 (현재 폴더만)\n> " "$1" > /dev/tty
    read SSEL < /dev/tty
    case "$SSEL" in 2) echo project ;; *) echo global ;; esac
  else
    case "${2:-global}" in project) echo project ;; *) echo global ;; esac
  fi
}

# 스킬 tar 다운로드 (한 번만)
echo "  스킬 파일 다운로드 중..."
download "$SKILL_URL" "$TMP/skill.tar.gz"

# 스킬 설치 — $1: 도구(claude/codex), $2: 범위(global/project)
install_skill() {
  _tool="$1"; _scope="$2"
  if [ "$_scope" = project ]; then _base="$(pwd)"; else _base="$HOME"; fi
  _dest="$_base/.$_tool/skills/kosis"
  rm -rf "$_dest"
  mkdir -p "$_dest"
  tar -xzf "$TMP/skill.tar.gz" -C "$_dest"
  echo "  ✓ 스킬($_tool, $_scope): $_dest"
}

case "$TARGET" in
  claude) install_skill claude "$(ask_scope Claude "$KOSIS_CLAUDE_SCOPE")" ;;
  codex)  install_skill codex  "$(ask_scope Codex  "$KOSIS_CODEX_SCOPE")" ;;
  both)
    install_skill claude "$(ask_scope Claude "$KOSIS_CLAUDE_SCOPE")"
    install_skill codex  "$(ask_scope Codex  "$KOSIS_CODEX_SCOPE")"
    ;;
  *) echo "알 수 없는 대상: $TARGET" >&2; exit 1 ;;
esac

# ── 3. PATH 안내 ──
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *)
    echo ""
    echo "PATH에 ~/.local/bin 추가가 필요합니다:"
    echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc   # zsh"
    echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc  # bash"
    echo "  새 터미널을 열거나 위 명령 실행 후 source ~/.zshrc"
    ;;
esac

echo ""
echo "✓ kosis $VERSION 설치 완료"

# ── 4. API 키 안내 ──
if [ ! -f "$HOME/.kosis/config.yaml" ] && [ -z "$KOSIS_API_KEY" ]; then
  echo ""
  echo "─────────────────────────────────────────────"
  echo " API 키 설정이 필요합니다:"
  echo "   kosis config setup        (대화형, 권장)"
  echo "   kosis config set-key KEY  (직접 입력)"
  echo " 키 발급: https://kosis.kr/openapi/"
  echo "─────────────────────────────────────────────"
fi
