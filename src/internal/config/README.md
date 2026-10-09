# KOSIS CLI - Config 패키지

`internal/config` 패키지는 KOSIS CLI의 모든 설정을 관리합니다.

## 설정 파일 위치

설정 파일은 사용자 홈 디렉토리의 `.kosis` 디렉토리에 저장됩니다.

```
~/.kosis/config.yaml
```

## 설정 파일 형식

```yaml
api_keys:
  - "ZTQ2ZGRm..."    # 기본 키
  - "MWIxNzRk..."    # 추가 키
default_format: table
cache_ttl_hours: 24
ai:
  default: hermes
  providers:
    hermes:
      base_url: http://100.70.244.98:8642/v1
      model: hermes-agent
      key_env: HERMES_API_KEY
    jev:                       # 선택 전용(TypeSafe System One), 이름 고정
      type: typesafe
      base_url: https://api.typesafe.ai/v1
      model: jev-latest
      key_env: TYPESAFE_API_KEY
    openai:
      base_url: https://api.openai.com/v1
      model: gpt-4o-mini
      key_env: OPENAI_API_KEY
```

예전 `ai.tools`/`cmd` 설정은 로드 시 hermes 기본값으로 자동 이관됩니다.

## API 키 설정

### 환경변수 사용 (최우선)

```bash
export KOSIS_API_KEY="your_api_key"
```

### 명령어로 설정

```bash
# 단일 키 설정 (기존 키 제거)
kosis config set-key "your_api_key"

# 여러 키 추가 (병렬 조회용)
kosis config add-key "api_key_2"
kosis config add-key "api_key_3"

# 키 목록 확인
kosis config key-list

# 키 제거
kosis config remove-key 1
```

## AI 공급자 관리

`AIProvider.Type`은 두 종류입니다 (비면 `openai`).

- `openai`: 글생성. OpenAI 호환 Chat Completions(`POST {base_url}/chat/completions`)로 요청을 검색어·조건·기간으로 해석 (기본 hermes).
- `typesafe`: 선택. TypeSafe System One(`{base_url}/systemone`)으로 후보 중 통계표·코드를 고름 (jev, 이름 고정, `set-ai` 기본값 불가).

환경변수 키가 설정 파일 키보다 우선합니다.

```bash
# 등록된 공급자 목록 (주소·모델·키 출처)
kosis config ai-list

# 기본 공급자 설정
kosis config set-ai openai

# 공급자 추가·덮어쓰기
kosis config ai-add openai --url https://api.openai.com/v1 --model gpt-4o-mini --key-env OPENAI_API_KEY [--type openai|typesafe]

# API 키 저장
kosis config ai-key hermes "<HERMES_API_KEY>"
kosis config ai-key jev "<TYPESAFE_API_KEY>"

# 공급자 제거 (기본 공급자 제거 시 hermes로 복귀)
kosis config ai-remove openai
```

## 주요 함수

### 설정 로드/저장

- `Load() (*Config, error)` - 설정 파일 로드
- `Save(cfg *Config) error` - 설정 파일 저장
- `DefaultConfig() *Config` - 기본 설정 반환

### API 키 관리

- `GetAPIKeys() ([]string, error)` - API 키 목록 반환
- `GetFirstAPIKey() (string, error)` - 첫 번째 API 키 반환
- `AddAPIKey(key string) error` - 키 추가
- `RemoveAPIKey(index int) error` - 키 제거
- `SetDefaultKey(key string) error` - 단일 키 설정
- `HasAPIKey() bool` - 키 설정 여부 확인

### 출력 형식 관리

- `GetDefaultFormat() (string, error)` - 기본 형식 반환
- `SetDefaultFormat(format string) error` - 형식 설정

### 캐시 관리

- `SetCacheTTL(hours int) error` - 캐시 TTL 설정

### AI 공급자 관리

- `GetAIProvider(name string) (string, AIProvider, error)` - 공급자 조회 (빈 이름이면 기본 공급자)
- `SetAIDefault(name string) error` - 기본 공급자 설정
- `AddAIProvider(name string, p AIProvider) error` - 공급자 추가·덮어쓰기
- `SetAIProviderKey(name, key string) error` - 공급자 API 키 저장
- `RemoveAIProvider(name string) error` - 공급자 제거
- `AIProvider.IsTypeSafe() bool` - typesafe(선택용) 공급자 여부 (Type이 비면 openai)
- `SelectorProvider` - 후보 중 하나를 고르는 공급자(Jev) 이름 상수
- `MaskKey(key string) string` - 키 마스킹 표시
- `AIProvider.ResolveKey() string` - 키 결정 (환경변수 우선, 없으면 설정 파일)

### 유틸리티

- `ConfigDir() string` - 설정 디렉토리 경로
- `ConfigFilePath() string` - 설정 파일 경로
- `EnsureConfigDir() error` - 디렉토리 생성
- `NoAPIKeyMessage() string` - API 키 없을 때 안내 메시지

## 우선순위

API 키 설정의 우선순위는 다음과 같습니다:

1. **환경변수** `KOSIS_API_KEY` (최우선)
2. **설정 파일** `~/.kosis/config.yaml`
3. **기본값** 빈 배열

## 에러 처리

모든 함수는 에러를 반환하므로 반드시 처리해야 합니다:

```go
keys, err := config.GetAPIKeys()
if err != nil {
    log.Fatal(err)
}
```

## 테스트

전체 테스트 실행:

```bash
go test ./internal/config -v
```

테스트 커버리지:

- 기본 설정
- 설정 파일 로드/저장
- API 키 추가/제거
- AI 공급자 관리
- 환경변수 우선순위
- 입력 검증

## 참고사항

- 설정 파일은 `~/.kosis/config.yaml` 형식으로 저장됩니다
- 디렉토리는 `700` 권한으로 생성됩니다 (사용자만 접근 가능)
- 환경변수 `KOSIS_API_KEY`는 설정 파일의 모든 키보다 우선됩니다
- API 키는 부분적으로만 표시됩니다 (보안)
