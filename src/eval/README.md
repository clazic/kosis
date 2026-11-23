# kosis AI 평가 세트

AI 가 kosis 로 통계 질문에 답하게 하고, 정답률 · 모르고 낸 오답 수 · 문항당 kosis 호출 수를 잰다.
릴리스 전후로 돌려 회귀를 잡고, 기능을 넣은 효과를 숫자로 확인한다.

- 문항: `questions.json` (129개 — 규칙에 있는 표 80, 처음 보는 표 33, 연쇄 16)
- 실행 · 채점: `../tools/run_eval.py` (표준 라이브러리만, AI 실행은 `claude -p`)
- 결과: `results/<시각>-<경로>-<모델>/` — `summary.md`, `results.jsonl`, `transcripts/` (git 무시)

## 쓰는 법

```bash
python3 -I src/tools/run_eval.py validate            # 문항 형식 점검
python3 -I src/tools/run_eval.py verify --workers 4  # 정답을 kosis 로 다시 조회해 대조 (AI 안 씀)
python3 -I src/tools/run_eval.py run --path cli --model sonnet
python3 -I src/tools/run_eval.py run --path mcp --mcp-bin /tmp/kosis-mcp --model sonnet
python3 -I src/tools/run_eval.py report src/eval/results/<폴더>   # 문항을 고친 뒤 다시 채점
```

- `--ids a,b` · `--types find,calc` · `--limit N` 으로 일부만 돌린다.
- `--kosis-bin` 으로 평가할 바이너리를 고른다 (기본은 PATH 의 kosis). 개발 중인 소스는 `cd src && go build -o /tmp/kosis-head .` 로 만든 것을 넘긴다.
- `run` 전에 `verify` 를 먼저 돌린다. 통계는 같은 시점 값도 개정된다 (잠정 → 확정). 어긋난 값이 개정이면 `verify --update` 로 고친다.

## 경로

| 경로 | AI 가 쓰는 것 |
|---|---|
| `cli` | kosis CLI + 스킬 문서 (SKILL.md · references · LEARNINGS.md) |
| `mcp` | kosis-mcp 도구만 (스킬 문서 없음) |

## 격리

- kosis 는 임시 HOME 에서 돈다: API 키만 복사하고, 로컬 규칙(`~/.kosis/rules.d`)은 비우고, 캐시는 새로 만든다.
- 스킬 문서는 임시 폴더로 복사해 그 폴더만 AI 에게 보여 준다. 저장소를 통째로 보여 주면 AI 가 이 폴더의 정답을 읽을 수 있다.
- `rules add · sync · revert`, `config` 변경, `update` 는 막는다. 앞 문항에서 배운 것이 뒤 문항을 돕지 않게 하기 위해서다.
- AI 는 사용자 설정(훅 · 스킬 · MCP)을 읽지 않고 뜬다 (`--setting-sources project,local --disable-slash-commands --strict-mcp-config`).

## 문항 형식

```json
{
 "id": "unsold-month",
 "type": "find",
 "question": "2026년 8월 전국 미분양 주택은 몇 호인가?",
 "source": "LEARNINGS RSI R2 — 월간 전국 미분양을 찾다 명령 19개",
 "answers": [
  {"table": "116/DT_MLTM_2080", "value": 69134, "unit": "호", "period": "202608",
   "truth": {"cmd": "d 116 DT_MLTM_2080 -c1 … -p M -s 202608 -e 202608"}}
 ]
}
```

| 필드 | 뜻 |
|---|---|
| `type` | `find` 표 찾기 · `codes` 코드 함정 · `period` 주기 · 시점 · `calc` 순위 · 증감 · 비율 · `definition` 헷갈리는 지표 · `absent` KOSIS 에서 못 찾은 것 |
| `answers` | 맞는 답 목록. 하나라도 맞으면 정답 (같은 값을 주는 표가 여럿일 때) |
| `value` | 숫자 또는 이름 문자열. 이름은 `aliases` 로 다른 표기를 받는다 (전라남도 · 전남) |
| `tol` · `abs_tol` | 허용 오차. 없으면 상대 0.5%. 가까운 오답이 있는 문항은 좁힌다 (생산액과 출하액은 0.18% 차이) |
| `truth.cmd` | 정답을 다시 조회하는 kosis 인자 (`d …` 또는 `ratio …`). `where` 로 행을 고르고 `field` 로 열을 고른다 (기본 `DT`). `agg: sum` 은 여러 행의 합. `truth` 가 목록이면 각 값을 더한다 (두 표를 더해야 하는 답) |
| `checked` | `absent` 문항에서 어디까지 찾아봤는지 |

### 묶음 (`set`)

| `set` | 뜻 |
|---|---|
| (없음) = `known` | `rules.yaml` 의 검증된 표에서 만든 문항 |
| `unseen` | 규칙 · 문서에 없는 표를 검색으로 찾아 만든 문항 |
| `chain` | 연쇄 문항 — 앞 표의 결과로 다음 표의 조회 대상이 정해진다 (`steps` 에 단계 수) |

### 연쇄 문항

```json
{
 "id": "c3-seoul-birth-pop", "type": "chain", "set": "chain", "steps": 3,
 "question": "2025년 합계출산율이 가장 낮은 시도를 찾고, 그 시도 안에서 … 그 구의 2025년 주민등록인구를 답하라.",
 "chain": [{"value": "서울특별시", "aliases": ["서울"]}, {"value": "송파구", "aliases": ["송파"]}],
 "answers": [{"table": "101/DT_1B040A3", "value": 643350, "truth": {"steps": [
   {"cmd": "d 101 DT_1B81A21 -c1 level:1 … --sort asc --top 1", "show": "C1_NM", "save": {"sido": "C1"}},
   {"cmd": "d 101 DT_1B81A23 -c1 {sido}* … --sort desc --top 2", "where": {"RANK": "2"}, "show": "C1_NM",
    "save": {"sgg": "C1"}, "map": {"sgg2": {"from": "sgg", "table": {"11240": "11710"}}}},
   {"cmd": "d 101 DT_1B040A3 -c1 {sgg2} -i T20 -p Y -s 2025 -e 2025"}]}}]
}
```

- `save` 는 고른 행의 열을 변수로 담고, 뒤 단계 `cmd` 의 `{변수}` 에 끼워 넣는다. 단계 값은 `{v1}`, `{v2}` … 로도 쓴다.
- `map` 은 표마다 다른 코드를 바꾼다. 앞 단계 답이 바뀌어 표에 없는 코드가 나오면 `verify` 가 알려 준다 — 그때 코드를 더 넣는다.
- `show` 는 그 단계에서 고른 것이다. 문항의 `chain` 과 순서대로 대조하고, AI 답의 `steps` 와도 대조해 **처음 틀린 단계**를 기록한다.
- `{"calc": "{v2}/{v3}*10000"}` 는 숫자 계산 단계다.
- 하위 지역을 `{sido}*` 로 조회하면 시도 합계 행이 섞인다. 건수처럼 합계가 늘 1위인 값은 `--top 2` 와 `"where": {"RANK": "2"}` 로 고른다.

문항에는 시점을 고정해 적는다 ("2025년", "2026년 8월"). "가장 최근" 은 시간이 지나면 정답이 바뀐다.

## 채점

AI 는 마지막에 `{"status","table","value","unit","period","note"}` JSON 으로 답한다.

| 결과 | 뜻 |
|---|---|
| `correct` | 값이 맞음 (`absent` 문항은 "못 찾았다" 고 답함) |
| `silent_wrong` | 답으로 냈는데 값이 틀림 — **모르고 낸 오답.** 가장 줄여야 하는 것 |
| `miss` | 답이 있는데 못 찾았다고 함 |
| `no_answer` | 형식에 맞는 답이 없음 (시간 초과 · 비용 상한 포함) |
| `review` | `absent` 문항에 답을 냄. 실제로 표를 찾았을 수 있으니 사람이 확인하고, 맞으면 문항과 `rules.yaml` 의 `gaps` 를 고친다 |

값이 맞으면 표가 달라도 정답이다. 정답 표와 같은 표를 썼는지는 따로 센다.

## 알아 둘 한계

- 문항 대부분이 `rules.yaml` 의 검증된 표에서 나왔다. `cli` 경로는 그 규칙을 읽을 수 있으므로 **이미 아는 표를 다시 맞히는 시험**에 가깝다. 회귀 확인과 `mcp` 경로 측정에는 맞지만, 처음 보는 표를 찾는 능력은 덜 잰다. 규칙에 없는 표로 만든 문항을 늘려야 한다.
- AI 답은 실행마다 달라진다. 한 번의 정답률 차이 몇 %p 는 잡음일 수 있다. 비교할 때는 같은 모델로 여러 번 돌린다.
- `absent` 문항은 "검색으로 못 찾았다" 는 기록이지 "KOSIS 에 없다" 는 증명이 아니다.
