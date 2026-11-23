#!/usr/bin/env python3
"""collect_catalog.py 결과(tables·columns·periods.jsonl)로 kosis 로컬 카탈로그(catalog.jsonl.gz)를 만든다.

한 줄에 표 하나: {"o","t","n","p":[주제 경로],"a":[축 이름],"i":[항목 이름],"c":[축 위 단계 코드 이름],"pr":[[주기,시작,끝]]}
kosis 는 KOSIS_CATALOG_FILE 또는 ~/.kosis/catalog.jsonl.gz 를 읽는다 (internal/catalog).

사용: python3 -I src/tools/build_catalog.py [-o 출력파일]   (입력 폴더는 KOSIS_CATALOG_DIR, 기본 ~/.kosis/catalog)
"""
import gzip
import json
import os
import sys

DIR = os.environ.get("KOSIS_CATALOG_DIR") or os.path.expanduser("~/.kosis/catalog")
MAX_ITEMS, MAX_CODES = 40, 60


def load(name):
    p = os.path.join(DIR, name)
    if not os.path.exists(p):
        return []
    with open(p, encoding="utf-8") as f:
        return [json.loads(l) for l in f if l.strip()]


def main():
    out = sys.argv[sys.argv.index("-o") + 1] if "-o" in sys.argv else os.path.join(DIR, "catalog.jsonl.gz")
    cols = {(c["org_id"], c["tbl_id"]): c for c in load("columns.jsonl")}
    prds = {(p["org_id"], p["tbl_id"]): p["periods"] for p in load("periods.jsonl")}
    # 시범 수집(--roots)은 경로 첫 칸이 목록 ID(I1) — 최상위 이름(주거)으로 바꾼다
    roots = {"A": "인구", "B": "사회일반", "C": "범죄ㆍ안전", "D": "노동", "E": "소득ㆍ소비ㆍ자산", "F": "보건", "G": "복지",
             "H1": "교육ㆍ훈련", "H2": "문화ㆍ여가", "I1": "주거", "I2": "국토이용", "J1": "경제일반ㆍ경기", "J2": "기업경영",
             "K1": "농림", "K2": "수산", "L": "광업ㆍ제조업", "M1": "건설", "M2": "교통ㆍ물류", "N1": "정보통신", "N2": "과학ㆍ기술",
             "O": "도소매ㆍ서비스", "P1": "임금", "P2": "물가", "Q": "국민계정", "R": "정부ㆍ재정", "S1": "금융", "S2": "무역ㆍ국제수지",
             "T": "환경", "U": "에너지", "V": "지역통계"}
    n = with_meta = with_prd = 0
    seen = set()
    with gzip.open(out, "wt", encoding="utf-8") as f:
        for t in load("tables.jsonl"):
            key = (t["org_id"], t["tbl_id"])
            if key in seen:
                continue
            seen.add(key)
            path = list(t.get("path", []))
            if path and path[0] in roots:
                path[0] = roots[path[0]]
            row = {"o": key[0], "t": key[1], "n": t.get("tbl_nm", ""), "p": path}
            c = cols.get(key)
            if c:
                with_meta += 1
                row["a"] = [a["name"] for a in c.get("axes", [])]
                row["i"] = list(dict.fromkeys(i["name"] for i in c.get("items", [])))[:MAX_ITEMS]
                codes = [x for a in c.get("axes", []) for x in a.get("top", [])]
                row["c"] = list(dict.fromkeys(codes))[:MAX_CODES]
            p = prds.get(key)
            if p:
                with_prd += 1
                row["pr"] = [[x["prd_se"], x["start"], x["end"]] for x in p]
            f.write(json.dumps(row, ensure_ascii=False, separators=(",", ":")) + "\n")
            n += 1
    print(f"{out}: 표 {n}개 (메타 {with_meta}, 시점 {with_prd}), {os.path.getsize(out) / 1e6:.1f}MB")


if __name__ == "__main__":
    main()
