# Module Context

이 패키지는 API 키, 기본 포맷, AI 도구 등록/선택 등 사용자 설정을 관리한다. quick와 대부분의 명령이 여기에 의존한다.

# Tech Stack & Constraints

- Viper 기반 설정 파일
- 환경변수 우선 정책

Constraints:
- `KOSIS_API_KEY` 우선순위를 깨지 않는다.
- 테스트는 실제 홈 디렉토리를 오염시키지 않아야 한다.
- 기본 AI 도구와 커스텀 도구 정책은 quick 동작과 맞아야 한다.

# Implementation Patterns

- 설정 경로/파일 쓰기와 값 검증을 분리한다.
- AI 도구 추가/제거/기본값 변경은 결정론적으로 동작해야 한다.
- README나 help에 드러나는 설정 키는 실제 구조체와 일치해야 한다.

# Testing Strategy

- `go test ./internal/config`
- 전체 영향 확인: `go test ./...`

# Local Golden Rules

Do:
- 설정 정책 변경 시 테스트를 같이 업데이트한다.
- quick나 root가 참조하는 기본값이 바뀌면 관련 경로도 같이 검증한다.

Don't:
- 환경변수 우선 규칙을 약화시키지 않는다.
- 커스텀 AI 도구 제거 시 기본값 처리처럼 사용자 영향이 큰 분기를 무테스트로 바꾸지 않는다.
