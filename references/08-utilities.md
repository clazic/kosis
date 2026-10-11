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

## 8.4 AI 규칙 파일

AI 에이전트용 규칙은 두 파일입니다: 배포 규칙 `references/rules.yaml`(모든 사용자 공통)과 로컬 규칙 `~/.kosis/rules.d/*.yaml`(이 컴퓨터에서 AI가 s·m·d 하며 확인한 사실, 규칙 하나당 파일 하나). `kosis rules list`로 함께 보고, `kosis rules add`(YAML 표준입력)로 로컬 규칙을 저장하고, `kosis rules sync`로 여러 컴퓨터의 로컬 규칙을 비공개 git 저장소 하나로 합치고(처음 한 번 `--init <URL>`), `kosis rules validate`로 형식을 점검합니다. `~/.kosis`는 `kosis update`로 지워지지 않습니다.
