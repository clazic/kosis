package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/relay"
)

func relayCmdEnv(t *testing.T, relayURL, n string) {
	t.Helper()
	dir := t.TempDir()
	config.SetConfigDirForTesting(dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("api_keys:\n  - FILEKEYmustNotLeak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOSIS_API_KEY", "ENVKEYmustNotLeak")
	t.Setenv("PES_RELAY_URL", relayURL)
	t.Setenv("PES_RELAY_TOKEN", "tok-cmd")
	t.Setenv("PES_RELAY_KOSIS_KEYS", n)
	relay.ResetForTesting(true)
	t.Cleanup(func() { config.SetConfigDirForTesting(""); relay.ResetForTesting(false) })
}

// 명령용 클라이언트: 설정 파일 · KOSIS_API_KEY 가 있어도 중계로 자리표시 키만 보낸다.
func TestRelayCmdClientIgnoresLocalKeys(t *testing.T) {
	var paths, queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		queries = append(queries, r.URL.RawQuery)
		if r.Header.Get("X-PES-Relay-Token") != "tok-cmd" {
			t.Errorf("토큰 머리글: %q", r.Header.Get("X-PES-Relay-Token"))
		}
		w.Write([]byte(`[{"TBL_NM":"인구","TBL_ID":"DT_1","ORG_ID":"101"}]`))
	}))
	defer srv.Close()
	relayCmdEnv(t, srv.URL, "2")

	for _, mk := range []func() (*api.Client, error){newCmdClient, NewAPIClient} {
		c, err := mk()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Search("인구", api.SearchOptions{ResultCount: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if len(paths) == 0 {
		t.Fatal("중계 호출이 없다")
	}
	for i, p := range paths {
		if p != "/kosis/https/kosis.kr/openapi/statisticsSearch.do" {
			t.Errorf("경로: %s", p)
		}
		if strings.Contains(queries[i], "mustNotLeak") || !strings.Contains(queries[i], "apiKey=__PES_KEY_") {
			t.Errorf("키는 자리표시여야 한다")
		}
	}
}

// 키 수 0: 규약의 글 그대로, 호출 없음.
func TestRelayCmdClientNoKeys(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	relayCmdEnv(t, srv.URL, "0")

	for name, mk := range map[string]func() (*api.Client, error){"newCmdClient": newCmdClient, "NewAPIClient": NewAPIClient} {
		_, err := mk()
		if err == nil || err.Error() != "PES 에 KOSIS 키가 없어요. PES 「조사 도구 · 그림 키」에서 키를 넣으세요." {
			t.Errorf("%s: %v", name, err)
		}
	}
	if hits != 0 {
		t.Errorf("호출이 있으면 안 된다: %d", hits)
	}
	if relay.Failed() == nil {
		t.Error("0 이 아닌 코드로 끝나게 표시돼야 한다")
	}
}

// 중계 모드에서는 업데이트 확인 알림을 끈다.
func TestRelayNoUpdateCheck(t *testing.T) {
	relayCmdEnv(t, "http://127.0.0.1:1", "1")
	t.Setenv("KOSIS_NO_UPDATE_CHECK", "")
	if shouldCheckUpdate() {
		t.Error("중계 모드에서는 업데이트 확인을 하지 않는다")
	}
}

// 중계 모드가 아니면 키 없음 안내가 지금과 같다.
func TestRelayOffNoKeyMessageUnchanged(t *testing.T) {
	dir := t.TempDir()
	config.SetConfigDirForTesting(dir)
	t.Cleanup(func() { config.SetConfigDirForTesting(""); relay.ResetForTesting(false) })
	t.Setenv("KOSIS_API_KEY", "")
	t.Setenv("PES_RELAY_URL", "")
	t.Setenv("PES_RELAY_KOSIS_KEYS", "0")
	relay.ResetForTesting(true)
	_, err := newCmdClient()
	if err == nil || err.Error() != "API 키가 설정되지 않았습니다\n\n"+config.NoAPIKeyMessage() {
		t.Errorf("오류 글이 바뀌었다: %v", err)
	}
	if relay.Failed() != nil {
		t.Error("중계 모드가 아니면 실패 표시가 없어야 한다")
	}
}
