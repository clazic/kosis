package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/clazic/kosis/internal/config"
)

// metaKeysField tools/call params._meta 안의 요청별 키 이름.
const metaKeysField = "kr.pes/keys"

const (
	maxRequestKeys = 50
	maxKeyLen      = 256
)

// loadDefaultKeys 기본 키: KOSIS_API_KEYS(쉼표) → 없으면 CLI 설정(~/.kosis/config.yaml, KOSIS_API_KEY).
func loadDefaultKeys() []string {
	if keys := splitKeys(os.Getenv("KOSIS_API_KEYS")); len(keys) > 0 {
		return keys
	}
	if cfg, err := config.Load(); err == nil {
		return splitKeys(strings.Join(cfg.APIKeys, ","))
	}
	return nil
}

func splitKeys(s string) []string {
	var out []string
	for _, k := range strings.Split(s, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// errNoKey 쓸 키가 없을 때.
var errNoKey = fmt.Errorf("API 키가 없어요 — KOSIS_API_KEYS 환경변수(쉼표로 여러 개), ~/.kosis/config.yaml, 또는 tools/call 의 _meta[\"%s\"] 로 KOSIS 키를 주세요 (발급: https://kosis.kr/openapi/)", metaKeysField)

// selectKeys 이 요청에 쓸 키. _meta 에 요청별 키가 있으면 그것만 (형식이 틀려도 기본 키로 넘어가지 않고 오류),
// 없으면 기본 키. perRequest 는 요청별 키를 썼는지.
func selectKeys(meta map[string]json.RawMessage, defaults []string) (keys []string, perRequest bool, err error) {
	raw, ok := meta[metaKeysField]
	if !ok {
		if len(defaults) == 0 {
			return nil, false, errNoKey
		}
		return defaults, false, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, true, fmt.Errorf("_meta[\"%s\"] 는 문자열 배열이어야 합니다", metaKeysField)
	}
	if len(list) == 0 {
		return nil, true, fmt.Errorf("_meta[\"%s\"] 가 비어 있습니다 — 요청별 키를 줄 때는 키가 하나 이상 있어야 합니다 (기본 키로 넘어가지 않습니다)", metaKeysField)
	}
	if len(list) > maxRequestKeys {
		return nil, true, fmt.Errorf("_meta[\"%s\"] 키는 %d개까지입니다 (받은 %d개)", metaKeysField, maxRequestKeys, len(list))
	}
	seen := map[string]bool{}
	for i, k := range list {
		k = strings.TrimSpace(k)
		if k == "" || len(k) > maxKeyLen || strings.ContainsAny(k, " ,\t\r\n") || hasControl(k) {
			return nil, true, fmt.Errorf("_meta[\"%s\"] 의 %d번째 키 형식이 맞지 않습니다 (빈 값·공백·쉼표·%d자 초과 불가)", metaKeysField, i+1, maxKeyLen)
		}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys, true, nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// maskKey 키를 ***<끝4자리> 로. 8자 미만 키는 끝자리도 보이지 않는다.
func maskKey(k string) string {
	if len(k) < 8 {
		return "***"
	}
	return "***" + k[len(k)-4:]
}

// apiKeyParam URL 에 남은 apiKey=값 (어떤 키든).
var apiKeyParam = regexp.MustCompile(`(?i)(apiKey=)([^&"'\s<>]+)`)

// scrubKeys 출력 직전 텍스트에서 키 원문(그대로·URL 인코딩)을 ***<끝4자리> 로 바꾸고, 남은 apiKey=값도 가린다.
func scrubKeys(text string, keys []string) string {
	ks := append([]string(nil), keys...)
	sort.Slice(ks, func(i, j int) bool { return len(ks[i]) > len(ks[j]) }) // 긴 키부터 (한 키가 다른 키를 품는 경우)
	for _, k := range ks {
		if k == "" {
			continue
		}
		m := maskKey(k)
		for _, v := range []string{k, url.QueryEscape(k), url.PathEscape(k)} {
			if v != "" {
				text = strings.ReplaceAll(text, v, m)
			}
		}
	}
	return apiKeyParam.ReplaceAllStringFunc(text, func(s string) string {
		sub := apiKeyParam.FindStringSubmatch(s)
		if strings.HasPrefix(sub[2], "***") {
			return s
		}
		return sub[1] + "***"
	})
}
