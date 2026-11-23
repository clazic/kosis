package cmd

import (
	"strings"
	"testing"
)

// 조회 전에 막는 인자 (API 를 부르지 않는다).
func TestBatchArgErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"101"}, "ORG TBL 로 시작"},
		{[]string{"101 DT_X -c1 00 -f md"}, "-f 는 batch 에서 쓰지 않습니다"},
		{[]string{"101 DT_X --pivot C1"}, "--pivot 는 batch"},
		{strings.Fields(strings.Repeat("a ", 11)), "10개까지"},
	} {
		err := runBatch(batchCmd, c.args)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v", c.args, err)
		}
	}
}
