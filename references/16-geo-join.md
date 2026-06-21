# 16. 지역코드 매핑 & 지도 결합 (Choropleth)

> KOSIS 통계를 지도(SGIS 경계 등)에 칠하거나 DB와 join할 때의 표준 절차.
> **핵심: 기계 결합엔 '코드', 사람 표시엔 '이름'.** 한글 이름은 중복(예: '중구'가 여러 시도에 존재)이라 join 키로 못 쓴다. 유일하게 안정적인 키는 분류 '코드'(`C1`=행정구역 코드 등)다.

---

## 16.1 핵심 원칙

- SGIS 경계 GeoJSON의 `adm_cd`(시도 2자리 · 시군구 5자리)와 join하려면 KOSIS 분류 **코드**(`C1`)가 필요하다.
- KOSIS 행정구역 코드 체계 = 통계청 행정구역 코드 = **SGIS adm_cd와 동일**. 즉 코드만 확보하면 무가공 join이 된다.
- 한글 이름 매칭은 **최후수단**이며 중복 때문에 위험하다.

---

## 16.2 코드를 얻는 3가지 방법 (우선순위 순)

| 순위 | 방법 | 상태 | 비고 |
|------|------|------|------|
| 1 | `meta -f json`의 CLASSIFICATIONS 역매핑 | **현재 표준** | 어떤 CLI 버전에서도 동작. 16.3 |
| 2 | `-f json`(코드 키 포함) 또는 `--with-code` | **CLI 지원 후** | 이슈 [clazic/kosis#1](https://github.com/clazic/kosis/issues/1) 머지 시 1순위로 승격. 16.4 |
| 3 | SGIS 행정구역 코드 목록과 이름 매칭 | 최후수단 | 중복 구이름 위험 |

> ⚠ **현재 CLI 함정**: `-f json` / `--fields "C1"`을 줘도 분류 코드가 `null`로 나온다(이름만 노출). CLI가 코드 키를 출력에서 드롭하기 때문(이슈 #1). 그래서 **지금은 16.3 역매핑이 표준**이다. 이슈가 머지되면 16.4가 표준이 되고 16.3은 불필요해진다.

---

## 16.3 표준 레시피 (현재) — meta 역매핑, 실측 검증됨 (2026-06-21)

데이터는 한글 이름만 오지만, **데이터가 전국→시도→시군구 계층 순서로 정렬**되어 온다는 점을 이용해
"현재 시도 컨텍스트 + 구 이름"으로 코드를 역매핑한다. 이러면 중복 구이름('중구' 등)도 유니크하게 풀린다.

```bash
# 1) meta를 json으로 받아 이름→코드 매핑표 확보 (CLASSIFICATIONS: ITM_ID/ITM_NM/UP_ITM_ID)
kosis m 101 DT_1FS1101 -f json > meta.json
# 2) 데이터는 이름으로 받기
kosis d 101 DT_1FS1101 -c1 ALL -c2 C -i T04 -p Y -l 1 -f json > data.json
```

```python
import json
meta = json.load(open('meta.json'))[0]
data = json.load(open('data.json'))

# 메타 A축(행정구역): 시도명→2자리코드, (시도2자리, 구이름)→5자리코드
sido_name2cd, sigungu = {}, {}
for c in meta['CLASSIFICATIONS']:
    if c['OBJ_ID'] != 'A':         # A축이 행정구역(표마다 OBJ_ID 확인)
        continue
    code, nm = c['ITM_ID'], c['ITM_NM']
    if len(code) == 2:
        sido_name2cd[nm] = code
    elif len(code) == 5:
        sigungu[(code[:2], nm)] = code

def num(v):
    try: return int(v)
    except: return None            # 'X'=비공개, '-'=해당없음

rows, cur = [], None
for r in data:
    nm, v = r['시도별'], num(r['수치값'])   # 키 이름은 meta의 분류명에 따름
    if nm == '전국':
        cur = None; continue
    if nm in sido_name2cd:                   # 시도 행 → 컨텍스트 갱신
        cur = sido_name2cd[nm]
        if nm == '세종특별자치시':            # 세종=시도이자 단일 시군구
            rows.append({'code': cur, 'name': nm, 'value': v})
        continue
    code = sigungu.get((cur, nm))            # 시군구 행 → (시도,구이름)으로 코드
    if code:
        rows.append({'code': code, 'name': nm, 'value': v})

json.dump(rows, open('mapped.json','w'), ensure_ascii=False)
```

→ `mapped.json`의 `code`를 SGIS 경계 GeoJSON `properties.adm_cd`와 join → choropleth.

---

## 16.4 표준 레시피 (향후) — CLI 코드 출력, **이슈 #1 머지 후**

> ⏳ [clazic/kosis#1](https://github.com/clazic/kosis/issues/1)이 머지되면 아래가 1순위 표준이 되고, 16.3 역매핑은 불필요해진다. **머지 전에는 동작하지 않으니 16.3을 쓸 것.**

```bash
# 시군구별 제조업 출하액을 코드와 함께 (머지 후 동작)
kosis d 101 DT_1FS1101 -c1 ALL -c2 C -i T04 -p Y -l 1 -f json
# 또는
kosis d 101 DT_1FS1101 -c1 ALL -c2 C -i T04 -p Y -l 1 --with-code -o mfg.csv
```

각 레코드에 `C1`(예: 11010), `C1_NM`(종로구), `ITM_ID`(T04), `DT`(수치)가 들어온다.
→ SGIS 경계 GeoJSON을 `properties.adm_cd == C1`로 join → choropleth. 별도 역매핑 불필요.

---

## 16.5 SGIS 경계 결합 체크리스트

- **코드 자릿수**: 시도=2자리, 시군구=5자리(앞 2자리가 시도). SGIS `boundary hadmarea`는 `--adm-cd` 필수 → 전국은 시도 17개를 각각 `--low-search 1`로 호출 후 병합.
- **통합시 불일치**: SGIS 경계는 통합시를 **일반구로 분할**(예: 수원시 31010 통계 ↔ 경계 31011 장안구). 통계 코드가 경계에 없으면 폴백: `cd → cd[:4]+'0'(통합시 본청) → cd[:2](세종 등 시도단위)`.
- **결측값**: `'X'`(비공개), `'-'`(해당없음)는 숫자가 아니므로 None 처리 후 회색 표시.
- **좌표계**: SGIS 경계는 UTM-K(EPSG:5179). Leaflet 표시 시 `proj4`로 좌표를 미리 WGS84로 변환 후 표준 `L.geoJSON` 사용(Proj4Leaflet CRS 트릭은 SVG 좌표 NaN 위험). 자세히는 `sgis` 스킬의 LEARNINGS 참조.

---

## 16.6 자주 쓰는 지역×지표 통계표 (검증됨)

| 통계표 | 내용 | 분류축 | 핵심 항목 |
|--------|------|--------|-----------|
| `101 DT_1FS1101` | 시도(시군구)/산업분류별 주요지표(10명 이상), 2020~2024 | A=행정구역(283) · B=산업(807, `C`=제조업) | T04=출하액, T06=부가가치, T01=사업체수, T02=종사자수 |

> ⚠ "제조업 **생산액**"을 찾을 때 검색어 `"제조업 생산액"`은 생산'지수'만 반환한다.
> 시군구 단위 실액수는 **출하액(T04)**이며 검색어는 **"광업제조업조사"**를 써라. (→ 15-ai-workflow 용어 매핑)
