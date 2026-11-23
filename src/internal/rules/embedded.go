package rules

import _ "embed"

// embeddedRules 저장소 references/rules.yaml 의 사본 (make sync-rules 로 맞춘다).
// Go embed 는 모듈 밖 파일을 읽지 못해 사본을 둔다. 사본이 원본과 다르면 TestEmbeddedMatchesRepo 가 실패한다.
//
//go:embed embedded_rules.yaml
var embeddedRules []byte

// EmbeddedName 내장 규칙의 출처 표시 (파일 경로 자리에 쓴다).
const EmbeddedName = "(kosis 바이너리 내장)"

// Embedded 바이너리에 내장된 배포 규칙. 스킬 폴더가 없는 환경(kosis-mcp 서버 등)에서 쓴다.
func Embedded() ([]Entry, error) { return parseShipped(embeddedRules, EmbeddedName) }
