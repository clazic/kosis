# 16. 지역코드 매핑 & 지도 결합 (Choropleth)

> KOSIS 통계를 지도(SGIS 경계 등)에 칠하거나 DB와 join할 때의 표준 절차.
> **핵심: 기계 결합엔 '코드', 사람 표시엔 '이름'.** 한글 이름은 중복(예: '중구'가 여러 시도에 존재)이라 join 키로 못 쓴다. 유일하게 안정적인 키는 분류 '코드'(`C1`=행정구역 코드 등)다.

---

## 16.1 핵심 원칙

- SGIS 경계 GeoJSON의 `adm_cd`(시도 2자리 · 시군구 5자리)와 join하려면 KOSIS 분류 **코드**(`C1`)가 필요하다.
- KOSIS 행정구역 코드 체계 = 통계청 행정구역 코드 = **SGIS adm_cd와 동일**. 즉 코드만 확보하면 무가공 join이 된다.
- 한글 이름 매칭은 **최후수단**이며 중복 때문에 위험하다.

---

## 16.2 코드를 얻는 2가지 방법 (우선순위 순)

| 순위 | 방법 | 상태 | 비고 |
|------|------|------|------|
| 1 | `-f json`(코드 자동 포함) 또는 `--with-code` | **현재 표준 (v0.5.0+)** | 역매핑 불필요. 16.3 |
| 2 | `meta -f json`의 CLASSIFICATIONS 역매핑 | **구버전 폴백** | v0.5.0 미만 또는 코드가 비는 특수상황. 16.4 |

> ✅ **v0.5.0부터** `kosis data`가 `-f json`·`--with-code`·`--fields` 사용 시 분류·항목 코드(`C1~C8`, `ITM_ID`)를 자동 출력한다(이슈 [clazic/kosis#1](https://github.com/clazic/kosis/issues/1) 해결). **table/csv 기본 출력은 종전대로 이름만** 나오니 코드가 필요하면 `--with-code`를 쓴다.

---

## 16.3 표준 레시피 (현재, v0.5.0+) — CLI 코드 출력

```bash
# 시군구별 제조업 출하액을 코드와 함께 (JSON은 코드 자동 포함)
kosis d 101 DT_1FS1101 -c1 ALL -c2 C -i T04 -p Y -l 1 -f json
# table/csv에서 코드가 필요하면 --with-code
kosis d 101 DT_1FS1101 -c1 ALL -c2 C -i T04 -p Y -l 1 --with-code -o mfg.csv
```

JSON 각 레코드에 **`"<분류축명> 코드"`** 키가 이름 옆에 함께 나온다(키 이름은 메타 분류축명 기반):

```json
{ "시도별 코드":"11010", "시도별":"종로구",
  "산업별 코드":"C", "산업별":"제조업(10~34)",
  "항목 코드":"T04", "항목":"출하액",
  "수치값":"1262246", "단위":"백만원", "시점":"2024" }
```

→ `"시도별 코드"`(시군구 5자리)를 SGIS 경계 GeoJSON `properties.adm_cd`와 join → choropleth. **역매핑 불필요.**

> 특정 코드 컬럼만 뽑으려면 `--fields "C1,C1_NM,DT"`처럼 원시 코드 키를 직접 지정할 수 있다(v0.5.0+에서 정상 동작).

---

## 16.4 폴백 레시피 (구버전 < v0.5.0) — meta 역매핑, 실측 검증됨 (2026-06-21)

> ⚠ v0.5.0 이상에서는 16.3을 쓰면 된다. 아래는 코드가 `null`로 나오는 구버전 또는 특수상황 폴백이다.

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
