package catalog

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const sample = `{"o":"310","t":"TX_A019","n":"판매전력량 추이","p":["에너지","한국전력통계"],"a":["적용요금별","월별"],"i":["판매전력량 추이"],"c":["합계","산업용","주택용"],"pr":[["년","1961","2025"]]}
{"o":"310","t":"DT_N_A004","n":"용도별 판매전력량 추이","p":["에너지","한국전력통계"],"a":["용도별"],"i":["판매량"],"c":["합계","제조업"],"pr":[["년","2000","2025"]]}
{"o":"301","t":"DT_036Y001","n":"원/달러 환율","p":["금융"],"a":["통화"],"i":["환율"],"pr":[["월","1990","2014"]]}
{"o":"101","t":"DT_2IFS011","n":"국내 통화 환율","p":["국제"],"a":["국가"],"i":["USD당 국내 통화"],"pr":[["월","1979","2026"]]}
not json
{"o":"","t":"X"}
`

func write(t *testing.T, gz bool) string {
	t.Helper()
	dir := t.TempDir()
	if !gz {
		p := filepath.Join(dir, "c.jsonl")
		os.WriteFile(p, []byte(sample), 0o600)
		return p
	}
	p := filepath.Join(dir, "c.jsonl.gz")
	f, _ := os.Create(p)
	w := gzip.NewWriter(f)
	w.Write([]byte(sample))
	w.Close()
	f.Close()
	return p
}

func TestLoadAndSearch(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	for _, gz := range []bool{false, true} {
		c, err := Load(write(t, gz))
		if err != nil || len(c.Tables) != 4 {
			t.Fatalf("gz=%v: %v %d", gz, err, len(c.Tables))
		}
		// 표 이름에 없는 말(분류 코드 이름 '산업용')로도 찾는다
		h := c.Search("산업용 전력", 5, now)
		if len(h) != 1 || h[0].Tbl != "TX_A019" || len(h[0].Matched) != 1 || h[0].Matched[0] != "분류 산업용" {
			t.Fatalf("산업용: %+v", h)
		}
		// 낱말이 모두 있어야 한다, 이름에 든 표가 앞, 멈춘 표는 뒤
		h = c.Search("환율", 5, now)
		if len(h) != 2 || h[0].Tbl != "DT_2IFS011" || h[1].Tbl != "DT_036Y001" || h[0].End() != "2026" {
			t.Errorf("환율: %+v", h)
		}
		if h := c.Search("가", 5, now); h != nil {
			t.Errorf("한 글자: %+v", h)
		}
		if h := c.Search("판매전력량", 1, now); len(h) != 1 || h[0].Tbl != "DT_N_A004" && h[0].Tbl != "TX_A019" {
			t.Errorf("n 상한: %+v", h)
		}
	}
	var nilc *Catalog
	if nilc.Search("환율", 5, now) != nil {
		t.Error("nil 카탈로그")
	}
}

func TestPathEnv(t *testing.T) {
	p := write(t, true)
	t.Setenv("KOSIS_CATALOG_FILE", p)
	if Path() != p {
		t.Errorf("env: %q", Path())
	}
	t.Setenv("KOSIS_CATALOG_FILE", p+".none")
	if Path() != "" {
		t.Errorf("없는 파일: %q", Path())
	}
}
