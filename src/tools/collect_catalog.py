#!/usr/bin/env python3
"""KOSIS 전체 통계표의 컬럼(분류 축·항목·시점)을 수집한다.

1단계 tree : 주제별(MT_ZTITLE) 목록 트리를 끝까지 따라가 통계표를 모은다 → tables.jsonl
2단계 meta : 통계표마다 kosis m --type ITM (API 1회) → columns.jsonl (축 이름·코드 수·예시, 항목·단위)
           시점(PRD)은 API 를 한 번 더 부르므로 나중에 따로 받는다

끊겨도 다시 실행하면 이어서 한다 (이미 받은 노드·표는 건너뜀). 실패는 errors.jsonl.
사용: python3 -I src/tools/collect_catalog.py [tree|meta|all] [--workers N]
키는 ~/.kosis/config.yaml 의 api_keys 를 모두 쓴다 (키당 1분 180건). 키 3개면 --workers 24 로 한도를 다 쓴다.
"""
import json, os, subprocess, sys, threading, time
from concurrent.futures import ThreadPoolExecutor

# 결과는 저장소 밖에 쌓는다 (수십만 줄). KOSIS_CATALOG_DIR 로 바꿀 수 있다.
DIR = os.environ.get("KOSIS_CATALOG_DIR") or os.path.expanduser("~/.kosis/catalog")
os.makedirs(DIR, exist_ok=True)
P = lambda n: os.path.join(DIR, n)
lock = threading.Lock()


RATE = 180 / 60  # KOSIS 한도 키당 1분 200건 → 여유 두고 키당 초당 3건


def load_keys():
    # ~/.kosis/config.yaml 의 api_keys 목록. kosis 의 시작 키는 무작위라 키별 한도를 지킬 수 없으므로
    # 여기서 키를 골라 KOSIS_API_KEY 로 넘긴다. 키 값은 출력하지 않는다.
    keys, inside = [], False
    with open(os.path.expanduser("~/.kosis/config.yaml"), encoding="utf-8") as f:
        for line in f:
            if line.startswith("api_keys:"):
                inside = True
            elif inside and line.lstrip().startswith("- "):
                keys.append(line.split("- ", 1)[1].strip().strip("'\""))
            elif inside and line.strip():
                break
    return keys or [None]


KEYS = load_keys()
_next = [0.0] * len(KEYS)


def throttle():
    """가장 빨리 쓸 수 있는 키를 골라 그 시각까지 기다린 뒤 키를 돌려준다."""
    with lock:
        now = time.monotonic()
        k = min(range(len(KEYS)), key=lambda i: _next[i])
        wait = _next[k] - now
        _next[k] = max(now, _next[k]) + 1 / RATE
    if wait > 0:
        time.sleep(wait)
    return k


def penalize(k):
    with lock:
        _next[k] = time.monotonic() + 61  # 한도 걸린 키는 1분 쉬게


def kosis(*args, tries=3):
    i = 0
    while i < tries:
        k = throttle()
        env = {**os.environ, "KOSIS_API_KEY": KEYS[k]} if KEYS[k] else None
        r = subprocess.run(["kosis", *args, "-f", "json"], capture_output=True, text=True, timeout=180, env=env)
        if r.returncode == 0:
            out = r.stdout.strip()
            return json.loads(out) if out else []
        err = next((l for l in r.stderr.splitlines() if l.startswith("Error")), r.stderr.strip()[-300:])
        if "[30]" in err:  # 결과 없음
            return []
        if "[40]" in err:  # 분당 한도 초과 → 그 키만 1분 쉬고 다른 키로 다시 (재시도 횟수에 안 셈)
            penalize(k)
            continue
        i += 1
        time.sleep(2 * i)
    raise RuntimeError(err)


def load(name):
    if not os.path.exists(P(name)):
        return []
    with open(P(name), encoding="utf-8") as f:
        return [json.loads(l) for l in f if l.strip()]


def append(name, obj):
    with lock, open(P(name), "a", encoding="utf-8") as f:
        f.write(json.dumps(obj, ensure_ascii=False) + "\n")


def tree(workers):
    # visited.jsonl: 펼친 목록 노드. 이어하기 때 다시 펼치지 않는다.
    done = {v["id"] for v in load("visited.jsonl")}
    seen_tbl = {(t["org_id"], t["tbl_id"]) for t in load("tables.jsonl")}
    pending = [(k["id"], k["path"]) for v in load("visited.jsonl") for k in v["children"] if k["id"] not in done]
    if not done:
        pending = [("", [])]

    def expand(node):
        nid, path = node
        try:
            rows = kosis("ls", "--parent", nid) if nid else kosis("ls", "--parent", "")
        except Exception as e:
            append("errors.jsonl", {"stage": "tree", "id": nid, "err": str(e)})
            return []
        kids = []
        for r in rows:
            if "LIST_ID" in r:
                kids.append({"id": r["LIST_ID"], "path": path + [r["LIST_NM"]]})
            elif "통계표ID" in r:
                key = (r["기관ID"], r["통계표ID"])
                with lock:
                    new = key not in seen_tbl
                    seen_tbl.add(key)
                if new:
                    append("tables.jsonl", {"org_id": key[0], "tbl_id": key[1], "tbl_nm": r.get("통계표명", ""), "path": path})
        append("visited.jsonl", {"id": nid, "children": kids})
        return [(k["id"], k["path"]) for k in kids]

    with ThreadPoolExecutor(workers) as ex:
        while pending:
            batch, pending = pending, []
            for kids in ex.map(expand, batch):
                pending += [k for k in kids if k[0] not in done]
            done.update(b[0] for b in batch)
            print(f"tree: 노드 {len(done)} · 통계표 {len(seen_tbl)} · 대기 {len(pending)}", file=sys.stderr, flush=True)


def summarize(t, rows):
    """kosis m --type ITM 원본 → 분류 축(class1..)·항목. 시점은 prd 단계에서 따로."""
    axes, items = {}, []
    for c in rows:
        if c.get("OBJ_ID") == "ITEM":
            items.append({"id": c["ITM_ID"], "name": c["ITM_NM"], "name_eng": c.get("ITM_NM_ENG") or "", "unit": c.get("UNIT_NM") or ""})
            continue
        a = axes.setdefault(c["OBJ_ID"], {"class": f"class{c.get('OBJ_ID_SN')}", "obj_id": c["OBJ_ID"], "name": c["OBJ_NM"],
                                         "name_eng": c.get("OBJ_NM_ENG") or "", "codes": 0, "top": []})
        a["codes"] += 1
        if not c.get("UP_ITM_ID") and len(a["top"]) < 8:
            a["top"].append(c["ITM_NM"])
    return {**t, "axes": sorted(axes.values(), key=lambda a: int(a["class"][5:]) if a["class"][5:].isdigit() else 99), "items": items}


def meta(workers):
    done = {(c["org_id"], c["tbl_id"]) for c in load("columns.jsonl")}
    todo = [t for t in load("tables.jsonl") if (t["org_id"], t["tbl_id"]) not in done]
    n = [0]

    def one(t):
        try:
            append("columns.jsonl", summarize(t, kosis("m", t["org_id"], t["tbl_id"], "--type", "ITM")))
        except Exception as e:
            append("errors.jsonl", {"stage": "meta", "org_id": t["org_id"], "tbl_id": t["tbl_id"], "err": str(e)})
        with lock:
            n[0] += 1
            if n[0] % 100 == 0:
                print(f"meta: {n[0]}/{len(todo)}", file=sys.stderr, flush=True)

    with ThreadPoolExecutor(workers) as ex:
        list(ex.map(one, todo))
    print(f"meta: 끝 {n[0]}/{len(todo)}", file=sys.stderr)


if __name__ == "__main__":
    stage = sys.argv[1] if len(sys.argv) > 1 else "all"
    workers = int(sys.argv[sys.argv.index("--workers") + 1]) if "--workers" in sys.argv else 4
    if stage in ("tree", "all"):
        tree(workers)
    if stage in ("meta", "all"):
        meta(workers)
