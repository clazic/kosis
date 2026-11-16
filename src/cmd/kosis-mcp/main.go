// kosis-mcp 는 KOSIS 조회 도구(kosis_search·kosis_meta·kosis_data·kosis_list·kosis_explain·kosis_indicator)를
// MCP(stdio, 줄 단위 JSON-RPC 2.0)로 내보내는 서버다. CLI(kosis)와 같은 API 클라이언트·키 풀·분할 조회·캐시·검사 코드를
// 함수로 직접 쓴다. stdout 에는 JSON-RPC 만, 진단은 stderr 로 (키 없이).
//
// 키: 기본은 환경변수 KOSIS_API_KEYS(쉼표로 여러 개), 없으면 ~/.kosis/config.yaml.
// tools/call 의 params._meta["kr.pes/keys"](문자열 배열)가 있으면 그 키들만 쓴다 (기본 키로 넘어가지 않는다).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/clazic/kosis/internal/cache"
	"github.com/clazic/kosis/internal/config"
)

var version = "dev"

const usage = `kosis-mcp — KOSIS(국가통계포털) 조회 MCP 서버 (stdio)

사용법:
  kosis-mcp            MCP 서버로 실행 (stdin/stdout, 줄 단위 JSON-RPC 2.0)
  kosis-mcp --http [주소]
                       Streamable HTTP 로 실행 (기본 127.0.0.1:8080, 경로 /mcp)
  kosis-mcp --version  버전
  kosis-mcp --help     이 도움말

키:
  KOSIS_API_KEYS       기본 API 키 (쉼표로 여러 개). 없으면 ~/.kosis/config.yaml 의 api_keys
  _meta["kr.pes/keys"] tools/call 마다 줄 키 (문자열 배열). 있으면 그 키만 쓴다
  X-Api-Keys: k1,k2    (HTTP) 그 POST 의 tools/call 에 요청별 키로

HTTP:
  KOSIS_MCP_TOKEN            있으면 Authorization: Bearer <토큰> 필수. loopback 이 아닌 주소는 토큰 없이 시작 거부
  KOSIS_MCP_ALLOWED_ORIGINS  허용할 Origin (쉼표). loopback 출처는 늘 허용

도구: kosis_search, kosis_meta, kosis_data, kosis_list, kosis_explain, kosis_indicator (읽기 전용)
`

func main() {
	httpAddr := ""
	argv := os.Args[1:]
	for i := 0; i < len(argv); i++ {
		switch a := argv[i]; {
		case a == "--version" || a == "-v" || a == "version":
			fmt.Println("kosis-mcp", version)
			return
		case a == "--help" || a == "-h" || a == "help":
			fmt.Print(usage)
			return
		case a == "--http":
			httpAddr = defaultHTTPAddr
			if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
				httpAddr = argv[i+1]
				i++
			}
		case strings.HasPrefix(a, "--http="):
			httpAddr = strings.TrimPrefix(a, "--http=")
		default:
			fmt.Fprintf(os.Stderr, "알 수 없는 인자: %s\n\n%s", a, usage)
			os.Exit(2)
		}
	}

	if httpAddr != "" {
		os.Stdout = os.Stderr
		s := newServer(nil, version, loadDefaultKeys(), openCache())
		if err := newHTTPServer(s, httpConfigFromEnv()).listenAndServe(httpAddr); err != nil {
			fmt.Fprintf(os.Stderr, "kosis-mcp: %v\n", s.scrub(err.Error(), nil))
			os.Exit(1)
		}
		return
	}

	// stdout 은 JSON-RPC 전용. 다른 코드가 실수로 fmt.Print 해도 프로토콜이 깨지지 않게 os.Stdout 을 stderr 로 돌린다.
	rpcOut := os.Stdout
	os.Stdout = os.Stderr

	s := newServer(rpcOut, version, loadDefaultKeys(), openCache())
	if err := s.serve(os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "kosis-mcp: %v\n", s.scrub(err.Error(), nil))
		os.Exit(1)
	}
}

// openCache CLI 와 같은 파일 캐시(~/.kosis/cache, TTL config cache_ttl_hours). 실패하면 캐시 없이.
func openCache() *cache.Cache {
	ttl := 24
	if cfg, err := config.Load(); err == nil && cfg.CacheTTLHours > 0 {
		ttl = cfg.CacheTTLHours
	}
	c, err := cache.New(filepath.Join(config.ConfigDir(), "cache"), ttl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kosis-mcp: 캐시 없이 실행합니다: %v\n", err)
		return nil
	}
	return c
}
