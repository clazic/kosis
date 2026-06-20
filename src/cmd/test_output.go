package cmd

import (
	"bytes"
	"fmt"
	"github.com/clazic/kosis/internal/output"
)

func ExampleOutput() {
	// 테스트 데이터
	data := []map[string]interface{}{
		{
			"ORG_ID": "101",
			"ORG_NM": "통계청",
			"TBL_ID": "DT_1IN1502",
			"TBL_NM": "인구(읍면동/5세별)",
			"C1_NM":  "서울",
			"ITM_NM": "총인구",
			"PRD_DE": "2024",
			"DT":     "9728000",
		},
		{
			"ORG_ID": "116",
			"ORG_NM": "국토교통부",
			"TBL_ID": "DT_MLTM_2086",
			"TBL_NM": "주택 미분양",
			"C1_NM":  "서울",
			"ITM_NM": "미분양",
			"PRD_DE": "2024-03",
			"DT":     "52400",
		},
	}

	fmt.Println("=== 테이블 포맷 (한글) ===")
	buf := &bytes.Buffer{}
	tf, err := output.NewFormatter("table")
	if err != nil {
		fmt.Println("에러:", err)
		return
	}
	tf.Format(data, output.FormatOptions{
		Writer: buf,
		Korean: true,
	})
	fmt.Print(buf.String())

	fmt.Println("\n=== JSON 포맷 (compact) ===")
	buf = &bytes.Buffer{}
	jf, err := output.NewFormatter("json")
	if err != nil {
		fmt.Println("에러:", err)
		return
	}
	jf.Format(data, output.FormatOptions{
		Writer: buf,
		Korean: true,
	})
	fmt.Print(buf.String())

	fmt.Println("\n=== CSV 포맷 (선택된 컬럼만) ===")
	buf = &bytes.Buffer{}
	cf, err := output.NewFormatter("csv")
	if err != nil {
		fmt.Println("에러:", err)
		return
	}
	cf.Format(data, output.FormatOptions{
		Writer:  buf,
		Korean:  true,
		Columns: []string{"ORG_NM", "TBL_NM", "C1_NM", "DT"},
	})
	// UTF-8 BOM 제외하고 출력
	fmt.Print(buf.String()[3:])
}
