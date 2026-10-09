#!/usr/bin/env sh
# kosis quick --ai 학습 평가 (macOS / Linux)
#
# 어려운 혼합 질문 n개를 여러 라운드 돌려 질문별 점수(100점)를 매기고,
# 라운드 사이에 사람의 교정을 정답으로 흉내 내 학습시켜 점수가 어떻게 오르는지 보여줍니다.
# 실제 학습 파일(~/.kosis/learnings.yaml)과 스킬 문서는 바뀌지 않습니다 (임시 학습 파일 사용).
#
# 사용법:
#   sh scripts/quick-ai-eval.sh              # 전체 질문, 3라운드
#   sh scripts/quick-ai-eval.sh 10           # 앞에서 10개만
#   sh scripts/quick-ai-eval.sh 10 5         # 10개, 5라운드
#
# 환경변수:
#   KOSIS_BIN        kosis 경로 (기본: 저장소 src/ 가 있으면 빌드해서 사용, 없으면 PATH 의 kosis)
#   HERMES_API_KEY   글생성 키 (또는 kosis config ai-key hermes <KEY>)
#   TYPESAFE_API_KEY 선택(Jev) 키 (또는 kosis config ai-key jev <KEY>)

LIMIT="${1:-0}"
ROUNDS="${2:-3}"
case "$LIMIT$ROUNDS" in *[!0-9]*) sed -n '2,17p' "$0"; exit 1 ;; esac

DIR=$(cd "$(dirname "$0")" && pwd)
QUESTIONS="$DIR/quick-ai-eval.yaml"

KOSIS="${KOSIS_BIN:-}"
if [ -z "$KOSIS" ]; then
  if [ -f "$DIR/../src/go.mod" ] && command -v go >/dev/null 2>&1; then
    # 저장소에서 실행하면 현재 소스로 빌드 (quick-eval 은 v0.5.8 이상에만 있음)
    TMPD=$(mktemp -d "${TMPDIR:-/tmp}/.kosis-eval-bin.XXXXXX") || exit 1
    trap 'rm -rf "$TMPD"' EXIT INT TERM
    echo "🔨 현재 소스로 kosis 빌드 중..."
    (cd "$DIR/../src" && go build -o "$TMPD/kosis" .) || exit 1
    KOSIS="$TMPD/kosis"
  else
    KOSIS="kosis"
  fi
fi

if ! "$KOSIS" quick-eval --help >/dev/null 2>&1; then
  echo "이 kosis 에는 quick-eval 이 없습니다 ($("$KOSIS" --version 2>/dev/null)). v0.5.8 이상으로 업데이트하거나 저장소에서 실행하세요."
  exit 1
fi

KOSIS_NO_UPDATE_CHECK=1 "$KOSIS" quick-eval "$QUESTIONS" --limit "$LIMIT" --rounds "$ROUNDS"
