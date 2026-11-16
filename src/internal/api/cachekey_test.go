package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 캐시 키·파일 이름·내용에 API 키가 들어가지 않는다 (캐시는 공개 통계 응답만, 사람 사이 공유 가능).
func TestCacheHasNoAPIKey(t *testing.T) {
	const secret = "SECRETkey+/abc=" // URL 인코딩되는 글자를 섞는다
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Query().Get("apiKey") != secret {
			t.Errorf("요청에 키가 없다: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`[{"TBL_NM":"인구"}]`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	c, err := NewClient([]string{secret})
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = srv.URL
	if err := c.InitCache(dir, 1); err != nil {
		t.Fatal(err)
	}

	// 이전 요청의 apiKey 가 이미 든 params 맵을 다시 넘겨도 키가 캐시 키에 들어가면 안 된다
	params := map[string]string{"searchNm": "인구", "apiKey": "OLDsecret"}
	if _, err := c.request("statisticsSearch.do?method=getList", params, false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "인구"}, false); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("두 번째 요청은 캐시에서 와야 한다 (서버 호출 %d번)", hits)
	}

	key := cacheKeyFor("statisticsSearch.do?method=getList", map[string]string{"searchNm": "인구", "apiKey": secret, "format": "json"})
	for _, k := range []string{secret, url.QueryEscape(secret), "OLDsecret", "apiKey"} {
		if strings.Contains(key, k) {
			t.Errorf("캐시 키에 %q: %s", k, key)
		}
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("캐시 파일 1개를 기대: %d", len(entries))
	}
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		for _, k := range []string{secret, url.QueryEscape(secret), "OLDsecret"} {
			if strings.Contains(e.Name(), k) || strings.Contains(string(data), k) {
				t.Errorf("캐시 파일 %s 에 키 %q", e.Name(), k)
			}
		}
	}
}

// 응답 본문에 키가 섞여 오면(오류 페이지가 요청 URL 을 되풀이하는 경우 등) 캐시하지 않는다.
func TestCacheSkipsBodyWithKey(t *testing.T) {
	const secret = "SECRETkey+/abc="
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"TBL_NM":"` + r.URL.RawQuery + `"}]`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	c, _ := NewClient([]string{secret})
	c.baseURL = srv.URL
	if err := c.InitCache(dir, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "x"}, false); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("키가 든 응답을 캐시했다: %d개", len(entries))
	}
}

// SetContext 의 ctx 가 끝나면 진행 중 HTTP 호출이 바로 끊긴다 (MCP 취소).
func TestContextCancelStopsRequest(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	c, _ := NewClient([]string{"k"})
	c.baseURL = srv.URL
	ctx, cancel := context.WithCancel(context.Background())
	c.SetContext(ctx)
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "x"}, true)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("취소되어야 한다: %v (%v)", err, time.Since(start))
	}
	if _, err := c.request("x", nil, true); err == nil || !strings.Contains(err.Error(), "취소") {
		t.Errorf("취소 뒤 호출은 바로 실패: %v", err)
	}
}
