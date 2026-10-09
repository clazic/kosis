# 5. 자연어 조회 — `quick` (별칭: `q`)

한 줄로 통계를 조회합니다. 규칙 기반 매칭 또는 AI를 사용합니다.

```bash
# 규칙 기반 (기본)
kosis q "서울 미분양 최근 5년"
kosis q "GDP 2020~2024"
kosis q "인구 2015,2020,2025"

# AI 사용
kosis q "GDP 최근 5년" --ai                 # 기본 공급자(hermes)
kosis q "서울 노원구 인구" --ai openai      # ai-add로 등록한 공급자 (--ai=openai 도 가능)

# 확인 프롬프트 없이 실행 (스크립트·AI 에이전트 등 입력을 받을 수 없는 환경 — 없으면 "실행 취소됨"으로 끝남)
kosis q "서울 미분양 최근 5년" --yes      # 또는 -y

# 출력 형식/파일 지정
kosis q "소비자물가 월별" -f json
kosis q "서울 미분양" -o 미분양.xlsx
```

## 내장 사전 (규칙 기반)

| 구분 | 키워드 | 매핑 |
|------|--------|------|
| 지역 | 전국, 서울, 부산, 대구, 인천, 광주, 대전, 울산, 세종, 경기, 강원, 충북, 충남, 전북, 전남, 경북, 경남, 제주 | 시도 코드 (00, 11, 21, ...) |
| 통계표 | 미분양 | 116/DT_MLTM_2086 (연간) |
| | 소비자물가 | `kosis s "소비자물가지수"`로 검색 후 meta 확인 |
| | GDP | 301/DT_200Y101 (연간지표) |
| | 인구 | 101/DT_1IN1502 |
| | 경제활동인구 | 101/DT_1DA7002S |
| 기간 패턴 | "최근 N개월/년", "2020~2024", "2020,2022,2025" | 자동 인식 |

## AI 모드 지원 공급자

`--ai`는 OpenAI 호환 Chat Completions API(`POST {base_url}/chat/completions`)를 호출해 `kosis data` 명령을 생성합니다. 값을 생략하면 기본 공급자를 씁니다.

| 공급자 | 플래그 | 비고 |
|------|--------|------|
| hermes | `--ai` / `--ai hermes` | 기본·내장 (제거 불가, `ai-add`로 덮어쓰기 가능). base_url `http://100.70.244.98:8642/v1`, model `hermes-agent` (o 머신 Hermes Agent API, Tailscale) |
| 커스텀 | `--ai <이름>` / `--ai=<이름>` | `kosis config ai-add`로 사전 등록 필요 (OpenAI 호환 API) |

API 키는 환경변수(`HERMES_API_KEY` 등)가 우선이며, 없으면 `kosis config ai-key <공급자> <KEY>`로 저장한 값을 씁니다.

```bash
kosis config ai-key hermes "<HERMES_API_KEY>"
kosis config ai-add openai --url https://api.openai.com/v1 --model gpt-4o-mini --key-env OPENAI_API_KEY
```
