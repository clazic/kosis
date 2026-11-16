package api

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// 성질: 나눈 묶음마다 URL 이 한도 이하이고, 나눈 인자들의 코드 조합(곱)은 빠짐·겹침 없이 원래와 같으며 순서도 같다.
func TestSplitForURLBoundProperty(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	c, _ := NewClient([]string{"KEY+/with=chars0123456789abcdefghijklmnopqrstuvwxyz"})
	codes := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = strconv.Itoa(10000000 + r.IntN(90000000))[:2+r.IntN(7)] + "_" + fmt.Sprint(i) // 길이가 제각각인 고유 코드
		}
		return out
	}
	for trial := 0; trial < 40; trial++ {
		a, b := codes(1+r.IntN(1500)), codes(1+r.IntN(400))
		opts := DataOptions{Class1: strings.Join(a, "+"), Class2: "ALL", Item: strings.Join(b, "+"), PrdSe: "Y", StartPrdDe: "2015", EndPrdDe: "2025"}
		subs := c.splitForURL("101", "DT_1IN1502", opts)
		type pair struct{ a, b string }
		seen := map[pair]int{}
		var orderA []string // class1 코드가 처음 나온 차례 = 원래 순서 (같은 시점 안 행 순서가 묶음 순서를 따른다)
		firstA := map[string]bool{}
		for _, so := range subs {
			if l := c.dataURLLen("101", "DT_1IN1502", so); l > maxRequestURLLen {
				t.Fatalf("trial %d: 묶음 URL %d자 > %d", trial, l, maxRequestURLLen)
			}
			if so.Class2 != "ALL" || so.PrdSe != "Y" || so.StartPrdDe != "2015" {
				t.Fatalf("나누지 않는 인자가 바뀌었다: %+v", so)
			}
			for _, x := range strings.Split(so.Class1, "+") {
				if !firstA[x] {
					firstA[x] = true
					orderA = append(orderA, x)
				}
			}
			for _, x := range strings.Split(so.Class1, "+") {
				for _, y := range strings.Split(so.Item, "+") {
					seen[pair{x, y}]++
				}
			}
		}
		if len(seen) != len(a)*len(b) {
			t.Fatalf("trial %d: 조합 %d개, 기대 %d", trial, len(seen), len(a)*len(b))
		}
		for p, n := range seen {
			if n != 1 {
				t.Fatalf("trial %d: 조합 %v 가 %d번", trial, p, n)
			}
		}
		if strings.Join(orderA, "+") != opts.Class1 {
			t.Fatalf("trial %d: class1 코드 순서가 바뀌었다", trial)
		}
	}
	// 짧으면 그대로
	if subs := c.splitForURL("101", "T", DataOptions{Class1: "00+11", Item: "T100"}); len(subs) != 1 || subs[0].Class1 != "00+11" {
		t.Errorf("짧은 조회는 그대로: %+v", subs)
	}
}

// fakeKOSIS 101/DT_1IN1502 모양 (분류 1축 4,158개 코드 × 항목 1 × 2015~2025 연간). URL 이 limit 를 넘으면 [21], 7,950자를 넘으면 414.
type fakeKOSIS struct {
	codes    []string
	limit    int
	tooLong  atomic.Int32
	dataReqs atomic.Int32
	maxURI   atomic.Int32
}

func newFakeKOSIS(n, limit int) *fakeKOSIS {
	f := &fakeKOSIS{limit: limit}
	f.codes = append(f.codes, "00")
	for i := 1; i < n; i++ {
		f.codes = append(f.codes, fmt.Sprintf("%d%06d", 11+i%28, i)) // 8자 읍면동 코드 모양
	}
	return f
}

func (f *fakeKOSIS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if l := int32(len(r.RequestURI)); l > f.maxURI.Load() {
		f.maxURI.Store(l)
	}
	if full := len("https://kosis.kr") + len(r.RequestURI); full > f.limit { // 실제 KOSIS 처럼: 4000바이트 넘으면 [21], 아주 길면 414
		f.tooLong.Add(1)
		if full > 7950 {
			w.WriteHeader(http.StatusRequestURITooLong)
			w.Write([]byte("<html>414 Request-URI Too Large</html>"))
			return
		}
		w.Write([]byte(`{"err":"21","errMsg":" 호출하신 URL 길이가 4000Byte를 넘어서 요청하실 수 없습니다."}`))
		return
	}
	q := r.URL.Query()
	var out []map[string]string
	switch {
	case q.Get("method") == "getMeta" && q.Get("type") == "ITM":
		for _, c := range f.codes {
			out = append(out, map[string]string{"OBJ_ID": "A", "OBJ_NM": "행정구역별(읍면동)", "OBJ_ID_SN": "1", "ITM_ID": c, "ITM_NM": "지역" + c})
		}
		out = append(out, map[string]string{"OBJ_ID": "ITEM", "OBJ_NM": "항목", "ITM_ID": "T100", "ITM_NM": "총인구", "UNIT_NM": "명"})
	case q.Get("method") == "getMeta" && q.Get("type") == "PRD":
		out = append(out, map[string]string{"PRD_SE": "Y", "STRT_PRD_DE": "2015", "END_PRD_DE": "2025"})
	case strings.Contains(r.URL.Path, "statisticsParameterData"):
		f.dataReqs.Add(1)
		cs := f.codes
		if q.Get("objL1") != "ALL" {
			cs = strings.Split(q.Get("objL1"), "+")
		}
		s, _ := strconv.Atoi(q.Get("startPrdDe"))
		e, _ := strconv.Atoi(q.Get("endPrdDe"))
		for y := s; y <= e; y++ {
			for _, c := range cs {
				out = append(out, map[string]string{"C1": c, "C1_NM": "지역" + c, "ITM_ID": "T100", "ITM_NM": "총인구", "PRD_SE": "A", "PRD_DE": strconv.Itoa(y), "DT": "1"})
			}
		}
	default:
		w.Write([]byte(`{"err":"20","errMsg":"bad"}`))
		return
	}
	json.NewEncoder(w).Encode(out)
}

// 회귀: kosis d 101 DT_1IN1502 -c1 ALL -i T100 -p Y -s 2015 -e 2025 — 분류 축 2차 분할이 코드 수천 개를 나열해 414 가 났다.
func TestAutoSplitDT1IN1502ShapeStaysUnderURLLimit(t *testing.T) {
	f := newFakeKOSIS(4158, 4000) // 실측 한도 (3,994 OK · 4,005 [21])
	srv := httptest.NewServer(f)
	defer srv.Close()
	c, _ := NewClient([]string{"k1-0123456789abcdef0123456789abcdef0123=", "k2-0123456789abcdef0123456789abcdef0123="})
	c.baseURL = srv.URL
	var mu sync.Mutex
	var prog [][2]int
	rows, err := c.DataWithAutoSplit("101", "DT_1IN1502",
		DataOptions{Class1: "ALL", Item: "T100", PrdSe: "Y", StartPrdDe: "2015", EndPrdDe: "2025"},
		SplitOptions{MaxCells: 40000}, func(d, n int) { mu.Lock(); prog = append(prog, [2]int{d, n}); mu.Unlock() })
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if n := f.tooLong.Load(); n != 0 {
		t.Fatalf("URL 길이 오류를 %d번 받았다 (최대 URI %d자)", n, f.maxURI.Load())
	}
	if want := 4158 * 11; len(rows) != want {
		t.Fatalf("행 %d개, 기대 %d", len(rows), want)
	}
	seen := map[string]bool{}
	for i, r := range rows {
		k := r.C1 + "|" + r.PrdDe
		if seen[k] {
			t.Fatalf("중복 행 %s", k)
		}
		seen[k] = true
		if i > 0 && rows[i-1].PrdDe > r.PrdDe {
			t.Fatalf("시점 순서가 깨졌다: %s 뒤 %s", rows[i-1].PrdDe, r.PrdDe)
		}
	}
	// 같은 시점 안에서는 메타 코드 순서
	for i, r := range rows[:4158] {
		if r.C1 != f.codes[i] {
			t.Fatalf("2015 의 %d번째 코드 %s, 기대 %s", i, r.C1, f.codes[i])
		}
	}
	if len(prog) < 2 || prog[len(prog)-1][0] != prog[len(prog)-1][1] {
		t.Errorf("진행 n/전체가 끝까지 와야 한다: %v", prog)
	}
	t.Logf("데이터 요청 %d번, 묶음 %d개, 최대 URI %d자", f.dataReqs.Load(), prog[len(prog)-1][1], f.maxURI.Load())
}

// 사용자가 코드를 길게 나열한 비연속 시점 조회(--periods)도 URL 한도 안에서 나눠 받는다.
func TestDataWithPeriodsLongCodeList(t *testing.T) {
	f := newFakeKOSIS(3000, 4000)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c, _ := NewClient([]string{"k"})
	c.baseURL = srv.URL
	rows, err := c.DataWithPeriods("101", "DT_1IN1502", []string{"2020", "2024"},
		DataOptions{Class1: strings.Join(f.codes, "+"), Item: "T100", PrdSe: "Y"})
	if err != nil {
		t.Fatal(err)
	}
	if f.tooLong.Load() != 0 || len(rows) != 3000*2 {
		t.Fatalf("414 %d번, 행 %d개", f.tooLong.Load(), len(rows))
	}
}
