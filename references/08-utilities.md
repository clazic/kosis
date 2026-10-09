# 8. 편의 기능

## 8.1 즐겨찾기 — `bookmark` (별칭: `bm`)

```bash
kosis bm add 101 DT_1IN1502 --name "인구통계"
kosis bm ls
kosis bm remove "인구통계"       # 이름 또는 인덱스로 제거
```

저장 위치: `~/.kosis/bookmarks.yaml`

## 8.2 조회 이력 — `history` (별칭: `hi`)

```bash
kosis hi                         # 최근 10개
kosis hi --limit 20              # 최근 20개
kosis hi replay 3                # 3번 이력 재실행
kosis hi clear                   # 전체 삭제
```

저장 위치: `~/.kosis/history.yaml` (최대 100개)

## 8.3 캐시 관리

```bash
kosis config cache-size          # 캐시 크기 확인
kosis config cache-clean         # 만료된 캐시 정리
kosis config cache-clear         # 전체 삭제
```

저장 위치: `~/.kosis/cache/` (기본 TTL: 24시간, 메타/검색만 캐시, 데이터는 캐시 안 함)

## 8.4 AI 공급자 설정

`quick --ai`는 공급자 2종을 함께 씁니다: 글생성(`hermes`, type openai, OpenAI 호환 Chat Completions)이 요청을 해석하고, 선택(`jev`, type typesafe, TypeSafe System One)이 실제 검색·메타 목록에서 통계표·코드를 고릅니다. 기본 글생성 공급자는 `hermes`(내장). 파이프라인 상세는 05번 문서 참고.

```bash
kosis config ai-list             # 공급자 목록 (주소·모델·키 출처: env:<변수>/config/없음, 역할 [글생성 (--ai)] / [선택 (Jev)])
kosis config ai-key hermes "<HERMES_API_KEY>"   # 키 저장 (환경변수 HERMES_API_KEY가 우선)
kosis config ai-key jev "<TYPESAFE_API_KEY>"    # Jev 키 (환경변수 TYPESAFE_API_KEY가 우선)
kosis config set-ai openai       # 기본 글생성 공급자 설정 (jev는 선택 전용이라 불가)
kosis config ai-add openai --url https://api.openai.com/v1 --model gpt-4o-mini --key-env OPENAI_API_KEY  # 추가·덮어쓰기 (--type openai|typesafe, 기본 openai)
kosis config ai-remove openai    # 제거 (기본 공급자 제거 시 hermes로 복귀)
```

예전 `ai.tools`/`cmd` 설정은 로드 시 hermes 기본값으로 자동 이관됩니다.

`quick --ai`의 학습 저장소 `~/.kosis/learnings.yaml`은 사용자 로컬 파일입니다(`kosis update`로 덮이지 않음, 초기화는 파일 삭제). 스킬의 `LEARNINGS.md`(사람·AI용 오답노트)와 별개입니다.
