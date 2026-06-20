package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/clazic/kosis/internal/cache"
)

const (
	baseURL        = "https://kosis.kr/openapi"
	httpTimeout    = 30 * time.Second
	defaultFormat  = "json"
	defaultJSONVD  = "Y"
	defaultJSONMVD = "Y"
	maxRetries     = 3               // 429 에러 재시도 최대 횟수
	initialBackoff = 1 * time.Second // 첫 번째 재시도 대기시간 (exponential backoff)

	// HTTP Transport 튜닝 (설계서 Step 1): 동시 분할 조회 시 TCP 연결 재사용
	maxIdleConns        = 100
	maxIdleConnsPerHost = 16 // 병렬 워커 동시성에 맞춘 호스트당 유휴 연결 수 (기본 2는 너무 작음)
	idleConnTimeout     = 90 * time.Second
)

// Client represents KOSIS API client.
type Client struct {
	baseURL    string
	apiKeys    []string
	httpClient *http.Client
	cache      *cache.Cache // 파일 기반 캐시로 변경
	keyIndex   int
	keyIndexMu sync.Mutex
	keyPool    chan int // 키 풀(설계서 Step 2): 가용 키 인덱스를 버퍼드 채널로 관리(borrow/return)
}

// NewClient creates a new KOSIS API client.
func NewClient(apiKeys []string) (*Client, error) {
	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("at least one API key is required")
	}

	// 키 풀 초기화(설계서 Step 2): 모든 키 인덱스를 버퍼드 채널에 채워둠
	keyPool := make(chan int, len(apiKeys))
	for i := range apiKeys {
		keyPool <- i
	}

	return &Client{
		baseURL: baseURL,
		apiKeys: apiKeys,
		keyPool: keyPool,
		httpClient: &http.Client{
			Timeout: httpTimeout,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment, // 기본 프록시 동작 보존
				MaxIdleConns:        maxIdleConns,
				MaxIdleConnsPerHost: maxIdleConnsPerHost,
				MaxConnsPerHost:     0, // 무제한 (rate limiter/세마포어가 제어)
				IdleConnTimeout:     idleConnTimeout,
				ForceAttemptHTTP2:   true,
			},
		},
		cache:    nil, // 나중에 InitCache로 초기화
		keyIndex: 0,
	}, nil
}

// InitCache는 파일 기반 캐시를 초기화합니다.
// 설정이 로드된 후 호출되어야 합니다.
func (c *Client) InitCache(cacheDir string, ttlHours int) error {
	fileCache, err := cache.New(cacheDir, ttlHours)
	if err != nil {
		return err
	}
	c.cache = fileCache
	return nil
}

// getNextAPIKey returns the next API key in round-robin fashion.
func (c *Client) getNextAPIKey() string {
	c.keyIndexMu.Lock()
	defer c.keyIndexMu.Unlock()

	key := c.apiKeys[c.keyIndex]
	c.keyIndex = (c.keyIndex + 1) % len(c.apiKeys)
	return key
}

// borrowKey 키 풀에서 가용 키 인덱스를 하나 빌려옵니다(설계서 Step 2).
// 가용 키가 없으면 반납될 때까지 블록됩니다.
func (c *Client) borrowKey() int {
	return <-c.keyPool
}

// returnKey 빌린 키 인덱스를 즉시 풀에 반납합니다.
// 풀 용량 == 키 개수이므로 반납은 절대 블록되지 않습니다.
func (c *Client) returnKey(idx int) {
	c.keyPool <- idx
}

// cooldownReturn 429를 받은 키를 d만큼 쿨다운한 뒤 풀에 반납합니다(동일 키 연타 방지).
// 호출자를 블록하지 않도록 별도 goroutine에서 대기 후 반납합니다.
// 호출 시점의 풀 채널을 캡처하여, 이후 c.keyPool이 재초기화(분류 축 2차 분할의 그룹별 실행)되어도
// 지연 반납이 다른 실행의 풀을 오염시키지 않도록 합니다.
func (c *Client) cooldownReturn(idx int, d time.Duration) {
	pool := c.keyPool
	go func() {
		if d > 0 {
			time.Sleep(d)
		}
		pool <- idx
	}()
}

// request performs a common API request with automatic parameter handling.
// noCache가 true일 때는 캐시를 사용하지 않습니다 (설계서 8.5절: 데이터 응답은 캐시 안함).
func (c *Client) request(endpoint string, params map[string]string, noCache bool) ([]byte, error) {
	return c.requestWithKey(endpoint, params, noCache, -1) // -1은 라운드로빈 사용
}

// requestWithKey performs an API request with a specific API key.
// keyIndex >= 0이면 해당 인덱스의 키를 사용, -1이면 라운드로빈 사용.
// 워커 풀 기반 병렬 조회(설계서 8.5절)에서 각 워커가 특정 키로 요청하도록 함.
// 429 Too Many Requests 에러 시 exponential backoff로 자동 재시도합니다 (최대 3회).
func (c *Client) requestWithKey(endpoint string, params map[string]string, noCache bool, keyIndex int) ([]byte, error) {
	// noCache가 false일 때만 캐시 조회
	var cacheKey string
	if !noCache && c.cache != nil {
		cacheKey = endpoint + "?" + url.Values(convertParamsToURLValues(params)).Encode()
		if data, found := c.cache.Get(cacheKey); found {
			return data, nil
		}
	}

	// Add common parameters
	if params == nil {
		params = make(map[string]string)
	}

	// API 키 선택: keyIndex >= 0이면 특정 키, -1이면 라운드로빈
	if keyIndex < 0 {
		params["apiKey"] = c.getNextAPIKey()
	} else {
		params["apiKey"] = c.apiKeys[keyIndex%len(c.apiKeys)]
	}
	params["format"] = defaultFormat
	// 메타 API(getMeta)는 jsonVD/jsonMVD 없이 호출해야 OBJ_NM, ITM_NM 등 전체 필드를 받음
	if !strings.Contains(endpoint, "getMeta") {
		params["jsonVD"] = defaultJSONVD
		params["jsonMVD"] = defaultJSONMVD
	}

	// Build query URL
	fullURL := fmt.Sprintf("%s/%s", c.baseURL, endpoint)
	queryURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// 엔드포인트의 기존 query string 보존 (예: method=getMeta)
	queryParams := queryURL.Query()
	for k, v := range params {
		queryParams.Set(k, v)
	}
	queryURL.RawQuery = queryParams.Encode()

	// 429 에러 발생 시 exponential backoff로 재시도
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Make HTTP request
		req, err := http.NewRequest("GET", queryURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("API request failed: %w", err)
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() // 루프 내에서 명시적으로 닫음 (defer는 함수 종료 시에만 실행되므로 루프 내 리소스 누수 방지)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		// 429 Too Many Requests 에러 처리
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("API rate limit exceeded (429)")
			if attempt < maxRetries {
				// Exponential backoff: 1초, 2초, 4초
				time.Sleep(backoff)
				backoff *= 2
				continue
			}
			return nil, fmt.Errorf("API rate limit exceeded after %d retries: %w", maxRetries, lastErr)
		}

		// Check HTTP status code
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, maskAPIKey(string(body)))
		}

		// Check for API error response (err field)
		// Handle both object and array responses
		if len(body) > 0 && body[0] == '{' {
			var errResp ErrorResponse
			if err := json.Unmarshal(body, &errResp); err == nil && errResp.Err != "" {
				return nil, fmt.Errorf("API 오류 [%s]: %s", errResp.Err, errResp.ErrMsg)
			}
		}

		// noCache가 false일 때만 캐시 저장 (설계서 8.5절에 따라 메타/검색만 캐시)
		if !noCache && c.cache != nil {
			if err := c.cache.Set(cacheKey, body); err != nil {
				// 캐시 저장 실패는 무시하고 데이터만 반환
				fmt.Fprintf(os.Stderr, "캐시 저장 오류: %v\n", err)
			}
		}

		return body, nil
	}

	return nil, lastErr
}

// convertParamsToURLValues converts string map to url.Values.
func convertParamsToURLValues(params map[string]string) map[string][]string {
	result := make(map[string][]string)
	for k, v := range params {
		result[k] = []string{v}
	}
	return result
}

// maskAPIKey masks any API key that appears in the given string.
// Replaces patterns like apiKey=XXXX with apiKey=XX****XX to prevent key leakage in error messages.
func maskAPIKey(s string) string {
	// apiKey= 뒤의 값을 마스킹
	result := s
	for {
		idx := strings.Index(result, "apiKey=")
		if idx < 0 {
			break
		}
		start := idx + len("apiKey=")
		end := start
		for end < len(result) && result[end] != '&' && result[end] != '"' && result[end] != ' ' && result[end] != '\n' {
			end++
		}
		key := result[start:end]
		var masked string
		if len(key) > 4 {
			masked = key[:2] + strings.Repeat("*", len(key)-4) + key[len(key)-2:]
		} else {
			masked = strings.Repeat("*", len(key))
		}
		result = result[:start] + masked + result[end:]
	}
	return result
}
