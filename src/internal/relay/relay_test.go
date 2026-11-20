package relay

import (
	"net/url"
	"testing"
)

func TestRewrite(t *testing.T) {
	cases := []struct{ base, in, want string }{
		{"http://127.0.0.1:53999", "https://kosis.kr/openapi/statisticsSearch.do?method=getList&apiKey=__PES_KEY_0__&searchNm=%EC%9D%B8%EA%B5%AC",
			"http://127.0.0.1:53999/kosis/https/kosis.kr/openapi/statisticsSearch.do?method=getList&apiKey=__PES_KEY_0__&searchNm=%EC%9D%B8%EA%B5%AC"},
		{"http://127.0.0.1:53999", "http://kosis.kr:8080/a%2Fb/c", "http://127.0.0.1:53999/kosis/http/kosis.kr:8080/a%2Fb/c"},
		{"http://127.0.0.1:53999", "https://kosis.kr", "http://127.0.0.1:53999/kosis/https/kosis.kr"},
		// 이미 중계 주소면 그대로
		{"http://127.0.0.1:53999", "http://127.0.0.1:53999/kosis/https/kosis.kr/x?y=1", "http://127.0.0.1:53999/kosis/https/kosis.kr/x?y=1"},
		// 중계와 같은 호스트라도 /kosis/ 아래가 아니면 감싼다
		{"http://127.0.0.1:53999", "http://127.0.0.1:53999/other", "http://127.0.0.1:53999/kosis/http/127.0.0.1:53999/other"},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.in)
		got, err := rewrite(c.base, u)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("rewrite(%s)\n got  %s\n want %s", c.in, got, c.want)
		}
	}
	if _, err := rewrite("not a url", &url.URL{Scheme: "https", Host: "kosis.kr"}); err == nil {
		t.Error("잘못된 PES_RELAY_URL 은 오류여야 한다")
	}
}

func TestActiveAndKeys(t *testing.T) {
	t.Cleanup(func() { ResetForTesting(false) })
	t.Setenv("PES_RELAY_URL", "http://127.0.0.1:53999/")
	t.Setenv("PES_RELAY_KOSIS_KEYS", "2")

	ResetForTesting(false)
	if Active() {
		t.Error("CLI 가 켜지 않았으면 중계 모드가 아니다")
	}
	ResetForTesting(true)
	if !Active() {
		t.Error("CLI + PES_RELAY_URL 이면 중계 모드다")
	}
	keys, err := Keys()
	if err != nil || len(keys) != 2 || keys[0] != "__PES_KEY_0__" || keys[1] != "__PES_KEY_1__" {
		t.Errorf("자리표시 키: %v %v", keys, err)
	}
	if Failed() != nil {
		t.Error("키가 있으면 실패 표시가 없어야 한다")
	}

	t.Setenv("PES_RELAY_KOSIS_KEYS", "0")
	if _, err := Keys(); err != ErrNoKey || err.Error() != "PES 에 kosis 키가 없어요. PES 「조사 도구 · 그림 키」에서 키를 넣으세요." {
		t.Errorf("키 없음 글: %v", err)
	}
	if Failed() == nil {
		t.Error("키 없음은 실패로 표시돼야 한다")
	}

	t.Setenv("PES_RELAY_URL", "")
	if Active() {
		t.Error("PES_RELAY_URL 이 없으면 중계 모드가 아니다")
	}
}
