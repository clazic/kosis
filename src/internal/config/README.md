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
  default: claude
  tools:
    claude:
      cmd: "claude -p '{prompt}'"
    gemini:
      cmd: "gemini -p '{prompt}'"
    codex:
      cmd: "codex -p '{prompt}'"
```

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

## AI 도구 관리

```bash
# 등록된 AI 도구 목록 확인
kosis config ai-list

# 기본 AI 도구 설정
kosis config set-ai gemini

# 커스텀 AI 도구 추가
kosis config ai-add ollama "ollama run llama3 '{prompt}'"

# AI 도구 제거
kosis config ai-remove ollama
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

### AI 도구 관리

- `GetAIConfig() (AIConfig, error)` - AI 설정 반환
- `SetAIDefault(name string) error` - 기본 AI 도구 설정
- `AddAITool(name, cmd string) error` - 도구 추가
- `RemoveAITool(name string) error` - 도구 제거
- `ListAITools() ([]AIToolInfo, error)` - 도구 목록 반환

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
- AI 도구 관리
- 환경변수 우선순위
- 입력 검증

## 참고사항

- 설정 파일은 `~/.kosis/config.yaml` 형식으로 저장됩니다
- 디렉토리는 `700` 권한으로 생성됩니다 (사용자만 접근 가능)
- 환경변수 `KOSIS_API_KEY`는 설정 파일의 모든 키보다 우선됩니다
- API 키는 부분적으로만 표시됩니다 (보안)
