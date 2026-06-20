package api

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SplitOptions 분할 조회 옵션
type SplitOptions struct {
	MaxCells    int  // 최대 셀 수 (기본 40000)
	NoAutoSplit bool // 자동 분할 비활성화
	Concurrency int  // 동시 조회 워커 수 (0/미지정 시 자동: max(키 개수, 2))
}

// effectiveConcurrency 사용자가 지정한 동시성 또는 자동 기본값(max(키 개수, 2))을 반환합니다.
func (c *Client) effectiveConcurrency(splitOpts SplitOptions) int {
	if splitOpts.Concurrency > 0 {
		return splitOpts.Concurrency
	}
	n := len(c.apiKeys)
	if n < 2 {
		n = 2
	}
	return n
}

// PeriodChunk 시점 축으로 분할된 청크
type PeriodChunk struct {
	Start string
	End   string
}

// DataWithAutoSplit 4만 셀 초과 시 자동 분할 조회
// meta API로 분류값/항목 개수를 파악하고, 예상 셀 수를 계산한 후,
// 4만 초과 시 자동으로 시점 축 분할 조회를 수행합니다.
// API 키가 여러 개면 워커 풀 기반 병렬 조회를 수행합니다.
func (c *Client) DataWithAutoSplit(orgID, tblID string, opts DataOptions, splitOpts SplitOptions, progressFn func(current, total int)) ([]DataRow, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}

	// 기본값 설정
	if splitOpts.MaxCells == 0 {
		splitOpts.MaxCells = 40000
	}

	// 1. Meta API로 예상 셀 수 계산
	summary, summaryErr := c.MetaSummary(orgID, tblID)
	if summaryErr != nil {
		// Meta 조회 실패 시: start/end가 있으면 보수적으로 5개 청크로 분할 시도
		if opts.StartPrdDe != "" && opts.EndPrdDe != "" {
			conservativeChunks := c.splitByPeriod(opts, splitOpts.MaxCells*5+1, splitOpts.MaxCells)
			if len(conservativeChunks) > 0 {
				fmt.Fprintf(os.Stderr, "⚠ 메타 조회 실패로 보수적 분할(%d 청크)을 시도합니다.\n", len(conservativeChunks))
				if c.effectiveConcurrency(splitOpts) <= 1 {
					return c.dataWithAutoSplitSequential(orgID, tblID, opts, conservativeChunks, progressFn)
				}
				return c.dataWithAutoSplitParallel(orgID, tblID, opts, conservativeChunks, c.effectiveConcurrency(splitOpts), progressFn)
			}
		}
		// start/end도 없으면 안내 메시지와 함께 에러 반환
		return nil, fmt.Errorf("메타 정보 조회에 실패했습니다. --start, --end로 시점 범위를 지정하거나 --latest로 최근 N개만 조회하세요: %w", summaryErr)
	}
	estimatedCells := estimateCellCountFromSummary(summary, opts)

	// 2. 4만 이하면 일반 조회
	if estimatedCells <= splitOpts.MaxCells {
		if progressFn != nil {
			progressFn(1, 1)
		}
		return c.Data(orgID, tblID, opts)
	}

	// 3. NoAutoSplit이 true면 에러 반환
	if splitOpts.NoAutoSplit {
		return nil, fmt.Errorf("조회 데이터가 %d셀로 4만 셀 제한을 초과합니다. --no-auto-split을 제거하거나 범위를 축소하세요", estimatedCells)
	}

	// 4. 20만 초과 시 경고 (지금은 경고만, 실제로는 프롬프트 필요)
	if estimatedCells > 200000 {
		fmt.Fprintf(os.Stderr, "⚠ 예상 셀 수: 약 %d건. 이는 조회 시간이 오래 걸릴 수 있습니다.\n", estimatedCells)
		fmt.Fprintf(os.Stderr, "  범위를 축소하거나 --periods를 사용하여 특정 시점만 조회하세요.\n")
		// 현재 단계에서는 경고만 하고 계속 진행
	}

	// 5. start/end가 없으면 메타에서 추출 (분할에 필요)
	if opts.StartPrdDe == "" || opts.EndPrdDe == "" {
		if summary != nil && len(summary.Periods) > 0 {
			for _, p := range summary.Periods {
				prdCode := strings.ToUpper(strings.TrimSpace(p.PrdSe))
				// 수록주기가 매칭되면 시점 범위를 설정
				optsPrd := strings.ToUpper(opts.PrdSe)
				prdMatch := (optsPrd == "" || prdCode == optsPrd ||
					(optsPrd == "Y" && (prdCode == "년" || prdCode == "Y")) ||
					(optsPrd == "M" && (prdCode == "월" || prdCode == "M")) ||
					(optsPrd == "Q" && (prdCode == "분기" || prdCode == "Q")) ||
					(optsPrd == "H" && (prdCode == "반기" || prdCode == "H")))
				if prdMatch && p.StrtPrdDe != "" && p.EndPrdDe != "" {
					// 시점 형식 정규화 (예: "1995.01" → "199501", "2025 4/4" → "20254")
					start := strings.ReplaceAll(strings.ReplaceAll(p.StrtPrdDe, ".", ""), " ", "")
					end := strings.ReplaceAll(strings.ReplaceAll(p.EndPrdDe, ".", ""), " ", "")
					// 분기 형식 변환: "19951/4" → "19951"
					start = strings.Split(start, "/")[0]
					end = strings.Split(end, "/")[0]

					// --latest N 사용 시: 전체 범위가 아닌 최근 N개 시점만 계산
					if opts.NewEstPrdCnt != "" {
						if n, parseErr := strconv.Atoi(opts.NewEstPrdCnt); parseErr == nil && n > 0 {
							start = c.calcLatestStart(end, n, opts.PrdSe)
						}
					}

					opts.StartPrdDe = start
					opts.EndPrdDe = end
					opts.NewEstPrdCnt = "" // start/end를 쓰므로 latest 제거
					break
				}
			}
		}
	}

	// Step 7a/7b: 단일 시점 하나가 이미 MaxCells를 초과하면 시점(기간) 축 분할로는 해결 불가.
	// 7b: 가능하면 분류 축으로 2차 분할하여 자동 조회. 7a: 불가하면 원인+해결 안내.
	if np := countPeriods(opts); np > 0 {
		cellsPerPeriod := (estimatedCells + np - 1) / np
		if cellsPerPeriod > splitOpts.MaxCells {
			if subOpts, ok := c.subdivideByClass(summary, opts, splitOpts.MaxCells, cellsPerPeriod); ok {
				fmt.Fprintf(os.Stderr, "ℹ 단일 시점이 %d셀 제한을 초과하여 분류 축으로 %d개 그룹으로 2차 분할합니다.\n", splitOpts.MaxCells, len(subOpts))
				return c.dataBySubOptions(orgID, tblID, subOpts, splitOpts, progressFn)
			}
			return nil, fmt.Errorf(
				"이 통계표는 단일 시점 하나가 약 %d셀로 %d셀 제한을 초과하여, 시점(기간) 축 분할로는 조회할 수 없습니다.\n"+
					"  → 분류값(--class1/--class2 등)을 특정 코드로 좁히거나 --item으로 항목을 지정하여 조회 범위를 줄이세요.\n"+
					"  (예: --class1 ALL 대신 특정 시도/시군구 코드를 지정)",
				cellsPerPeriod, splitOpts.MaxCells)
		}
	}

	// 시점 축으로 분할
	chunks := c.splitByPeriod(opts, estimatedCells, splitOpts.MaxCells)
	if len(chunks) == 0 {
		// 시점 분할이 불가능하고 예상 셀이 제한 초과이면 에러 반환
		if estimatedCells > splitOpts.MaxCells {
			return nil, fmt.Errorf("예상 셀 수(%d)가 %d셀 제한을 초과하지만 시점 축 분할이 불가능합니다. "+
				"분류값(--class1 등)을 좁히거나 --item으로 항목을 지정하여 조회 범위를 축소하세요", estimatedCells, splitOpts.MaxCells)
		}
		// 제한 이하이면 일반 조회
		return c.Data(orgID, tblID, opts)
	}

	// 6. 동시성에 따라 순차 또는 병렬 실행 (키 개수와 분리 — 키 1개여도 concurrency>1이면 병렬)
	concurrency := c.effectiveConcurrency(splitOpts)
	if concurrency <= 1 {
		return c.dataWithAutoSplitSequential(orgID, tblID, opts, chunks, progressFn)
	}
	return c.dataWithAutoSplitParallel(orgID, tblID, opts, chunks, concurrency, progressFn)
}

// dataWithAutoSplitSequential 순차 실행 (API 키 1개일 때)
func (c *Client) dataWithAutoSplitSequential(orgID, tblID string, opts DataOptions, chunks []PeriodChunk, progressFn func(current, total int)) ([]DataRow, error) {
	var allResults []DataRow
	for i, chunk := range chunks {
		if progressFn != nil {
			progressFn(i+1, len(chunks))
		}

		// 분할된 시점으로 옵션 생성
		chunkOpts := opts
		chunkOpts.StartPrdDe = chunk.Start
		chunkOpts.EndPrdDe = chunk.End

		results, err := c.Data(orgID, tblID, chunkOpts)
		if err != nil {
			// API 오류 코드 30: "데이터가 존재하지 않습니다" → 해당 구간은 건너뜀
			if strings.Contains(err.Error(), "API 오류 [30]") {
				continue
			}
			return allResults, fmt.Errorf("분할 조회 [%s~%s] 실패: %w", chunk.Start, chunk.End, err)
		}

		allResults = append(allResults, results...)
	}

	// 정렬 및 중복 제거
	sortByPeriod(allResults)
	allResults = deduplicateRows(allResults)

	return allResults, nil
}

// dataWithAutoSplitParallel 작업 큐 + 키 풀 기반 병렬 실행 (설계서 Step 3).
// 워커 수는 키 개수와 분리되어 concurrency로 제어됩니다.
func (c *Client) dataWithAutoSplitParallel(orgID, tblID string, opts DataOptions, chunks []PeriodChunk, concurrency int, progressFn func(current, total int)) ([]DataRow, error) {
	return c.runParallelChunks(chunks, concurrency, progressFn, func(keyIdx int, chunk PeriodChunk) ([]DataRow, error) {
		chunkOpts := opts
		chunkOpts.StartPrdDe = chunk.Start
		chunkOpts.EndPrdDe = chunk.End
		// 429 재시도는 requestWithKey가 단일 책임(설계서 Step 4). splitter는 키 재배정만 담당.
		return c.dataWithSpecificKey(orgID, tblID, chunkOpts, keyIdx)
	})
}

// runParallelChunks 청크들을 작업 큐 + 키 풀로 병렬 처리합니다 (설계서 Step 3·4).
// fetch를 주입받아 HTTP 없이 단위 테스트가 가능합니다.
//   - 워커 수 = min(concurrency, len(chunks)) → 키 개수와 무관하게 동시 요청 수를 제어.
//   - 키 풀: concurrency 크기의 토큰을 키에 라운드로빈 분배. 429 키는 쿨다운 반납으로 자연 회피.
//   - 429가 fetch 단일 경로에서 끝까지 남으면 청크를 큐에 재투입(상한 maxRequeue), 다른 가용 키로 흘러감.
//   - 결과는 인덱스 슬롯에 기록해 순서를 보존한 뒤 정렬·중복 제거 (기존 동작 유지).
func (c *Client) runParallelChunks(chunks []PeriodChunk, concurrency int, progressFn func(current, total int), fetch func(keyIdx int, chunk PeriodChunk) ([]DataRow, error)) ([]DataRow, error) {
	type chunkResult struct {
		Index int
		Data  []DataRow
		Err   error
	}
	type job struct {
		index    int
		chunk    PeriodChunk
		requeues int
	}
	const maxRequeue = 2

	if len(chunks) == 0 {
		return nil, nil
	}

	numWorkers := concurrency
	if numWorkers < 1 {
		numWorkers = 1
	}
	if numWorkers > len(chunks) {
		numWorkers = len(chunks)
	}

	// 키 풀을 동시성 크기로 구성: 토큰을 키에 라운드로빈 분배(키가 적어도 concurrency만큼 동시 요청 가능).
	nKeys := len(c.apiKeys)
	if nKeys < 1 {
		nKeys = 1
	}
	c.keyPool = make(chan int, numWorkers)
	for i := 0; i < numWorkers; i++ {
		c.keyPool <- i % nKeys
	}

	jobs := make(chan job, len(chunks))
	results := make(chan chunkResult, len(chunks))
	done := make(chan struct{})
	for i, chunk := range chunks {
		jobs <- job{index: i, chunk: chunk}
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// 종료 신호 우선 확인
				select {
				case <-done:
					return
				default:
				}
				select {
				case <-done:
					return
				case j := <-jobs:
					keyIdx := c.borrowKey()
					data, err := fetch(keyIdx, j.chunk)
					if err != nil && strings.Contains(err.Error(), "429") && j.requeues < maxRequeue {
						// 429: 키 쿨다운 반납 후 청크 재투입. jobs는 닫지 않으므로 send 안전.
						backoff := time.Duration(1<<uint(j.requeues)) * initialBackoff
						c.cooldownReturn(keyIdx, backoff)
						j.requeues++
						jobs <- j
						continue
					}
					c.returnKey(keyIdx)
					results <- chunkResult{Index: j.index, Data: data, Err: err}
				}
			}
		}()
	}

	// 결과 수집: 각 청크는 최종적으로 정확히 1개의 결과를 생성. 인덱스 슬롯에 기록해 순서 보존.
	allResults := make([]chunkResult, len(chunks))
	var lastErr error
	var fatalErr error
	for i := 0; i < len(chunks); i++ {
		r := <-results
		allResults[r.Index] = r
		if progressFn != nil {
			progressFn(i+1, len(chunks))
		}
		if r.Err != nil {
			if strings.Contains(r.Err.Error(), "API 오류 [30]") {
				continue // 데이터 없음 → 건너뜀
			}
			if strings.Contains(r.Err.Error(), "429") {
				lastErr = r.Err // 재투입 한도 소진된 429 → 경고 후 계속
				continue
			}
			fatalErr = fmt.Errorf("분할 조회 [%d] 실패: %w", r.Index, r.Err)
			break
		}
	}

	close(done) // 워커 종료 신호 (jobs는 닫지 않음 — 재투입 send 패닉 방지)
	wg.Wait()

	if fatalErr != nil {
		return nil, fatalErr
	}
	if lastErr != nil {
		fmt.Fprintf(os.Stderr, "경고: 일부 청크가 429 재시도 한도를 초과했습니다: %v\n", lastErr)
	}

	// 순서대로 병합 → 정렬 → 중복 제거 (기존 동작 유지)
	var merged []DataRow
	for _, r := range allResults {
		merged = append(merged, r.Data...)
	}
	sortByPeriod(merged)
	merged = deduplicateRows(merged)
	return merged, nil
}

// countFilterValues는 필터 값에서 실제 선택된 개수를 추정합니다.
// "ALL" → metaCount, "" → 1, "00+11+21" → 3, "11*" → metaCount/4 (추정)
func countFilterValues(filterVal string, metaCount int) int {
	if filterVal == "" {
		return 1
	}
	upper := strings.ToUpper(filterVal)
	if upper == "ALL" {
		return metaCount
	}
	if strings.HasSuffix(filterVal, "*") {
		// 하위 전체 선택: 전체의 1/2로 추정 (과소추정 방지를 위해 보수적으로 계산)
		est := metaCount / 2
		if est < 1 {
			est = 1
		}
		return est
	}
	// "00+11+21" → + 로 구분된 개수
	return len(strings.Split(filterVal, "+"))
}

// estimateCellCountFromSummary는 이미 조회된 MetaSummaryResult를 사용하여 예상 셀 수를 계산합니다.
// MetaSummary를 중복 호출하지 않도록 리팩토링된 함수입니다.
func estimateCellCountFromSummary(summary *MetaSummaryResult, opts DataOptions) int {
	// 분류 그룹별 항목 수 계산
	objGroups := map[string]int{}
	objOrder := []string{}
	for _, cl := range summary.Classifications {
		if cl.ObjID != "" {
			if _, exists := objGroups[cl.ObjID]; !exists {
				objOrder = append(objOrder, cl.ObjID)
			}
			objGroups[cl.ObjID]++
		}
	}

	// 각 분류 그룹의 실제 선택 개수 계산
	classFilters := []string{opts.Class1, opts.Class2, opts.Class3, opts.Class4, opts.Class5, opts.Class6, opts.Class7, opts.Class8}
	classCount := 1
	for i, objID := range objOrder {
		metaCount := objGroups[objID]
		filterVal := ""
		if i < len(classFilters) {
			filterVal = classFilters[i]
		}
		classCount *= countFilterValues(filterVal, metaCount)
	}
	if classCount == 0 {
		classCount = 1
	}

	// 항목 개수: 실제 필터 반영
	itemCount := countFilterValues(opts.Item, len(summary.Items))
	if itemCount == 0 {
		itemCount = 1
	}

	// 시점 개수 계산
	periodCount := 1
	// --latest가 있으면 그 값을 사용
	if opts.NewEstPrdCnt != "" {
		if n, err := strconv.Atoi(opts.NewEstPrdCnt); err == nil && n > 0 {
			periodCount = n
		}
	} else if opts.StartPrdDe != "" && opts.EndPrdDe != "" {
		// start~end 범위 (문자열이 4자 미만이면 연도 파싱 불가하므로 기본값 유지)
		if len(opts.StartPrdDe) >= 4 && len(opts.EndPrdDe) >= 4 {
			startY, _ := strconv.Atoi(opts.StartPrdDe[:4])
			endY, _ := strconv.Atoi(opts.EndPrdDe[:4])
			if endY >= startY {
				periodCount = endY - startY + 1
				prd := strings.ToUpper(opts.PrdSe)
				if prd == "M" {
					periodCount *= 12
				}
				if prd == "Q" {
					periodCount *= 4
				}
				if prd == "H" {
					periodCount *= 2
				}
			}
		}
	} else {
		// 시점 정보 없으면 메타에서 추정
		for _, p := range summary.Periods {
			prdCode := strings.ToUpper(strings.TrimSpace(p.PrdSe))
			optsPrd := strings.ToUpper(opts.PrdSe)
			prdMatch := (optsPrd == "" || prdCode == optsPrd ||
				(optsPrd == "Y" && (prdCode == "년" || prdCode == "Y")) ||
				(optsPrd == "M" && (prdCode == "월" || prdCode == "M")) ||
				(optsPrd == "Q" && (prdCode == "분기" || prdCode == "Q")) ||
				(optsPrd == "H" && (prdCode == "반기" || prdCode == "H")))
			if prdMatch && p.StrtPrdDe != "" && p.EndPrdDe != "" {
				start := strings.ReplaceAll(strings.ReplaceAll(p.StrtPrdDe, ".", ""), " ", "")
				end := strings.ReplaceAll(strings.ReplaceAll(p.EndPrdDe, ".", ""), " ", "")
				start = strings.Split(start, "/")[0]
				end = strings.Split(end, "/")[0]
				// 문자열이 4자 미만이면 연도 파싱 불가하므로 건너뜀
				if len(start) < 4 || len(end) < 4 {
					break
				}
				startY, _ := strconv.Atoi(start[:4])
				endY, _ := strconv.Atoi(end[:4])
				if endY > startY {
					periodCount = endY - startY + 1
					if prdCode == "월" || prdCode == "M" {
						periodCount *= 12
					}
					if prdCode == "분기" || prdCode == "Q" {
						periodCount *= 4
					}
					if prdCode == "반기" || prdCode == "H" {
						periodCount *= 2
					}
				}
				break
			}
		}
	}

	// 예상 행 수 = 분류 × 항목 × 시점, 셀 수 = 행 × 컬럼(~14)
	estimatedCells := classCount * itemCount * periodCount * 14

	return estimatedCells
}

// subdivideByClass 분류 축 2차 분할(설계서 Step 7b).
// 단일 시점이 maxCells를 초과할 때, 필터가 ALL이고 카디널리티가 가장 큰 분류 축의 코드를
// 그룹으로 나눠 각 그룹별 DataOptions를 생성합니다. 분할 불가 시 (nil, false).
func (c *Client) subdivideByClass(summary *MetaSummaryResult, opts DataOptions, maxCells, cellsPerPeriod int) ([]DataOptions, bool) {
	if summary == nil || cellsPerPeriod <= maxCells {
		return nil, false
	}

	// 분류 축별 코드 목록 + 등장 순서
	objOrder := []string{}
	codesByObj := map[string][]string{}
	for _, cl := range summary.Classifications {
		if cl.ObjID == "" {
			continue
		}
		if _, ok := codesByObj[cl.ObjID]; !ok {
			objOrder = append(objOrder, cl.ObjID)
		}
		if cl.ItmID != "" {
			codesByObj[cl.ObjID] = append(codesByObj[cl.ObjID], cl.ItmID)
		}
	}
	if len(objOrder) == 0 {
		return nil, false
	}

	classFilters := []string{opts.Class1, opts.Class2, opts.Class3, opts.Class4, opts.Class5, opts.Class6, opts.Class7, opts.Class8}

	// 분할 대상: 필터가 ALL이고 코드 수가 가장 많은 축 (enumerate 가능한 축만)
	splitIdx, splitCount := -1, 0
	for i, objID := range objOrder {
		filter := ""
		if i < len(classFilters) {
			filter = classFilters[i]
		}
		if strings.ToUpper(filter) != "ALL" {
			continue
		}
		if n := len(codesByObj[objID]); n > splitCount {
			splitCount, splitIdx = n, i
		}
	}
	if splitIdx < 0 || splitCount <= 1 {
		return nil, false
	}

	// 그룹 크기: 실제 단일 시점 셀(cellsPerPeriod)을 기준으로,
	// 분할 후 (groupSize/splitCount) 비율로 줄어들어 maxCells 이하가 되도록 계산.
	// groupSize = floor(splitCount × maxCells / cellsPerPeriod). 여유분(0.9)으로 보수적 계산.
	groupSize := splitCount * maxCells * 9 / (cellsPerPeriod * 10)
	if groupSize < 1 {
		groupSize = 1
	}
	if groupSize >= splitCount {
		return nil, false // 이 축만으로는 분할 효과 없음
	}

	codes := codesByObj[objOrder[splitIdx]]
	var subOpts []DataOptions
	for i := 0; i < len(codes); i += groupSize {
		end := i + groupSize
		if end > len(codes) {
			end = len(codes)
		}
		so := opts
		setClassByIndex(&so, splitIdx, strings.Join(codes[i:end], "+"))
		subOpts = append(subOpts, so)
	}
	if len(subOpts) <= 1 {
		return nil, false
	}
	return subOpts, true
}

// setClassByIndex 0-based 분류 축 인덱스에 해당하는 ClassN 필드를 설정합니다.
func setClassByIndex(opts *DataOptions, idx int, val string) {
	switch idx {
	case 0:
		opts.Class1 = val
	case 1:
		opts.Class2 = val
	case 2:
		opts.Class3 = val
	case 3:
		opts.Class4 = val
	case 4:
		opts.Class5 = val
	case 5:
		opts.Class6 = val
	case 6:
		opts.Class7 = val
	case 7:
		opts.Class8 = val
	}
}

// dataBySubOptions 분류 축 2차 분할로 생성된 각 그룹을 조회(각 그룹은 다시 시점 분할+병렬)하고 병합합니다.
func (c *Client) dataBySubOptions(orgID, tblID string, subOpts []DataOptions, splitOpts SplitOptions, progressFn func(current, total int)) ([]DataRow, error) {
	var all []DataRow
	total := len(subOpts)
	for i, so := range subOpts {
		// 각 그룹은 분류 축이 좁혀져 단일 시점이 maxCells 이하 → 일반 시점 분할 경로를 재사용
		part, err := c.DataWithAutoSplit(orgID, tblID, so, splitOpts, nil)
		if err != nil {
			return nil, fmt.Errorf("분류 그룹 [%d/%d] 조회 실패: %w", i+1, total, err)
		}
		all = append(all, part...)
		if progressFn != nil {
			progressFn(i+1, total)
		}
	}
	sortByPeriod(all)
	all = deduplicateRows(all)
	return all, nil
}

// countPeriods 시작~종료 시점 사이의 수록주기 개수를 계산합니다 (Step 7a: 단일 시점 셀 추정용).
// 계산 불가 시 0을 반환합니다.
func countPeriods(opts DataOptions) int {
	start := opts.StartPrdDe
	end := opts.EndPrdDe
	if start == "" || end == "" {
		return 0
	}
	period := strings.ToUpper(opts.PrdSe)
	if period == "" {
		period = "Y"
	}
	atoi := func(s string) (int, bool) { n, err := strconv.Atoi(s); return n, err == nil }
	switch period {
	case "Y":
		s, ok1 := atoi(start)
		e, ok2 := atoi(end)
		if !ok1 || !ok2 || e < s {
			return 0
		}
		return e - s + 1
	case "M":
		if len(start) != 6 || len(end) != 6 {
			return 0
		}
		sy, o1 := atoi(start[:4])
		sm, o2 := atoi(start[4:])
		ey, o3 := atoi(end[:4])
		em, o4 := atoi(end[4:])
		if !o1 || !o2 || !o3 || !o4 {
			return 0
		}
		n := (ey-sy)*12 + (em - sm) + 1
		if n < 1 {
			return 0
		}
		return n
	case "Q", "H":
		if len(start) != 5 || len(end) != 5 {
			return 0
		}
		per := 4
		if period == "H" {
			per = 2
		}
		sy, o1 := atoi(start[:4])
		sp, o2 := atoi(start[4:])
		ey, o3 := atoi(end[:4])
		ep, o4 := atoi(end[4:])
		if !o1 || !o2 || !o3 || !o4 {
			return 0
		}
		n := (ey-sy)*per + (ep - sp) + 1
		if n < 1 {
			return 0
		}
		return n
	}
	return 0
}

// splitByPeriod 시점 축으로 분할
// startPrdDe와 endPrdDe를 기반으로 청크를 생성합니다.
func (c *Client) splitByPeriod(opts DataOptions, totalCells, maxCells int) []PeriodChunk {
	// 시점 정보가 없으면 분할 불가
	if opts.StartPrdDe == "" && opts.EndPrdDe == "" && opts.NewEstPrdCnt == "" {
		return nil
	}

	// 현재는 startPrdDe와 endPrdDe를 기반으로 분할
	if opts.StartPrdDe == "" || opts.EndPrdDe == "" {
		return nil
	}

	start := opts.StartPrdDe
	end := opts.EndPrdDe

	// 수록주기 파악
	period := opts.PrdSe
	if period == "" {
		period = "Y" // 기본값: 연
	}

	// 필요한 청크 개수 계산
	chunksNeeded := (totalCells + maxCells - 1) / maxCells
	if chunksNeeded <= 1 {
		return nil
	}

	// 연도 분할 (월별은 나중에 확장 가능)
	if period == "Y" || period == "y" {
		return c.splitYearRange(start, end, chunksNeeded)
	}

	// 월별 분할
	if period == "M" || period == "m" {
		return c.splitMonthRange(start, end, chunksNeeded)
	}

	// 분기별 분할
	if period == "Q" || period == "q" {
		return c.splitQuarterRange(start, end, chunksNeeded)
	}

	// 반기별 분할
	if period == "H" || period == "h" {
		return c.splitHalfRange(start, end, chunksNeeded)
	}

	return nil
}

// splitYearRange 연도 범위를 청크로 분할
// 예: "2015" ~ "2024" (10년)를 2개 청크로 분할하면 "2015" ~ "2019", "2020" ~ "2024"
func (c *Client) splitYearRange(start, end string, chunksNeeded int) []PeriodChunk {
	startYear, errStart := strconv.Atoi(start)
	endYear, errEnd := strconv.Atoi(end)

	if errStart != nil || errEnd != nil {
		return nil
	}

	totalYears := endYear - startYear + 1
	yearsPerChunk := (totalYears + chunksNeeded - 1) / chunksNeeded

	var chunks []PeriodChunk
	for i := 0; i < chunksNeeded; i++ {
		chunkStart := startYear + (i * yearsPerChunk)
		chunkEnd := chunkStart + yearsPerChunk - 1

		// 마지막 청크는 종료 연도를 endYear로 맞춤
		if i == chunksNeeded-1 {
			chunkEnd = endYear
		}

		if chunkStart <= endYear {
			chunks = append(chunks, PeriodChunk{
				Start: fmt.Sprintf("%d", chunkStart),
				End:   fmt.Sprintf("%d", chunkEnd),
			})
		}
	}

	return chunks
}

// splitMonthRange 월도 범위를 청크로 분할
// 예: "202001" ~ "202412" (24개월)를 2개 청크로 분할하면 "202001" ~ "202012", "202101" ~ "202412"
func (c *Client) splitMonthRange(start, end string, chunksNeeded int) []PeriodChunk {
	// start, end 형식: "YYYYMM"
	if len(start) != 6 || len(end) != 6 {
		return nil
	}

	startYear, _ := strconv.Atoi(start[:4])
	startMonth, _ := strconv.Atoi(start[4:6])
	endYear, _ := strconv.Atoi(end[:4])
	endMonth, _ := strconv.Atoi(end[4:6])

	// 월을 절대값으로 변환
	startTotal := startYear*12 + startMonth
	endTotal := endYear*12 + endMonth

	totalMonths := endTotal - startTotal + 1
	monthsPerChunk := (totalMonths + chunksNeeded - 1) / chunksNeeded

	var chunks []PeriodChunk
	for i := 0; i < chunksNeeded; i++ {
		chunkStartTotal := startTotal + (i * monthsPerChunk)
		chunkEndTotal := chunkStartTotal + monthsPerChunk - 1

		// 마지막 청크는 종료월을 endTotal로 맞춤
		if i == chunksNeeded-1 {
			chunkEndTotal = endTotal
		}

		if chunkStartTotal <= endTotal {
			chunkStartYear := chunkStartTotal / 12
			chunkStartMonth := (chunkStartTotal % 12)
			if chunkStartMonth == 0 {
				chunkStartMonth = 12
				chunkStartYear--
			}

			chunkEndYear := chunkEndTotal / 12
			chunkEndMonth := (chunkEndTotal % 12)
			if chunkEndMonth == 0 {
				chunkEndMonth = 12
				chunkEndYear--
			}

			chunks = append(chunks, PeriodChunk{
				Start: fmt.Sprintf("%04d%02d", chunkStartYear, chunkStartMonth),
				End:   fmt.Sprintf("%04d%02d", chunkEndYear, chunkEndMonth),
			})
		}
	}

	return chunks
}

// splitQuarterRange 분기 범위를 청크로 분할
// 예: "20151" ~ "20244" (10년 4분기)를 2개 청크로 분할
func (c *Client) splitQuarterRange(start, end string, chunksNeeded int) []PeriodChunk {
	// start, end 형식: "YYYYQ" (Q=1~4)
	if len(start) != 5 || len(end) != 5 {
		return nil
	}

	startYear, _ := strconv.Atoi(start[:4])
	startQuarter, _ := strconv.Atoi(start[4:5])
	endYear, _ := strconv.Atoi(end[:4])
	endQuarter, _ := strconv.Atoi(end[4:5])

	// 분기를 절대값으로 변환
	startTotal := startYear*4 + startQuarter
	endTotal := endYear*4 + endQuarter

	totalQuarters := endTotal - startTotal + 1
	quartersPerChunk := (totalQuarters + chunksNeeded - 1) / chunksNeeded

	var chunks []PeriodChunk
	for i := 0; i < chunksNeeded; i++ {
		chunkStartTotal := startTotal + (i * quartersPerChunk)
		chunkEndTotal := chunkStartTotal + quartersPerChunk - 1

		// 마지막 청크는 종료분기를 endTotal로 맞춤
		if i == chunksNeeded-1 {
			chunkEndTotal = endTotal
		}

		if chunkStartTotal <= endTotal {
			chunkStartYear := chunkStartTotal / 4
			chunkStartQuarter := (chunkStartTotal % 4)
			if chunkStartQuarter == 0 {
				chunkStartQuarter = 4
				chunkStartYear--
			}

			chunkEndYear := chunkEndTotal / 4
			chunkEndQuarter := (chunkEndTotal % 4)
			if chunkEndQuarter == 0 {
				chunkEndQuarter = 4
				chunkEndYear--
			}

			chunks = append(chunks, PeriodChunk{
				Start: fmt.Sprintf("%04d%d", chunkStartYear, chunkStartQuarter),
				End:   fmt.Sprintf("%04d%d", chunkEndYear, chunkEndQuarter),
			})
		}
	}

	return chunks
}

// splitHalfRange 반기 범위를 청크로 분할
// 예: "20151" ~ "20242" (10년 반기)를 2개 청크로 분할
func (c *Client) splitHalfRange(start, end string, chunksNeeded int) []PeriodChunk {
	// start, end 형식: "YYYYH" (H=1~2)
	if len(start) != 5 || len(end) != 5 {
		return nil
	}

	startYear, _ := strconv.Atoi(start[:4])
	startHalf, _ := strconv.Atoi(start[4:5])
	endYear, _ := strconv.Atoi(end[:4])
	endHalf, _ := strconv.Atoi(end[4:5])

	// 반기를 절대값으로 변환
	startTotal := startYear*2 + startHalf
	endTotal := endYear*2 + endHalf

	totalHalves := endTotal - startTotal + 1
	halvesPerChunk := (totalHalves + chunksNeeded - 1) / chunksNeeded

	var chunks []PeriodChunk
	for i := 0; i < chunksNeeded; i++ {
		chunkStartTotal := startTotal + (i * halvesPerChunk)
		chunkEndTotal := chunkStartTotal + halvesPerChunk - 1

		// 마지막 청크는 종료반기를 endTotal로 맞춤
		if i == chunksNeeded-1 {
			chunkEndTotal = endTotal
		}

		if chunkStartTotal <= endTotal {
			chunkStartYear := chunkStartTotal / 2
			chunkStartHalf := (chunkStartTotal % 2)
			if chunkStartHalf == 0 {
				chunkStartHalf = 2
				chunkStartYear--
			}

			chunkEndYear := chunkEndTotal / 2
			chunkEndHalf := (chunkEndTotal % 2)
			if chunkEndHalf == 0 {
				chunkEndHalf = 2
				chunkEndYear--
			}

			chunks = append(chunks, PeriodChunk{
				Start: fmt.Sprintf("%04d%d", chunkStartYear, chunkStartHalf),
				End:   fmt.Sprintf("%04d%d", chunkEndYear, chunkEndHalf),
			})
		}
	}

	return chunks
}

// calcLatestStart는 종료 시점에서 N개 시점 전의 시작 시점을 계산합니다.
// --latest N 사용 시 전체 범위 대신 최근 N개 시점만 조회하기 위해 사용합니다.
func (c *Client) calcLatestStart(end string, n int, prdSe string) string {
	period := strings.ToUpper(prdSe)
	if period == "" {
		period = "Y"
	}

	switch period {
	case "Y":
		endYear, err := strconv.Atoi(end)
		if err != nil {
			return end
		}
		return fmt.Sprintf("%d", endYear-n+1)
	case "M":
		if len(end) < 6 {
			return end
		}
		endYear, _ := strconv.Atoi(end[:4])
		endMonth, _ := strconv.Atoi(end[4:6])
		total := endYear*12 + endMonth - n + 1
		y := total / 12
		m := total % 12
		if m == 0 {
			m = 12
			y--
		}
		return fmt.Sprintf("%04d%02d", y, m)
	case "Q":
		if len(end) < 5 {
			return end
		}
		endYear, _ := strconv.Atoi(end[:4])
		endQ, _ := strconv.Atoi(end[4:5])
		total := endYear*4 + endQ - n + 1
		y := total / 4
		q := total % 4
		if q == 0 {
			q = 4
			y--
		}
		return fmt.Sprintf("%04d%d", y, q)
	case "H":
		if len(end) < 5 {
			return end
		}
		endYear, _ := strconv.Atoi(end[:4])
		endH, _ := strconv.Atoi(end[4:5])
		total := endYear*2 + endH - n + 1
		y := total / 2
		h := total % 2
		if h == 0 {
			h = 2
			y--
		}
		return fmt.Sprintf("%04d%d", y, h)
	}
	return end
}

// sortByPeriod PRD_DE(시점) 기준으로 오름차순 정렬
func sortByPeriod(rows []DataRow) {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].PrdDe < rows[j].PrdDe
	})
}

// deduplicateRows 중복 행 제거 (C1~C8 + ITM_ID + PRD_DE 조합 기준)
func deduplicateRows(rows []DataRow) []DataRow {
	seen := make(map[string]bool, len(rows))
	result := make([]DataRow, 0, len(rows))
	for _, row := range rows {
		key := row.C1 + "|" + row.C2 + "|" + row.C3 + "|" + row.C4 + "|" +
			row.C5 + "|" + row.C6 + "|" + row.C7 + "|" + row.C8 + "|" +
			row.ItmID + "|" + row.PrdDe
		if !seen[key] {
			seen[key] = true
			result = append(result, row)
		}
	}
	return result
}
