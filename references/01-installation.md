# 1. 설치 및 초기 설정

## 1.1 설치

**바이너리 다운로드:**

```bash
# macOS (Apple Silicon)
curl -L https://github.com/<owner>/kosis-cli/releases/latest/download/kosis-darwin-arm64 -o /usr/local/bin/kosis
chmod +x /usr/local/bin/kosis

# Linux (amd64)
curl -L https://github.com/<owner>/kosis-cli/releases/latest/download/kosis-linux-amd64 -o /usr/local/bin/kosis
chmod +x /usr/local/bin/kosis

# Windows (amd64) - PowerShell
Invoke-WebRequest -Uri "https://github.com/<owner>/kosis-cli/releases/latest/download/kosis-windows-amd64.exe" -OutFile "$env:USERPROFILE\bin\kosis.exe"
# $env:USERPROFILE\bin 을 PATH에 추가
```

**jq 설치 (JSON 파이프라인 사용 시 필요):**

```bash
# macOS
brew install jq

# Linux (Ubuntu/Debian)
sudo apt install jq

# Linux (CentOS/RHEL/Fedora)
sudo yum install jq          # CentOS/RHEL
sudo dnf install jq          # Fedora

# Windows (Chocolatey)
choco install jq

# Windows (Scoop)
scoop install jq

# Windows (winget)
winget install jqlang.jq
```

**Go로 직접 빌드:**

```bash
git clone https://github.com/<owner>/kosis-cli.git
cd kosis-cli

# 현재 OS용 빌드
make build                    # → bin/kosis

# 전체 OS 크로스 컴파일
make build-all
# → bin/mac/kosis             macOS arm64
# → bin/linux/kosis           Linux amd64
# → bin/windows/kosis.exe     Windows amd64

# 로컬 설치 (/usr/local/bin)
make install
```

**Windows PATH 설정:**

```powershell
# PowerShell (현재 세션)
$env:PATH += ";$env:USERPROFILE\bin"

# PowerShell (영구 설정)
[Environment]::SetEnvironmentVariable("PATH", "$env:PATH;$env:USERPROFILE\bin", "User")

# CMD에서 한글 출력 깨짐 방지
chcp 65001
```

## 1.2 API 키 설정

```bash
# 방법 1: CLI로 설정
kosis config set-key <YOUR_API_KEY>

# 방법 2: 환경변수 (우선순위 최상위)
export KOSIS_API_KEY="<YOUR_API_KEY>"

# 대용량 병렬 조회용: API 키 추가 등록
kosis config add-key <API_KEY_2>
kosis config add-key <API_KEY_3>
kosis config key-list              # 등록된 키 확인
```

> **API 키 발급 절차:**
> 1. https://kosis.kr/openapi/ 접속
> 2. 회원가입/로그인
> 3. "인증키 신청" 메뉴에서 API 키 발급
> 4. 발급된 키를 `kosis config set-key`로 등록
> 5. (선택) 대용량 조회 시 추가 키를 발급받아 `config add-key`로 등록

## 1.3 설정 파일

경로: `~/.kosis/config.yaml`

```yaml
api_keys: ["키1", "키2"]       # API 키 목록
default_format: "table"        # 기본 출력 형식
cache_ttl_hours: 24            # 캐시 유효 시간
ai:
  default: "hermes"            # 기본 AI 공급자 (OpenAI 호환 API)
  providers:
    hermes:
      base_url: "http://100.70.244.98:8642/v1"
      model: "hermes-agent"
      key_env: "HERMES_API_KEY"   # 키는 환경변수 또는 `kosis config ai-key`로 지정
    jev:                          # 선택 전용(통계표·코드 고르기). type: typesafe, 이름 jev 고정
      type: "typesafe"
      base_url: "https://api.typesafe.ai/v1"
      model: "jev-latest"
      key_env: "TYPESAFE_API_KEY"
```

`quick --ai`는 글생성 공급자(hermes)와 선택 공급자(jev)를 함께 씁니다. 학습 결과는 별도 파일 `~/.kosis/learnings.yaml`에 저장됩니다 (05번 문서 참고).
