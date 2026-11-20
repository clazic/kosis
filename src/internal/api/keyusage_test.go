package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// KeyUsage 키 자리별로 상위 요청 · 실패 · 429 를 센다 (보내지 않은 키는 빠진다).
func TestKeyUsageCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("apiKey") {
		case "k-500":
			w.WriteHeader(http.StatusInternalServerError)
		case "k-err":
			w.Write([]byte(`{"err":"20","errMsg":"x"}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	c, err := NewClient([]string{"k-ok", "k-500", "k-err", "k-unused"})
	if err != nil {
		t.Fatal(err)
	}
	c.SetBaseURL(srv.URL + "/")
	if got := c.KeyUsage(); len(got) != 0 {
		t.Fatalf("처음엔 비어 있어야 한다: %v", got)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ { // 워커 여럿이 같은 키를 함께 쓴다
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.requestWithKey("x.do?method=getList", map[string]string{"n": fmt.Sprint(i)}, true, i%3)
		}()
	}
	wg.Wait()
	want := []KeyUsage{{0, 7, 0, 0}, {1, 7, 7, 0}, {2, 6, 6, 0}}
	if got := c.KeyUsage(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("키별 요청 수:\n got %v\nwant %v", got, want)
	}

	// 전송 오류(서버가 닫힘)도 요청 한 번 · 실패 한 번
	srv.Close()
	_, _ = c.requestWithKey("x.do?method=getList", nil, true, 3)
	if got := c.KeyUsage(); len(got) != 4 || got[3] != (KeyUsage{3, 1, 1, 0}) {
		t.Errorf("전송 오류: %v", got)
	}
}
