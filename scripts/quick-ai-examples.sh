#!/usr/bin/env sh
# kosis quick --ai 예시 10가지를 순서대로 실행합니다 (macOS / Linux).
#
# 사용법:
#   sh scripts/quick-ai-examples.sh            # 하나씩 실행, 매번 확인(y/n) — 확인한 선택은 학습됨
#   sh scripts/quick-ai-examples.sh --yes      # 확인 없이 연속 실행 (학습하지 않음)
#   sh scripts/quick-ai-examples.sh 7          # 7번만 실행
#
# 환경변수:
#   KOSIS_BIN        사용할 kosis 경로 (기본: PATH 의 kosis)
#   HERMES_API_KEY   글생성 키 (또는 kosis config ai-key hermes <KEY>)
#   TYPESAFE_API_KEY 선택(Jev) 키 (또는 kosis config ai-key jev <KEY>)

KOSIS="${KOSIS_BIN:-kosis}"
YES=""
ONLY=""
for arg in "$@"; do
  case "$arg" in
    -y|--yes) YES="--yes" ;;
    [0-9]|10) ONLY="$arg" ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    *) echo "알 수 없는 인자: $arg (--help 참고)"; exit 1 ;;
  esac
done

if ! command -v "$KOSIS" >/dev/null 2>&1; then
  echo "kosis 를 찾을 수 없습니다. 설치하거나 KOSIS_BIN 으로 경로를 지정하세요."
  exit 1
fi
echo "사용 바이너리: $(command -v "$KOSIS") ($("$KOSIS" --version 2>/dev/null))"
echo "※ --ai 의 hermes 해석 + Jev 선택 + 학습은 v0.5.7 이상에서 동작합니다."
echo

# 번호|요청|무엇을 보여주는 예시인가
EXAMPLES='1|작년까지 최근 5년간 울산에서 태어난 아기 수|상대 기간("작년") 해석 + 지역 + 일상어("태어난 아기")→출생아수
2|서울 아파트 매매가격지수 최근 6개월|규칙 사전에 없는 통계, 월간 표, 유일한 값은 [유일]로 자동 확정
3|부산 소비자물가지수 2024년 월별|연도 범위를 월 형식으로 변환 (202401~202412)
4|전국 실업률 최근 8분기|요청 주기(분기)가 없는 표면 연간으로 환산 (8분기→2년). 신뢰도가 낮으면 ⚠ 표시
5|경기도 혼인 건수 2020년부터|"~부터"(끝 없음)를 올해까지로 해석
6|제주 고용률 최근 3년|분류축 2개(시도·성별)를 한 번에 병렬 선택
7|대구 65세 이상 인구 최근 5년|교정 학습 연습: 추계인구 표를 고르면 n → "통계표가 틀림"으로 정답을 가르쳐 보세요
8|2023년 시도별 합계출산율 중 세종|단일 연도 + 지역 하나 고르기
9|전국 사업체 수 최근 3년|타 기관(고용노동부) 표, 분류축 3개(산업·성별·규모)
10|강원 인구 순이동 2022~2024|연속 범위, 음수 값 포함 결과'

TOTAL=10
echo "$EXAMPLES" | while IFS='|' read -r NO REQ DESC; do
  [ -n "$ONLY" ] && [ "$ONLY" != "$NO" ] && continue
  echo "════════════════════════════════════════════════════════════"
  echo " [$NO/$TOTAL] $REQ"
  echo "   ↳ $DESC"
  echo "════════════════════════════════════════════════════════════"
  # </dev/tty: while 루프의 표준입력은 예시 목록이므로, 확인 프롬프트는 터미널에서 받는다
  if [ -n "$YES" ]; then
    "$KOSIS" q "$REQ" --ai --yes </dev/null
  else
    "$KOSIS" q "$REQ" --ai </dev/tty
  fi
  if [ -z "$ONLY" ] && [ "$NO" != "$TOTAL" ] && [ -z "$YES" ]; then
    printf '\n▶ Enter: 다음 예시 / q: 종료 '
    read -r ANSWER </dev/tty
    [ "$ANSWER" = "q" ] && break
  fi
  echo
done
