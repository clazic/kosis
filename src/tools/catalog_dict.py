#!/usr/bin/env python3
"""collect_catalog.py 결과(columns.jsonl)로 컬럼 사전을 만든다.

- 축(class1..8): 이름별 사용 수, 최상위 코드 서명(signature)별 갈래, 묶음 축 여부
- 같은 뜻 다른 이름: 최상위 코드 서명이 같은 축 이름들
- 항목: 이름·단위 빈도, 단위 칸이 빈 항목
결과: dictionary.json (AI용), dictionary.md (사람용). 위치는 collect_catalog.py 와 같은 KOSIS_CATALOG_DIR.

뜻은 미리 정한 목록이 아니라 수집한 데이터에서만 끌어낸다.
사용: python3 -I src/tools/catalog_dict.py
"""
import collections as C
import json
import os
import re

DIR = os.environ.get("KOSIS_CATALOG_DIR") or os.path.expanduser("~/.kosis/catalog")
P = lambda n: os.path.join(DIR, n)
SIG_N = 4  # 서명에 쓰는 최상위 코드 수


def norm(s):
    return re.sub(r"\s+", "", s or "")


def sig(top):
    return " | ".join(norm(t) for t in top[:SIG_N])


def main():
    rows = [json.loads(l) for l in open(P("columns.jsonl"), encoding="utf-8") if l.strip()]
    axis_names = C.Counter(norm(a["name"]) for r in rows for a in r["axes"])
    vocab = {n for n, c in axis_names.items() if c >= 3}  # 다른 표에서 축 이름으로 쓰이는 말

    by_name = C.defaultdict(lambda: {"tables": 0, "name_eng": C.Counter(), "sigs": C.Counter(), "examples": {},
                                     "codes": [], "mixed": 0})
    by_sig = C.defaultdict(lambda: {"tables": 0, "names": C.Counter(), "top": None})
    nax = C.Counter()
    for r in rows:
        nax[len(r["axes"])] += 1
        for a in r["axes"]:
            n, s = a["name"], sig(a["top"])
            d = by_name[n]
            d["tables"] += 1
            d["codes"].append(a["codes"])
            if a.get("name_eng"):
                d["name_eng"][a["name_eng"].strip()] += 1
            d["sigs"][s] += 1
            d["examples"].setdefault(s, f'{r["org_id"]}/{r["tbl_id"]}')
            # 묶음 축: 최상위 코드(첫 '전체·계' 뒤) 절반 이상이 다른 표의 축 이름
            rest = [norm(t) for t in a["top"][1:]]
            if len(rest) >= 2 and sum(t in vocab or t + "별" in vocab for t in rest) * 2 >= len(rest):
                d["mixed"] += 1
            g = by_sig[s]
            g["tables"] += 1
            g["names"][n] += 1
            g["top"] = g["top"] or a["top"][:SIG_N]

    axes = []
    for n, d in sorted(by_name.items(), key=lambda x: -x[1]["tables"]):
        codes = sorted(d["codes"])
        axes.append({
            "name": n, "tables": d["tables"],
            "name_eng": d["name_eng"].most_common(1)[0][0] if d["name_eng"] else "",
            "mixed_ratio": round(d["mixed"] / d["tables"], 2),
            "codes_median": codes[len(codes) // 2],
            "variants": len(d["sigs"]),
            "top_sigs": [{"top": s, "tables": c, "example": d["examples"][s]} for s, c in d["sigs"].most_common(5)],
        })
    synonyms = []
    for s, g in sorted(by_sig.items(), key=lambda x: -x[1]["tables"]):
        if len(g["names"]) >= 2 and g["tables"] >= 5:
            synonyms.append({"top": s, "tables": g["tables"], "names": g["names"].most_common(8)})

    items = C.Counter()
    units = C.Counter()
    item_units = C.defaultdict(C.Counter)
    for r in rows:
        for i in r["items"]:
            items[norm(i["name"])] += 1
            units[i["unit"] or "(빈 칸)"] += 1
            item_units[norm(i["name"])][i["unit"] or "(빈 칸)"] += 1

    out = {
        "tables": len(rows), "axis_count_dist": dict(sorted(nax.items())), "distinct_axis_names": len(by_name),
        "axes": axes, "synonyms": synonyms[:300],
        "units": units.most_common(60),
        "items": [{"name": n, "tables": c, "units": item_units[n].most_common(3)} for n, c in items.most_common(300)],
    }
    with open(P("dictionary.json"), "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)
    write_md(out)
    print(f"표 {len(rows)} · 축 이름 {len(by_name)} · 같은 뜻 묶음 {len(synonyms)} → {P('dictionary.md')}")


def write_md(o):
    md = lambda t: t.replace(" | ", " · ")  # 표 칸 안의 | 는 마크다운 표를 깨므로
    L = [f"# KOSIS 통계표 컬럼 사전", "",
         f"- 대상 통계표: {o['tables']:,}개 (주제별 목록, `kosis m --type ITM`)",
         f"- 서로 다른 축 이름: {o['distinct_axis_names']:,}개",
         "- 축 개수별 표 수: " + ", ".join(f"{k}축 {v:,}" for k, v in o["axis_count_dist"].items()), "",
         "## 공통 컬럼", "",
         "모든 표의 데이터 열은 같다. 표마다 다른 것은 C1~C8 이 **무슨 축인지**와 ITM 이 **무슨 항목인지**다.", "",
         "| 열 | 뜻 |", "|---|---|",
         "| `C1`~`C8` / `C1_NM`~`C8_NM` | 분류 축 코드/이름. N 은 메타의 `OBJ_ID_SN` (= `--classN`) |",
         "| `ITM_ID` / `ITM_NM` | 항목 코드/이름 — 값(DT)이 무엇을 센 것인지 |",
         "| `UNIT_NM` | 단위. **빈 칸이 많다** (아래 단위 표) — 항목 이름·주석에서 확인 |",
         "| `PRD_SE` / `PRD_DE` | 수록주기 / 시점 |", "| `DT` | 값 |", "",
         "## 축 이름 상위 100", "",
         "`묶음`= 최상위 코드가 다른 표의 축 이름(성별·연령별…)인 비율. 높으면 **이름이 아니라 최상위 코드가 뜻**이다 — "
         "한 축 안에 여러 기준이 있고, 기준 하나의 하위 코드만 골라야 한다(`kosis m --under`).",
         "`갈래`= 같은 이름인데 최상위 코드가 다른 경우의 수 — 이름이 같아도 코드 체계가 다르다.", "",
         "| 축 이름 | 영문 | 표 수 | 묶음 | 갈래 | 코드 수(중앙) | 가장 흔한 최상위 코드 | 예시 표 |", "|---|---|---:|---:|---:|---:|---|---|"]
    for a in o["axes"][:100]:
        t = a["top_sigs"][0]
        L.append(f"| {a['name']} | {a['name_eng']} | {a['tables']:,} | {a['mixed_ratio']:.0%} | {a['variants']:,} | "
                 f"{a['codes_median']} | {md(t['top'])} | {t['example']} |")
    L += ["", "## 같은 뜻 다른 이름 (최상위 코드가 같은 축)", "",
          "| 최상위 코드 | 표 수 | 쓰인 축 이름 |", "|---|---:|---|"]
    for s in o["synonyms"][:60]:
        L.append(f"| {md(s['top'])} | {s['tables']:,} | " + ", ".join(f"{n}({c})" for n, c in s["names"]) + " |")
    L += ["", "## 단위", "", "| 단위 | 항목 수 |", "|---|---:|"]
    L += [f"| {u} | {c:,} |" for u, c in o["units"][:30]]
    L += ["", "## 항목 이름 상위 60", "", "| 항목 | 표 수 | 단위 |", "|---|---:|---|"]
    L += [f"| {i['name']} | {i['tables']:,} | " + ", ".join(f"{u}({c})" for u, c in i["units"]) + " |" for i in o["items"][:60]]
    with open(P("dictionary.md"), "w", encoding="utf-8") as f:
        f.write("\n".join(L) + "\n")


if __name__ == "__main__":
    main()
