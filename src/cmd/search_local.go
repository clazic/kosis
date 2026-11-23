package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/catalog"
)

// maxLocalHits 검색 결과 앞에 붙이는 로컬 카탈로그 표 수.
const maxLocalHits = 5

// localHit 로컬 카탈로그에서 찾은 표 (KOSIS 검색 결과와 같은 키 + SOURCE·MATCH).
type localHit struct {
	res   api.SearchResult
	match string
}

// localSearch 1쪽·거르기 없는 검색이면 로컬 카탈로그에서 찾은 표 중 KOSIS 결과에 없는 것을 돌려준다.
// note 는 stderr·notices 한 줄 (카탈로그가 없거나 찾은 것이 없으면 "").
// 결과 수의 절반까지만 (최대 maxLocalHits) — KOSIS 결과를 밀어내지 않게.
func localSearch(keyword string, page int, filtered bool, kosis []api.SearchResult, limit int) (hits []localHit, note string) {
	if page != 1 || filtered {
		return nil, ""
	}
	capN := min(maxLocalHits, max(1, limit/2))
	c := catalog.Default()
	if c == nil {
		return nil, ""
	}
	seen := map[string]bool{}
	for _, r := range kosis {
		seen[r.OrgID+"/"+r.TblID] = true
	}
	for _, h := range c.Search(keyword, maxLocalHits*3, time.Now()) {
		if seen[h.Org+"/"+h.Tbl] {
			continue
		}
		hits = append(hits, localHit{
			res: api.SearchResult{OrgID: h.Org, TblID: h.Tbl, TblNM: h.Name, MTAtitle: strings.Join(h.Path, " > "),
				StrtPrdDe: h.Start(), EndPrdDe: h.End()},
			match: strings.Join(h.Matched, ", "),
		})
		if len(hits) == capN {
			break
		}
	}
	if len(hits) == 0 {
		return nil, ""
	}
	return hits, fmt.Sprintf("로컬 카탈로그(표 %d개, %s 수집)에서 KOSIS 검색에 없던 표 %d개를 앞에 붙였습니다 (SOURCE=로컬, MATCH=표 이름 밖에서 맞은 말) — 수록 끝 시점(END_PRD_DE)을 보고 고르세요",
		len(c.Tables), c.ModTime.Format("2006-01-02"), len(hits))
}

// mergeLocal 로컬 표를 앞에, KOSIS 결과를 뒤에 — 합쳐 limit 개.
func mergeLocal(local []localHit, kosis []api.SearchResult, limit int) (rs []api.SearchResult, src, match []string) {
	for _, h := range local {
		rs, src, match = append(rs, h.res), append(src, "로컬"), append(match, h.match)
	}
	for _, r := range kosis {
		rs, src, match = append(rs, r), append(src, "KOSIS"), append(match, "")
	}
	if len(rs) > limit {
		rs, src, match = rs[:limit], src[:limit], match[:limit]
	}
	return rs, src, match
}
