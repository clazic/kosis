// Package api provides KOSIS Open API client and data structures.
package api

import "fmt"

// SearchResult represents a search result from statisticsSearch API.
type SearchResult struct {
	OrgID     string `json:"ORG_ID"`
	TblID     string `json:"TBL_ID"`
	TblNM     string `json:"TBL_NM"`
	OrgNM     string `json:"ORG_NM"`
	StatID    string `json:"STAT_ID"`
	StatNM    string `json:"STAT_NM"`
	VwCD      string `json:"VW_CD"`
	MTAtitle  string `json:"MT_ATITLE"`
	StrtPrdDe string `json:"STRT_PRD_DE"`
	EndPrdDe  string `json:"END_PRD_DE"`
	RecTblSe  string `json:"REC_TBL_SE"`
}

// MetaResult represents metadata result from statisticsData.getMeta API.
type MetaResult struct {
	ObjID   string `json:"OBJ_ID"`
	ObjNM   string `json:"OBJ_NM"`
	ObjIDSN string `json:"OBJ_ID_SN"`
	ItmID   string `json:"ITM_ID"`
	ItmNM   string `json:"ITM_NM"`
	UpItmID string `json:"UP_ITM_ID"`
	Level   int    `json:"LEVEL"`
	Type    string `json:"TYPE"`
	Code    string `json:"CODE"`
	Label   string `json:"LABEL"`
	PrdSe     string `json:"PRD_SE"`
	PrdDe     string `json:"PRD_DE"`
	StrtPrdDe string `json:"STRT_PRD_DE"`
	EndPrdDe  string `json:"END_PRD_DE"`
	UnitNM    string `json:"UNIT_NM"`
}

// MetaSummaryResult groups classification, item, and period metadata.
type MetaSummaryResult struct {
	Classifications []MetaResult `json:"classifications"`
	Items           []MetaResult `json:"items"`
	Periods         []MetaResult `json:"periods"`
}

// ColumnDef는 데이터 컬럼 하나의 정의입니다.
type ColumnDef struct {
	Key      string // 필드 키 (예: "C1_NM", "ITM_NM", "PRD_DE", "DT")
	Label    string // 표시 이름 (예: "시도별", "항목", "시점", "수치값")
	HasValue bool   // 데이터에 값이 있는 컬럼인지
}

// ColumnMeta는 통계표의 컬럼 메타정보입니다.
// 메타 조회 결과에서 생성되어 데이터 표시에 사용됩니다.
type ColumnMeta struct {
	Columns []ColumnDef
}

// BuildColumnMeta는 MetaSummaryResult에서 컬럼 메타정보를 생성합니다.
func (s *MetaSummaryResult) BuildColumnMeta() *ColumnMeta {
	cm := &ColumnMeta{}

	// 분류 그룹: ObjID 등장 순서대로 C1~C8 매핑
	seen := map[string]bool{}
	classIdx := 1
	for _, c := range s.Classifications {
		if c.ObjID != "" && !seen[c.ObjID] {
			seen[c.ObjID] = true
			label := c.ObjNM
			if label == "" {
				label = fmt.Sprintf("분류%d", classIdx)
			}
			cm.Columns = append(cm.Columns, ColumnDef{
				Key:   fmt.Sprintf("C%d_NM", classIdx),
				Label: label,
			})
			classIdx++
			if classIdx > 8 {
				break
			}
		}
	}

	// 항목
	if len(s.Items) > 0 {
		label := "항목"
		if s.Items[0].ObjNM != "" {
			label = s.Items[0].ObjNM
		}
		cm.Columns = append(cm.Columns, ColumnDef{Key: "ITM_NM", Label: label})
	}

	// 고정 컬럼 (빠짐없이 모두 포함)
	cm.Columns = append(cm.Columns,
		ColumnDef{Key: "PRD_SE", Label: "수록주기"},
		ColumnDef{Key: "PRD_DE", Label: "시점"},
		ColumnDef{Key: "DT", Label: "수치값"},
		ColumnDef{Key: "UNIT_NM", Label: "단위"},
		ColumnDef{Key: "LST_CHN_DE", Label: "비고"},
	)

	return cm
}

// FilterByData는 실제 데이터에 값이 있는 컬럼만 필터링합니다.
func (cm *ColumnMeta) FilterByData(rows []DataRow) *ColumnMeta {
	filtered := &ColumnMeta{}
	for _, col := range cm.Columns {
		hasValue := false
		for _, row := range rows {
			if v := row.GetField(col.Key); v != "" {
				hasValue = true
				break
			}
		}
		if hasValue {
			col.HasValue = true
			filtered.Columns = append(filtered.Columns, col)
		}
	}
	return filtered
}

// GetLabel는 키에 해당하는 라벨을 반환합니다.
func (cm *ColumnMeta) GetLabel(key string) string {
	for _, col := range cm.Columns {
		if col.Key == key {
			return col.Label
		}
	}
	return key
}

// DataRow represents a single data row from statisticsParameterData API.
type DataRow struct {
	OrgID  string `json:"ORG_ID"`
	TblID  string `json:"TBL_ID"`
	TblNM  string `json:"TBL_NM"`
	C1     string `json:"C1"`
	C1NM   string `json:"C1_NM"`
	C2     string `json:"C2"`
	C2NM   string `json:"C2_NM"`
	C3     string `json:"C3"`
	C3NM   string `json:"C3_NM"`
	C4     string `json:"C4"`
	C4NM   string `json:"C4_NM"`
	C5     string `json:"C5"`
	C5NM   string `json:"C5_NM"`
	C6     string `json:"C6"`
	C6NM   string `json:"C6_NM"`
	C7     string `json:"C7"`
	C7NM   string `json:"C7_NM"`
	C8     string `json:"C8"`
	C8NM   string `json:"C8_NM"`
	ItmID  string `json:"ITM_ID"`
	ItmNM  string `json:"ITM_NM"`
	UnitID string `json:"UNIT_ID"`
	UnitNM string `json:"UNIT_NM"`
	PrdSe  string `json:"PRD_SE"`
	PrdDe  string `json:"PRD_DE"`
	DT     string `json:"DT"`
	LstChn string `json:"LST_CHN_DE"`
}

// GetField는 키 이름으로 DataRow의 필드 값을 반환합니다.
func (r DataRow) GetField(key string) string {
	switch key {
	case "C1_NM":
		return r.C1NM
	case "C2_NM":
		return r.C2NM
	case "C3_NM":
		return r.C3NM
	case "C4_NM":
		return r.C4NM
	case "C5_NM":
		return r.C5NM
	case "C6_NM":
		return r.C6NM
	case "C7_NM":
		return r.C7NM
	case "C8_NM":
		return r.C8NM
	case "ITM_NM":
		return r.ItmNM
	case "PRD_DE":
		return r.PrdDe
	case "DT":
		return r.DT
	case "UNIT_NM":
		return r.UnitNM
	case "PRD_SE":
		return r.PrdSe
	case "ORG_ID":
		return r.OrgID
	case "TBL_ID":
		return r.TblID
	case "TBL_NM":
		return r.TblNM
	case "LST_CHN_DE":
		return r.LstChn
	default:
		return ""
	}
}

// StatList represents an item from statisticsList.getList API.
type StatList struct {
	VwCD     string `json:"VW_CD"`
	VwNM     string `json:"VW_NM"`
	ListID   string `json:"LIST_ID"`
	ListNM   string `json:"LIST_NM"`
	OrgID    string `json:"ORG_ID"`
	TblID    string `json:"TBL_ID"`
	TblNM    string `json:"TBL_NM"`
	StatID   string `json:"STAT_ID"`
	SendDe   string `json:"SEND_DE"`
	RecTblSe string `json:"REC_TBL_SE"`
}

// ExplainResult represents explanation data from statisticsExplData API.
type ExplainResult struct {
	OrgID        string `json:"orgId"`
	StatsNM      string `json:"statsNm"`
	WritingPurps string `json:"writingPurps"`
	StatsPeriod  string `json:"statsPeriod"`
	DataCollect  string `json:"dataCollect"`
	SurveyScope  string `json:"surveyScope"`
	PublishDe    string `json:"publishDe"`
	OrgNM        string `json:"orgNm"`
}

// IndicatorResult represents indicator data from indicator APIs.
type IndicatorResult struct {
	StatJipyoID  string `json:"jipyoId"`
	StatJipyoNM  string `json:"jipyoNm"`
	JipyoExplan  string `json:"jipyoExplan"`
	JipyoExplan1 string `json:"jipyoExplan1"`
	DataValue    string `json:"dataValue"`
	DataDE       string `json:"dataDe"`
	UnitNM       string `json:"unitNm"`
	LstUpdate    string `json:"lstUpdate"`
	ListID       string `json:"listId"`
	ListNM       string `json:"listNm"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Err    string `json:"err"`
	ErrMsg string `json:"errMsg"`
}

// SearchOptions contains optional search parameters.
type SearchOptions struct {
	Sort        string
	StartCount  int
	ResultCount int
}

// MetaOptions contains optional metadata parameters.
type MetaOptions struct {
	Type string
}

// DataOptions contains optional data query parameters.
type DataOptions struct {
	Class1       string
	Class2       string
	Class3       string
	Class4       string
	Class5       string
	Class6       string
	Class7       string
	Class8       string
	Item         string
	PrdSe        string
	Periods      string
	StartPrdDe   string
	EndPrdDe     string
	NewEstPrdCnt string
	PrdInterval  string
	OutputFields string
}

// PaginationOptions contains pagination parameters.
type PaginationOptions struct {
	PageNo    int
	NumOfRows int
}

// ListOptions contains list filtering parameters.
type ListOptions struct {
	VwCd     string
	ParentID string
}

// BigDataOptions contains big data query parameters.
type BigDataOptions struct {
	Type string
}
