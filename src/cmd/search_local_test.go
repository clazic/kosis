package cmd

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/catalog"
)

func TestLocalSearchMerge(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.jsonl.gz")
	f, _ := os.Create(p)
	w := gzip.NewWriter(f)
	w.Write([]byte(`{"o":"310","t":"TX_A019","n":"판매전력량 추이","p":["에너지"],"c":["산업용"],"pr":[["년","1961","2025"]]}
{"o":"310","t":"TX_A025","n":"전력판매수입 추이","p":["에너지"],"c":["산업용"],"pr":[["년","1979","2025"]]}
{"o":"310","t":"TX_A017","n":"고객호수 추이 전력","p":["에너지"],"c":["산업용"],"pr":[["년","1961","2025"]]}
`))
	w.Close()
	f.Close()
	t.Setenv("KOSIS_CATALOG_FILE", p)
	if catalog.Default() == nil { // 프로세스에 한 번 읽는다 — 다른 테스트가 먼저 읽었으면 건너뛴다
		t.Skip("카탈로그를 이미 다른 경로로 읽음")
	}
	kosis := []api.SearchResult{{OrgID: "310", TblID: "TX_A025", TblNM: "전력판매수입 추이"}, {OrgID: "101", TblID: "DT_X"}}
	// limit 4 → 로컬은 절반(2)까지, KOSIS 에 이미 있는 표는 빼고
	local, note := localSearch("산업용 전력", 1, false, kosis, 4)
	if len(local) != 2 || note == "" {
		t.Fatalf("로컬: %+v %q", local, note)
	}
	for _, h := range local {
		if h.res.TblID == "TX_A025" {
			t.Error("KOSIS 결과와 중복")
		}
		if h.match != "분류 산업용" {
			t.Errorf("MATCH: %q", h.match)
		}
	}
	rs, src, _ := mergeLocal(local, kosis, 4)
	if len(rs) != 4 || src[0] != "로컬" || src[1] != "로컬" || src[2] != "KOSIS" || rs[2].TblID != "TX_A025" {
		t.Errorf("합치기: %+v %v", rs, src)
	}
	if l, _ := localSearch("산업용 전력", 2, false, kosis, 4); l != nil {
		t.Error("2쪽에는 붙이지 않는다")
	}
	if l, _ := localSearch("산업용 전력", 1, true, kosis, 4); l != nil {
		t.Error("거르기가 있으면 붙이지 않는다")
	}
}
