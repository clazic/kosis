#!/usr/bin/env python3
"""AI 가 kosis 로 통계 질문에 답하게 하고 채점한다 (평가 세트: src/eval/questions.json).

  validate          문항 형식 점검
  verify            정답(truth)을 kosis 로 다시 조회해 expect 값과 대조 — 값이 바뀌었거나 문항이 틀렸으면 알린다
  run               문항마다 AI 한 개를 띄워 답을 받고 채점 → 결과 폴더(results.jsonl, summary.md, transcripts/)
  report <폴더>     저장된 결과로 요약을 다시 만든다

경로(--path):
  cli   AI 가 kosis CLI 와 스킬 문서(SKILL.md·references·LEARNINGS.md)를 쓴다
  mcp   AI 가 kosis-mcp 도구만 쓴다 (스킬 문서 없음)

격리: kosis 는 임시 HOME 에서 돈다 (API 키만 복사, 로컬 규칙 rules.d 없음, 새 캐시).
      rules add·sync·revert, config 변경, update 는 막는다 — 앞 문항이 뒤 문항을 돕지 않게.
채점: 값이 맞으면 correct. 값이 틀렸는데 AI 가 답으로 냈으면 silent_wrong(모르고 낸 오답).

사용:
  python3 -I src/tools/run_eval.py verify
  python3 -I src/tools/run_eval.py run --path cli --model sonnet --ids cpi-rate-month,unsold-month
표준 라이브러리만 쓴다. AI 실행은 claude CLI (claude -p).
"""
import argparse
import datetime
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
QUESTIONS = os.path.join(REPO, "src", "eval", "questions.json")
RESULTS = os.path.join(REPO, "src", "eval", "results")

TYPES = ("find", "codes", "period", "calc", "definition", "absent", "chain")
DEFAULT_TOL = 0.005  # 상대 오차 0.5%
BLOCKED = ("rules add", "rules sync", "rules revert", "config set-key", "config add-key",
           "config remove-key", "config setup", "config cache-clear", "update", "uninstall")


# ── 문항 ────────────────────────────────────────────────────────

def load_questions(path=QUESTIONS):
    with open(path, encoding="utf-8") as f:
        return json.load(f)["questions"]


def validate(qs):
    """문항 형식 문제 목록 (없으면 빈 목록)."""
    probs, seen = [], set()
    for i, q in enumerate(qs):
        qid = q.get("id") or f"#{i}"
        say = lambda m: probs.append(f"{qid}: {m}")
        if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", q.get("id", "")):
            say("id 는 소문자·숫자·하이픈")
        if qid in seen:
            say("id 중복")
        seen.add(qid)
        if q.get("type") not in TYPES:
            say(f"type 은 {'|'.join(TYPES)}")
        if not q.get("question"):
            say("question 없음")
        if not q.get("source"):
            say("source 없음 (어디서 나온 문항인지)")
        answers = q.get("answers", [])
        if q.get("type") == "absent":
            if answers:
                say("absent 문항에는 answers 를 두지 않는다")
            if not q.get("checked"):
                say("absent 문항은 checked(확인한 범위) 필요")
            continue
        if not answers:
            say("answers 없음")
        for j, a in enumerate(answers):
            tag = f"answers[{j}]"
            if not re.fullmatch(r"[A-Za-z0-9_]+/[A-Za-z0-9_.\-]+", a.get("table", "")):
                say(f"{tag}.table 은 ORG/TBL")
            if "value" not in a:
                say(f"{tag}.value 없음")
            truths = a.get("truth")
            if isinstance(truths, dict) and "steps" in truths:
                for st in truths["steps"]:
                    if not st.get("cmd") and not st.get("calc"):
                        say(f"{tag}.truth.steps 의 단계에는 cmd 또는 calc 가 필요")
                shows = sum(1 for st in truths["steps"] if st.get("show"))
                if shows and shows != len(q.get("chain") or []):  # show 없는 단계 정답은 다른 해석 경로 (chain_alt)
                    say(f"chain {len(q.get('chain') or [])}개와 show 단계 {shows}개가 다름")
                continue
            truths = truths if isinstance(truths, list) else [truths]
            for t in truths:
                if not t or not t.get("cmd"):
                    say(f"{tag}.truth.cmd 없음 (정답을 다시 조회할 kosis 인자)")
                elif shlex.split(t["cmd"])[0] not in ("d", "ratio"):
                    say(f"{tag}.truth.cmd 는 d 또는 ratio 로 시작")
                if t and t.get("agg") not in (None, "sum", "count"):
                    say(f"{tag}.truth.agg 는 sum|count")
    return probs


# ── 값 비교 ─────────────────────────────────────────────────────

def to_num(v):
    if isinstance(v, bool):
        return None
    if isinstance(v, (int, float)):
        return float(v)
    if isinstance(v, str):
        s = v.strip().replace(",", "").replace("%", "").replace("+", "")
        try:
            return float(s)
        except ValueError:
            return None
    return None


def squash(s):
    return re.sub(r"\s+", "", str(s)).lower()


def value_match(got, want, tol=None, abs_tol=None):
    wn = to_num(want) if not isinstance(want, str) else None
    if wn is None:  # 문자열 정답 (지역 이름·사망원인 등) — 공백 무시, 한쪽이 다른 쪽을 포함
        g, w = squash(got), squash(want)
        return bool(g) and (g == w or w in g or g in w)
    gn = to_num(got)
    if gn is None:
        return False
    if abs_tol is not None:
        return abs(gn - wn) <= abs_tol
    tol = DEFAULT_TOL if tol is None else tol
    return abs(gn - wn) <= abs(wn) * tol + 1e-12


def period_match(got, want):
    g, w = re.sub(r"\D", "", str(got or "")), re.sub(r"\D", "", str(want or ""))
    return bool(g) and (g == w or g.startswith(w) or w.startswith(g))


# ── kosis 실행 환경 (임시 HOME + 기록하는 래퍼) ──────────────────

class Sandbox:
    """임시 HOME(키만 복사)과, 호출을 기록하고 쓰기 명령을 막는 kosis 래퍼."""

    def __init__(self, kosis_bin, skill_dir, catalog=None):
        self.catalog = os.path.abspath(catalog) if catalog else None
        self.real = os.path.abspath(kosis_bin)
        self.root = tempfile.mkdtemp(prefix="kosis-eval-")
        os.chmod(self.root, 0o700)
        # 스킬 문서만 복사한다 — 저장소를 통째로 보여 주면 AI 가 정답 파일(src/eval)을 읽을 수 있다
        self.skill = os.path.join(self.root, "skill")
        os.makedirs(self.skill)
        for name in ("SKILL.md", "LEARNINGS.md"):
            shutil.copy(os.path.join(skill_dir, name), self.skill)
        shutil.copytree(os.path.join(skill_dir, "references"), os.path.join(self.skill, "references"))
        self.home = os.path.join(self.root, "home")
        os.makedirs(os.path.join(self.home, ".kosis", "rules.d"))
        src = os.path.expanduser("~/.kosis/config.yaml")
        if os.path.exists(src):
            shutil.copy(src, os.path.join(self.home, ".kosis", "config.yaml"))
            os.chmod(os.path.join(self.home, ".kosis", "config.yaml"), 0o600)
        self.bin = os.path.join(self.root, "bin")
        os.makedirs(self.bin)
        cases = "|".join(f'"{b}"*' for b in BLOCKED)
        wrapper = f"""#!/bin/sh
# kosis 평가용 래퍼: 호출 기록, 쓰기 명령 차단
case "$1 $2" in {cases})
  echo "Error: 평가 중에는 쓸 수 없는 명령입니다 ($1 $2) — 조회 명령만 쓰세요" >&2
  [ -n "$KOSIS_EVAL_LOG" ] && printf '1\\t%s\\n' "$*" >> "$KOSIS_EVAL_LOG"
  exit 1;;
esac
HOME={shlex.quote(self.home)} KOSIS_RULES_FILE={shlex.quote(os.path.join(self.skill, "references", "rules.yaml"))} {shlex.quote(self.real)} "$@"
rc=$?
[ -n "$KOSIS_EVAL_LOG" ] && printf '%s\\t%s\\n' "$rc" "$*" >> "$KOSIS_EVAL_LOG"
exit $rc
"""
        self.kosis = os.path.join(self.bin, "kosis")
        with open(self.kosis, "w", encoding="utf-8") as f:
            f.write(wrapper)
        os.chmod(self.kosis, 0o755)

    def env(self, log=None):
        e = dict(os.environ)
        e.pop("KOSIS_CATALOG_FILE", None)  # 로컬 카탈로그는 --catalog 로만 (A/B 비교가 섞이지 않게)
        if self.catalog:
            e["KOSIS_CATALOG_FILE"] = self.catalog
        e["PATH"] = self.bin + os.pathsep + e.get("PATH", "")
        if log:
            e["KOSIS_EVAL_LOG"] = log
        return e

    def close(self):
        shutil.rmtree(self.root, ignore_errors=True)


# ── verify: 정답 다시 조회 ──────────────────────────────────────

def run_truth(sb, truth):
    """truth 를 실행해 값 하나를 돌려준다. (값, 오류). 목록이면 각 값을 더한다 (두 표를 더해야 하는 정답)."""
    if isinstance(truth, list):
        total = 0.0
        for t in truth:
            v, err = run_truth(sb, t)
            if err or to_num(v) is None:
                return None, err or f"숫자가 아닌 값 {v!r}"
            total += to_num(v)
        return total, None
    if isinstance(truth, dict) and "steps" in truth:
        v, err, _ = run_steps(sb, truth["steps"])
        return v, err
    rows, err = kosis_rows(sb, truth["cmd"])
    if err:
        return None, err
    v, err, _ = pick_value(rows, truth)
    return v, err


def kosis_rows(sb, cmd):
    """kosis 인자 글을 실행해 행 목록을 돌려준다. (행, 오류)"""
    args = shlex.split(cmd)
    if args[0] == "d" and "--api-keys" not in args:
        args.append("--api-keys")
    args += ["-f", "json"]
    for attempt in range(3):
        r = subprocess.run([sb.kosis, *args], capture_output=True, text=True, timeout=300, env=sb.env())
        if r.returncode == 0:
            break
        # [40] 분당 한도는 1분, 그 밖의 실패(일시적인 메타·네트워크 오류)는 잠깐 쉬고 다시
        time.sleep(61 if "[40]" in r.stderr else 3)
    if r.returncode != 0:
        err = [l for l in r.stderr.splitlines() if l.startswith("Error")] or r.stderr.strip().splitlines()[-1:]
        return None, "kosis 실패: " + " ".join(err)[:300]
    try:
        return json.loads(r.stdout or "[]"), None
    except json.JSONDecodeError as e:
        return None, f"JSON 파싱 실패: {e}"


def pick_value(rows, t):
    """where 로 행을 고르고 field(기본 DT) 값을 돌려준다. (값, 오류, 고른 행 또는 None)"""
    for k, v in (t.get("where") or {}).items():
        rows = [x for x in rows if str(x.get(k, "")) == str(v)]
    field = t.get("field", "DT")
    agg = t.get("agg")
    if agg == "count":
        return len(rows), None, None
    if agg == "sum":
        nums = [to_num(x.get(field)) for x in rows]
        if not rows or any(n is None for n in nums):
            return None, f"sum: 숫자가 아닌 값이 있거나 행이 없음 ({len(rows)}행)", None
        return sum(nums), None, None
    if len(rows) != 1:
        return None, f"행이 1개가 아님 ({len(rows)}행) — where 로 좁히세요", None
    if field not in rows[0]:
        return None, f"열 {field} 없음 (있는 열: {', '.join(rows[0])})", None
    return rows[0][field], None, rows[0]


def run_steps(sb, steps):
    """연쇄 정답: 앞 단계 결과를 다음 단계 명령에 끼워 넣는다. (마지막 값, 오류, 단계마다 고른 것)

    단계: {"cmd": "d … -c1 {sido} …", "where", "field", "agg",
           "save": {"sido": "C1"},                       고른 행의 열을 변수로
           "map": {"sido2": {"from": "sido", "table": {"26": "31"}}},   표마다 다른 코드 바꾸기
           "show": "C1_NM"}                              이 단계에서 고른 것 (문항 chain 과 대조)
          또는 {"calc": "{a}/{b}*100"} 숫자 계산. 단계 값은 {v1}, {v2} … 로도 쓴다."""
    vars_, shown, val = {}, [], None

    def subst(text):
        for k, v in vars_.items():
            text = text.replace("{" + k + "}", str(v))
        return text

    for i, st in enumerate(steps, 1):
        tag = f"{i}단계"
        row = None
        if "calc" in st:
            expr = subst(st["calc"])
            if not re.fullmatch(r"[0-9eE+\-*/(). ]+", expr):
                return None, f"{tag} calc 에 숫자·연산자가 아닌 것이 있음: {expr}", shown
            try:
                val = eval(expr, {"__builtins__": {}})  # 숫자와 연산자만 통과시킨 식
            except Exception as e:
                return None, f"{tag} calc 실패: {e}", shown
        else:
            cmd = subst(st["cmd"])
            if "{" in cmd:
                return None, f"{tag} 명령에 채워지지 않은 변수: {cmd}", shown
            t = dict(st, where={k: subst(str(v)) for k, v in (st.get("where") or {}).items()})
            need = set((st.get("save") or {}).values()) | ({st["show"]} if st.get("show") else set())
            for attempt in range(3):
                rows, err = kosis_rows(sb, cmd)
                if err:
                    return None, f"{tag} {err} (kosis {cmd})", shown
                val, err, row = pick_value(rows, t)
                # 동시에 여러 조회가 돌 때 kosis 가 가끔 코드 열(C1 …) 없이 답한다 — 다시 조회한다
                if not err and (row is None or need <= set(row)):
                    break
                time.sleep(2)
            if err:
                return None, f"{tag} {err} (kosis {cmd})", shown
        vars_[f"v{i}"] = val
        for name, col in (st.get("save") or {}).items():
            if row is None or col not in row:
                return None, f"{tag} save: 열 {col} 없음", shown
            vars_[name] = row[col]
        for name, m in (st.get("map") or {}).items():
            src = str(vars_.get(m["from"]))
            if src not in m["table"]:
                return None, f"{tag} map {name}: {src!r} 가 표에 없음 — 앞 단계 답이 바뀌었으면 코드를 더 넣으세요", shown
            vars_[name] = m["table"][src]
        if st.get("show"):
            shown.append(row[st["show"]] if row is not None and st["show"] in row else val)
    return val, None, shown


def cmd_verify(a):
    qs = pick(load_questions(), a)
    probs = validate(load_questions())
    if probs:
        print("\n".join(probs), file=sys.stderr)
        return 1
    sb = Sandbox(a.kosis_bin, a.skill_dir)
    bad, changed = 0, {}
    try:
        jobs = [(q, i, ans) for q in qs for i, ans in enumerate(q.get("answers", []))]

        def one(job):
            q, i, ans = job
            if isinstance(ans["truth"], dict) and "steps" in ans["truth"]:
                got, err, shown = run_steps(sb, ans["truth"]["steps"])
                chain = q.get("chain") or []
                if err is None and shown and not (len(shown) == len(chain) and all(
                        any(value_match(g, w) for w in [c["value"]] + list(c.get("aliases") or [])) for g, c in zip(shown, chain))):
                    err = f"단계에서 고른 것이 다름: 조회 {shown} / 문항 {[c['value'] for c in chain]}"
                    return q["id"], i, ans, got, err, False
                if a.verbose and err is None:
                    err = f"단계: {shown}"
            else:
                got, err = run_truth(sb, ans["truth"])
            ok = (err is None or err.startswith("단계: ")) and value_match(got, ans["value"], ans.get("tol", 1e-6), ans.get("abs_tol"))
            return q["id"], i, ans, got, err, ok

        with ThreadPoolExecutor(a.workers) as ex:
            for qid, i, ans, got, err, ok in ex.map(one, jobs):
                if ok:
                    if a.verbose:
                        print(f"ok   {qid}[{i}] {ans['table']} = {got} {err or ''}")
                    continue
                bad += 1
                if got is not None:
                    changed[(qid, i)] = got
                ts = ans["truth"]["steps"] if isinstance(ans["truth"], dict) and "steps" in ans["truth"] else ans["truth"]
                ts = [t for t in (ts if isinstance(ts, list) else [ts]) if t.get("cmd")]
                print(f"FAIL {qid}[{i}] {ans['table']}: expect {ans['value']!r}, got {got!r} {err or ''}\n     " + "\n     ".join("kosis " + t["cmd"] for t in ts))
    finally:
        sb.close()
    if a.update and changed:  # 다시 조회한 값으로 expect 를 고친다 (개정된 통계·새로 쓴 문항)
        with open(QUESTIONS, encoding="utf-8") as f:
            doc = json.load(f)
        for q in doc["questions"]:
            for i, ans in enumerate(q.get("answers", [])):
                if (q["id"], i) in changed:
                    got = changed[(q["id"], i)]
                    num = to_num(got)
                    ans["value"] = got if num is None else (int(num) if num == int(num) and "." not in str(got) else num)
        with open(QUESTIONS, "w", encoding="utf-8") as f:
            json.dump(doc, f, ensure_ascii=False, indent=1)
            f.write("\n")
        print(f"--update: {len(changed)}개 값을 다시 조회한 값으로 고쳤습니다 — 위 목록을 보고 개정인지 문항 오류인지 확인하세요")
    n = sum(len(q.get("answers", [])) for q in qs)
    print(f"정답 {n}개 중 {n - bad}개 일치, {bad}개 불일치 (문항 {len(qs)}개, absent {sum(q['type'] == 'absent' for q in qs)}개는 조회 없음)")
    return 1 if bad else 0


# ── run: AI 실행 ────────────────────────────────────────────────

ANSWER_FORMAT = """마지막에 아래 형식의 JSON 코드 블록 하나로 답을 끝낸다 (그 뒤에 아무것도 쓰지 않는다):
```json
{"status": "ok", "table": "ORG_ID/TBL_ID", "value": 12345.6, "unit": "단위", "period": "시점", "note": "조사명·범위·주의할 점"}
```
- value: 질문이 묻는 값 하나. 숫자는 KOSIS 가 준 단위 그대로(단위를 바꾸거나 반올림하지 않는다). 이름을 묻는 질문(어느 지역, 어떤 원인)이면 그 이름 문자열.
- table: 그 값을 가져온 통계표. period: 그 값의 시점 (예 2025, 202608).
- KOSIS 에서 찾지 못했으면 지어내지 말고 {"status": "absent", "note": "무엇을 어디까지 찾아봤는지"} 로 답한다.
- 계산한 값이면 note 에 계산식과 쓴 표를 적는다.
- 여러 단계를 거쳐야 하는 질문이면 "steps" 를 더한다: 단계마다 고른 것(지역·연도·항목 이름 또는 값)을 순서대로 적은 배열. 예 "steps": ["전라남도", "영광군"]. table 은 마지막 값을 가져온 표."""

PROMPT_CLI = """너는 KOSIS(국가통계포털) 통계 질문에 답하는 조사원이다.
쓸 수 있는 도구는 터미널의 `kosis` 명령뿐이다. 사용 안내는 {skill}/SKILL.md 에 있다 — 먼저 읽고 그 안내대로 조회한다
(참조 문서는 {skill}/references/, 오답노트는 {skill}/LEARNINGS.md).
기억이나 짐작으로 답하지 않는다. kosis 로 조회해 확인한 값만 답한다. 웹 검색은 하지 않는다.
지금은 평가 중이라 규칙 저장(kosis rules add·sync)은 하지 않는다.

질문: {question}

{fmt}"""

PROMPT_MCP = """너는 KOSIS(국가통계포털) 통계 질문에 답하는 조사원이다.
쓸 수 있는 도구는 kosis MCP 도구(kosis_search, kosis_meta, kosis_data …)뿐이다.
기억이나 짐작으로 답하지 않는다. 도구로 조회해 확인한 값만 답한다.

질문: {question}

{fmt}"""


def parse_answer(text):
    """마지막 JSON 코드 블록(없으면 마지막 {…})을 답으로."""
    blocks = re.findall(r"```(?:json)?\s*(\{.*?\})\s*```", text or "", flags=re.S)
    cands = blocks or re.findall(r"(\{[^{}]*\"status\"[^{}]*\})", text or "", flags=re.S)
    for c in reversed(cands):
        try:
            obj = json.loads(c)
            if isinstance(obj, dict) and "status" in obj:
                return obj
        except json.JSONDecodeError:
            continue
    return None


def read_stream(path):
    """claude stream-json 기록 → 마지막 답 글, 도구 호출, 비용."""
    out = {"text": "", "tools": [], "cost_usd": None, "turns": None, "error": None}
    errs = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                ev = json.loads(line)
            except json.JSONDecodeError:
                continue
            msg = ev.get("message") or {}
            if ev.get("type") == "assistant":
                for c in msg.get("content") or []:
                    if c.get("type") == "tool_use":
                        out["tools"].append({"id": c.get("id"), "name": c.get("name"), "input": c.get("input")})
            elif ev.get("type") == "user":
                for c in msg.get("content") or []:
                    if isinstance(c, dict) and c.get("type") == "tool_result":
                        errs[c.get("tool_use_id")] = bool(c.get("is_error"))
            elif ev.get("type") == "result":
                out["text"] = ev.get("result") or ""
                if ev.get("api_error") or ev.get("api_error_status"):  # 사용 한도·API 오류 — AI 가 답한 것이 아니다
                    out["api_error"] = f"{ev.get('api_error_status')} {ev.get('api_error') or ''} {out['text'][:120]}".strip()
                out["cost_usd"] = ev.get("total_cost_usd")
                out["turns"] = ev.get("num_turns")
                if ev.get("is_error"):
                    out["error"] = str(ev.get("subtype") or ev.get("result"))[:300]
    for t in out["tools"]:
        t["is_error"] = errs.get(t["id"], False)
    return out


def cli_calls_from_stream(stream):
    """kosis 호출 기록(.kosis.tsv)이 없을 때: Bash 명령 글에서 kosis 호출을 센다 (추정 — 오류는 Bash 호출 단위)."""
    s = read_stream(stream)
    calls = errors = 0
    for t in s["tools"]:
        if t["name"] != "Bash":
            continue
        n = len(re.findall(r"(?:^|[;&|(\n]\s*)kosis\s", str((t.get("input") or {}).get("command", ""))))
        calls += n
        errors += 1 if n and t["is_error"] else 0
    return calls, errors


def ask_ai(sb, q, a, outdir):
    qid = q["id"]
    work = os.path.join(sb.root, "work", qid)
    os.makedirs(work, exist_ok=True)
    log = os.path.join(outdir, "transcripts", f"{qid}.kosis.tsv")
    stream = os.path.join(outdir, "transcripts", f"{qid}.jsonl")
    base = [a.claude_bin, "-p", "--output-format", "stream-json", "--verbose", "--no-session-persistence",
            "--disable-slash-commands", "--strict-mcp-config", "--setting-sources", "project,local",
            "--permission-mode", "dontAsk", "--model", a.model]
    if a.max_budget:
        base += ["--max-budget-usd", str(a.max_budget)]
    if a.path == "cli":
        prompt = PROMPT_CLI.format(skill=sb.skill, question=q["question"], fmt=ANSWER_FORMAT)
        cmd = base + ["--tools", "Bash,Read,Grep,Glob", "--allowedTools", "Bash", "Read", "Grep", "Glob",
                      "--add-dir", sb.skill]
    else:
        prompt = PROMPT_MCP.format(question=q["question"], fmt=ANSWER_FORMAT)
        menv = {"HOME": sb.home}
        if sb.catalog:
            menv["KOSIS_CATALOG_FILE"] = sb.catalog
        conf = {"mcpServers": {"kosis": {"command": os.path.abspath(a.mcp_bin), "args": [], "env": menv}}}
        cmd = base + ["--tools", "", "--allowedTools", "mcp__kosis", "--mcp-config", json.dumps(conf)]
    t0 = time.time()
    timed_out = False
    with open(stream, "w", encoding="utf-8") as so:
        try:
            p = subprocess.run(cmd, input=prompt, stdout=so, stderr=subprocess.PIPE, text=True,
                               cwd=work, env=sb.env(log), timeout=a.timeout)
            stderr = p.stderr
        except subprocess.TimeoutExpired as e:
            timed_out, stderr = True, (e.stderr or b"").decode("utf-8", "replace") if isinstance(e.stderr, bytes) else (e.stderr or "")
    s = read_stream(stream)
    rec = {"id": qid, "type": q["type"], "seconds": round(time.time() - t0, 1), "cost_usd": s["cost_usd"],
           "turns": s["turns"], "answer": parse_answer(s["text"]), "final_text": s["text"][-2000:]}
    if s.get("api_error"):
        rec["error"], rec["aborted"] = "API 오류로 실행 못 함: " + s["api_error"], True
    elif timed_out:
        rec["error"] = f"시간 초과 ({a.timeout}초)"
    elif s["error"] or (not s["text"] and stderr):
        rec["error"] = (s["error"] or stderr.strip()[-300:])
    if a.path == "cli":
        calls = []
        if os.path.exists(log):
            with open(log, encoding="utf-8") as f:
                for line in f:
                    rc, _, args = line.rstrip("\n").partition("\t")
                    calls.append((rc, args))
        rec["kosis_calls"] = len(calls)
        rec["kosis_errors"] = sum(rc != "0" for rc, _ in calls)
    else:
        calls = [t for t in s["tools"] if str(t["name"]).startswith("mcp__kosis")]
        rec["kosis_calls"] = len(calls)
        rec["kosis_errors"] = sum(t["is_error"] for t in calls)
    rec["other_tools"] = sum(1 for t in s["tools"] if not str(t["name"]).startswith("mcp__kosis") and t["name"] != "Bash")
    return rec


def grade(q, rec):
    """rec 에 outcome·table_match 를 채운다.
    correct / silent_wrong(답으로 냈는데 틀림) / miss(있는데 못 찾음) / no_answer(형식에 맞는 답 없음)
    / review(없다고 알려진 문항에 답을 냄 — 사람이 확인)"""
    if rec.get("aborted") or "hit your" in str(rec.get("final_text", "")).lower():  # 옛 기록: 사용 한도 문구
        rec["aborted"], rec["outcome"] = True, "aborted"
        return rec
    rec["steps_n"] = q.get("steps")
    rec["set"] = q.get("set", "known")  # known: 규칙에 있는 표에서 만든 문항, unseen: 규칙·문서에 없는 표
    ans = rec.get("answer")
    if not ans:
        rec["outcome"] = "no_answer"
        return rec
    status = str(ans.get("status", "")).lower()
    if q["type"] == "absent":
        # 못 찾았던 표를 AI 가 실제로 찾았을 수 있다 — 사람이 확인한다
        rec["outcome"] = "correct" if status == "absent" else "review"
        return rec
    if status != "ok":
        rec["outcome"] = "miss"
        return rec
    tbl = str(ans.get("table", "")).strip().replace(" ", "")
    rec["table_match"] = any(tbl == x["table"] for x in q["answers"])
    hit = None
    for x in q["answers"]:
        wants = [x["value"]] + list(x.get("aliases") or [])
        if x.get("scale_free") and to_num(x["value"]):  # '인구에 비해' — 1천 명당·1만 명당·% 어느 단위든 같은 비율이면 정답
            wants += [to_num(x["value"]) * 10 ** k for k in range(-6, 7) if k]
        if not any(value_match(ans.get("value"), w, x.get("tol"), x.get("abs_tol")) for w in wants):
            continue
        if x.get("check_period") and not period_match(ans.get("period"), x.get("period")):
            continue
        hit = x
        break
    rec["outcome"] = "correct" if hit else "silent_wrong"
    if hit:
        rec["matched"] = hit["table"]
    else:
        # 이름을 묻는 질문에 value 로 숫자를 쓰고 이름은 note 에만 적은 답 — 오답으로 두되 따로 센다 (답 형식을 못 지킨 것)
        names = [w for x in q["answers"] if isinstance(x["value"], str) for w in [x["value"]] + list(x.get("aliases") or [])]
        if names and to_num(ans.get("value")) is not None:
            blob = squash(str(ans.get("note", "")) + json.dumps(ans.get("steps", ""), ensure_ascii=False))
            rec["name_in_note"] = any(squash(w) in blob for w in names)
    chain = q.get("chain")
    if chain:  # 연쇄 문항: 어느 단계에서 처음 틀렸는지 (chain_alt 는 다른 해석으로 고른 단계 — 맞는 쪽을 쓴다)
        steps = ans.get("steps") if isinstance(ans.get("steps"), list) else []

        def step_oks(ch):
            out = []
            for k, c in enumerate(ch):
                c = c if isinstance(c, dict) else {"value": c}
                g = steps[k] if k < len(steps) else None
                out.append(g is not None and any(value_match(g, w, c.get("tol"), c.get("abs_tol")) for w in [c["value"]] + list(c.get("aliases") or [])))
            return out
        oks = max([step_oks(chain)] + [step_oks(alt) for alt in q.get("chain_alt") or []], key=sum)
        rec["alt_path"] = oks != step_oks(chain)
        rec["chain_ok"] = oks
        rec["first_wrong_step"] = next((k + 1 for k, o in enumerate(oks) if not o), None)
        if rec["first_wrong_step"] is None and rec["outcome"] != "correct":
            rec["first_wrong_step"] = len(chain) + 1  # 고른 것은 다 맞고 마지막 값에서 틀림
    need = q.get("mention_any")
    if need:
        blob = squash(json.dumps(ans, ensure_ascii=False))
        rec["mentioned"] = any(squash(w) in blob for w in need)
    return rec


def summarize(recs, meta):
    aborted = [r for r in recs if r.get("outcome") == "aborted"]
    recs = [r for r in recs if r.get("outcome") != "aborted"]
    lines = [f"# 평가 결과 — {meta.get('path')} · {meta.get('model')}", "",
             f"- 실행: {meta.get('started')} · kosis {meta.get('kosis_version')} · 문항 {len(recs)}개",
             f"- {meta['note']}" if meta.get("note") else None, ""]
    cols = ["correct", "silent_wrong", "miss", "no_answer", "review"]
    lines += ["| 유형 | 문항 | 정답 | 모르고 낸 오답 | 못 찾음 | 답 없음 | 확인 필요 | 정답률 | 평균 kosis 호출 | 평균 오류 호출 |",
              "|---|---|---|---|---|---|---|---|---|---|"]

    def row(name, rs):
        if not rs:
            return None
        c = {k: sum(r.get("outcome") == k for r in rs) for k in cols}
        calls = sum(r.get("kosis_calls") or 0 for r in rs) / len(rs)
        errs = sum(r.get("kosis_errors") or 0 for r in rs) / len(rs)
        return (f"| {name} | {len(rs)} | {c['correct']} | {c['silent_wrong']} | {c['miss']} | {c['no_answer']} | {c['review']} | "
                f"{100 * c['correct'] / len(rs):.0f}% | {calls:.1f} | {errs:.1f} |")

    for t in TYPES:
        r = row(t, [x for x in recs if x["type"] == t])
        if r:
            lines.append(r)
    lines.append(row("**전체**", recs) or "")
    sets = sorted({r.get("set", "known") for r in recs})
    if len(sets) > 1:
        lines += ["", "| 문항 묶음 | 문항 | 정답 | 모르고 낸 오답 | 못 찾음 | 답 없음 | 확인 필요 | 정답률 | 평균 kosis 호출 | 평균 오류 호출 |",
                  "|---|---|---|---|---|---|---|---|---|---|"]
        for st in sets:
            lines.append(row(st, [x for x in recs if x.get("set", "known") == st]))
    ch = [r for r in recs if r.get("steps_n")]
    if ch:
        lines += ["", "| 연쇄 단계 수 | 문항 | 정답 | 모르고 낸 오답 | 못 찾음 | 답 없음 | 확인 필요 | 정답률 | 평균 kosis 호출 | 평균 오류 호출 |",
                  "|---|---|---|---|---|---|---|---|---|---|"]
        for n in sorted({r["steps_n"] for r in ch}):
            lines.append(row(f"{n}단계", [r for r in ch if r["steps_n"] == n]))
        fw = [r["first_wrong_step"] for r in ch if r.get("first_wrong_step")]
        if fw:
            lines += ["", "- 연쇄 문항에서 처음 틀린 단계: " + ", ".join(f"{k}단계 {fw.count(k)}건" for k in sorted(set(fw)))]
    slips = [r for r in recs if r.get("name_in_note")]
    if slips:
        lines += ["", f"- 모르고 낸 오답 가운데 {len(slips)}건은 답 형식 문제: 이름을 묻는 질문에 value 로 숫자를 쓰고 정답 이름은 note 에 적었다 ({', '.join(r['id'] for r in slips)})"]
    tm = [r for r in recs if "table_match" in r]
    cost = sum(r.get("cost_usd") or 0 for r in recs)
    secs = sum(r.get("seconds") or 0 for r in recs)
    if aborted:
        lines += ["", f"- **API 오류(사용 한도 등)로 실행하지 못한 {len(aborted)}건은 위 집계에서 뺐다**: {', '.join(r['id'] for r in aborted[:20])}"]
    lines += ["", f"- 정답 표와 같은 표를 쓴 비율: {sum(r['table_match'] for r in tm)}/{len(tm)}" if tm else "",
              f"- 비용 ${cost:.2f} · AI 실행 시간 합 {secs / 60:.1f}분", "",
              "## 틀린 문항", ""]
    wrong = [r for r in recs if r.get("outcome") != "correct"]
    if not wrong:
        lines.append("없음")
    for r in wrong:
        a = r.get("answer") or {}
        if r.get("first_wrong_step"):
            lines.append(f"  - 아래 문항: {r['first_wrong_step']}단계에서 처음 틀림 — AI 가 고른 것 {a.get('steps')}")
        lines.append(f"- `{r['id']}` ({r['type']}) **{r.get('outcome')}** — 답 {a.get('value')!r} ({a.get('table')}, {a.get('period')}) "
                     f"· kosis {r.get('kosis_calls')}회 · {r.get('error') or (a.get('note') or '')[:160]}")
    return "\n".join(l for l in lines if l is not None) + "\n"


def pick(qs, a):
    if getattr(a, "ids", None):
        want = [x.strip() for x in a.ids.split(",") if x.strip()]
        by = {q["id"]: q for q in qs}
        missing = [w for w in want if w not in by]
        if missing:
            sys.exit(f"없는 문항 id: {', '.join(missing)}")
        qs = [by[w] for w in want]
    if getattr(a, "types", None):
        qs = [q for q in qs if q["type"] in a.types.split(",")]
    if getattr(a, "limit", None):
        qs = qs[: a.limit]
    return qs


def cmd_run(a):
    all_qs = load_questions()
    probs = validate(all_qs)
    if probs:
        print("\n".join(probs), file=sys.stderr)
        return 1
    qs = pick(all_qs, a)
    if a.path == "mcp" and not (a.mcp_bin and os.path.exists(a.mcp_bin)):
        sys.exit("--path mcp 에는 --mcp-bin <kosis-mcp 바이너리> 가 필요합니다 (cd src && go build -o /tmp/kosis-mcp ./cmd/kosis-mcp)")
    started = datetime.datetime.now().strftime("%Y-%m-%d-%H%M%S")
    outdir = os.path.abspath(a.out) if a.out else os.path.join(RESULTS, f"{started}-{a.path}-{re.sub(r'[^A-Za-z0-9.-]', '_', a.model)}")
    os.makedirs(os.path.join(outdir, "transcripts"), exist_ok=True)
    sb = Sandbox(a.kosis_bin, a.skill_dir, getattr(a, "catalog", None))
    ver = subprocess.run([a.kosis_bin, "--version"], capture_output=True, text=True).stdout.strip()
    meta = {"path": a.path, "model": a.model, "catalog": getattr(a, "catalog", None), "started": started, "kosis_version": ver, "kosis_bin": a.kosis_bin,
            "skill_dir": a.skill_dir, "questions": len(qs), "mcp_bin": a.mcp_bin if a.path == "mcp" else None}
    by = {q["id"]: q for q in qs}
    recs = []
    try:
        with ThreadPoolExecutor(a.workers) as ex, open(os.path.join(outdir, "results.jsonl"), "w", encoding="utf-8") as rf:
            for rec in ex.map(lambda q: ask_ai(sb, q, a, outdir), qs):
                grade(by[rec["id"]], rec)
                recs.append(rec)
                rf.write(json.dumps(rec, ensure_ascii=False) + "\n")
                rf.flush()
                print(f"{rec['outcome']:<13}{rec['id']}  kosis {rec.get('kosis_calls')}회(오류 {rec.get('kosis_errors')})  {rec['seconds']}s", flush=True)
    finally:
        sb.close()
    with open(os.path.join(outdir, "meta.json"), "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    summary = summarize(recs, meta)
    with open(os.path.join(outdir, "summary.md"), "w", encoding="utf-8") as f:
        f.write(summary)
    print("\n" + summary + f"\n결과 폴더: {outdir}")
    return 0


def cmd_report(a):
    with open(os.path.join(a.dir, "meta.json"), encoding="utf-8") as f:
        meta = json.load(f)
    by = {q["id"]: q for q in load_questions()}
    recs = []
    with open(os.path.join(a.dir, "results.jsonl"), encoding="utf-8") as f:
        for line in f:
            rec = json.loads(line)
            tsv = os.path.join(a.dir, "transcripts", f"{rec['id']}.kosis.tsv")
            stream = os.path.join(a.dir, "transcripts", f"{rec['id']}.jsonl")
            if meta.get("path") == "cli" and not os.path.exists(tsv) and os.path.exists(stream):
                rec["kosis_calls"], rec["kosis_errors"] = cli_calls_from_stream(stream)
                rec["calls_estimated"] = True
            if rec["id"] in by:  # 문항이 고쳐졌으면 다시 채점
                for k in ("outcome", "table_match", "matched", "mentioned", "chain_ok", "first_wrong_step", "name_in_note", "alt_path"):
                    rec.pop(k, None)
                grade(by[rec["id"]], rec)
            recs.append(rec)
    with open(os.path.join(a.dir, "results.jsonl"), "w", encoding="utf-8") as f:
        for rec in recs:
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")
    if any(r.get("calls_estimated") for r in recs):
        meta["note"] = "kosis 호출 수는 Bash 명령 글에서 센 추정값 (호출 기록 파일 없음)"
    summary = summarize(recs, meta)
    with open(os.path.join(a.dir, "summary.md"), "w", encoding="utf-8") as f:
        f.write(summary)
    print(summary)
    return 0


def cmd_validate(a):
    qs = load_questions()
    probs = validate(qs)
    if probs:
        print("\n".join(probs), file=sys.stderr)
        return 1
    by = {}
    for q in qs:
        by[q["type"]] = by.get(q["type"], 0) + 1
    print(f"문항 {len(qs)}개 형식 이상 없음 — " + ", ".join(f"{t} {by.get(t, 0)}" for t in TYPES))
    return 0


def main():
    ap = argparse.ArgumentParser(description="kosis AI 평가")
    sub = ap.add_subparsers(dest="cmd", required=True)

    def common(p):
        p.add_argument("--ids", help="문항 id (쉼표)")
        p.add_argument("--types", help="유형 (쉼표)")
        p.add_argument("--limit", type=int, help="앞에서 N개만")
        p.add_argument("--kosis-bin", default=shutil.which("kosis") or "kosis", help="평가할 kosis 바이너리 (기본: PATH)")
        p.add_argument("--skill-dir", default=REPO, help="SKILL.md·references 가 있는 폴더 (기본: 이 저장소)")
        p.add_argument("--workers", type=int, default=2)

    sub.add_parser("validate").set_defaults(fn=cmd_validate)
    v = sub.add_parser("verify")
    common(v)
    v.add_argument("-v", "--verbose", action="store_true")
    v.add_argument("--update", action="store_true", help="어긋난 expect 값을 다시 조회한 값으로 고친다")
    v.set_defaults(fn=cmd_verify)
    r = sub.add_parser("run")
    common(r)
    r.add_argument("--path", choices=("cli", "mcp"), default="cli")
    r.add_argument("--model", default="sonnet")
    r.add_argument("--mcp-bin", help="kosis-mcp 바이너리 (--path mcp)")
    r.add_argument("--claude-bin", default=shutil.which("claude") or "claude")
    r.add_argument("--timeout", type=int, default=900, help="문항당 제한 시간(초)")
    r.add_argument("--max-budget", type=float, default=2.0, help="문항당 비용 상한(USD), 0 이면 없음")
    r.add_argument("--out", help="결과 폴더")
    r.add_argument("--catalog", help="로컬 카탈로그 파일 (KOSIS_CATALOG_FILE). 없으면 쓰지 않는다")
    r.set_defaults(fn=cmd_run)
    rp = sub.add_parser("report")
    rp.add_argument("dir")
    rp.set_defaults(fn=cmd_report)
    a = ap.parse_args()
    sys.exit(a.fn(a))


if __name__ == "__main__":
    main()
