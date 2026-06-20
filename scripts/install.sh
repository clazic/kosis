#!/bin/sh
# KOSIS CLI 설치 스크립트 (macOS / Linux)
# 사용법: curl -fsSL https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.sh | sh
set -e

REPO="clazic/kosis"
CLAUDE_SKILL="$HOME/.claude/skills/kosis"
CODEX_SKILL="$HOME/.codex/skills/kosis"

# 최신 버전 확인
if [ -n "$KOSIS_VERSION" ]; then
  VERSION="$KOSIS_VERSION"
else
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')"
fi

if [ -z "$VERSION" ]; then
  echo "오류: 버전 정보를 가져올 수 없습니다." >&2
  exit 1
fi

echo "kosis $VERSION 설치 중..."

# OS/아키텍처 감지
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

SKILL_ASSET="kosis-skill-${VERSION}.tar.gz"
BIN_ASSET="kosis-${OS}-${ARCH}"
SKILL_URL="https://github.com/${REPO}/releases/download/${VERSION}/${SKILL_ASSET}"
BIN_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BIN_ASSET}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 다운로드 함수 (curl 또는 wget)
download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  else
    wget -qO "$2" "$1"
  fi
}

# 1. 스킬 tarball 다운로드
echo "  스킬 파일 다운로드 중..."
download "$SKILL_URL" "$TMP/skill.tar.gz"

# 2. 스킬 파일 설치 (2곳)
for DEST in "$CLAUDE_SKILL" "$CODEX_SKILL"; do
  rm -rf "$DEST"
  mkdir -p "$DEST"
  tar -xzf "$TMP/skill.tar.gz" -C "$DEST"
  echo "  ✓ 스킬 파일 설치: $DEST"
done

# 3. OS별 바이너리 다운로드
echo "  바이너리 다운로드 중 ($BIN_ASSET)..."
download "$BIN_URL" "$TMP/kosis"
chmod +x "$TMP/kosis"

# 4. 바이너리를 두 스킬 폴더에 배치
for DEST in "$CLAUDE_SKILL" "$CODEX_SKILL"; do
  mkdir -p "$DEST/apps"
  cp "$TMP/kosis" "$DEST/apps/kosis-${OS}-${ARCH}"
  chmod +x "$DEST/apps/kosis-${OS}-${ARCH}"
done

# 5. ~/.local/bin/kosis symlink
mkdir -p "$HOME/.local/bin"
ln -sf "$CLAUDE_SKILL/apps/kosis-${OS}-${ARCH}" "$HOME/.local/bin/kosis"

# 6. PATH 안내
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

# 7. API 키 안내
if [ ! -f "$HOME/.kosis/config.yaml" ] && [ -z "$KOSIS_API_KEY" ]; then
  echo ""
  echo "─────────────────────────────────────────────"
  echo " API 키 설정이 필요합니다:"
  echo "   kosis config setup        (대화형, 권장)"
  echo "   kosis config set-key KEY  (직접 입력)"
  echo " 키 발급: https://kosis.kr/openapi/"
  echo "─────────────────────────────────────────────"
fi
