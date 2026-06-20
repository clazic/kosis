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

## 8.4 AI 도구 설정

```bash
kosis config set-ai claude       # 기본 AI 설정
kosis config ai-list             # 등록된 AI 목록
kosis config ai-add ollama "ollama run llama3 '{prompt}'"  # 커스텀 추가
kosis config ai-remove ollama    # 제거
```
