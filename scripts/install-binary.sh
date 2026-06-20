#!/bin/sh
# Claude 플러그인용 — 바이너리만 다운로드하여 apps/ 에 배치
set -e

SKILL_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
REPO="clazic/kosis"

# 버전 읽기
VERSION="$(cat "$SKILL_DIR/VERSION" 2>/dev/null | tr -d '[:space:]')"
if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')"
fi

# OS/아키텍처 감지
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "지원하지 않는 아키텍처: $ARCH" >&2; exit 1 ;;
esac

ASSET="kosis-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
DEST="$SKILL_DIR/apps/${ASSET}"

echo "kosis ${VERSION} 바이너리 다운로드 중 (${ASSET})..." >&2

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$TMP/kosis"
else
  wget -qO "$TMP/kosis" "$URL"
fi

mkdir -p "$SKILL_DIR/apps"
cp "$TMP/kosis" "$DEST"
chmod +x "$DEST"

echo "✓ 바이너리 설치 완료: $DEST" >&2
