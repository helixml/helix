#!/usr/bin/env python3
"""Run browser-support evals against Helix org bots.

Each variant is one org bot (harness x model x prompt). For every variant the
bot is (re)started on a fresh session, then each question is sent as its own
turn, with the thread cleared between questions. Answers are graded against
questions.json; timing, LLM usage and mock-system traffic are recorded.

    . run/env.sh
    python3 run_eval.py --variants variants.json --only sup-opencode-glm --tag baseline
    python3 run_eval.py --variants variants_top.json --instance --tag instance   # on bot instances
"""

import argparse
import json
import os
import re
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone

HERE = os.path.dirname(os.path.abspath(__file__))
API = os.environ.get("HELIX_URL", "http://localhost:8080").rstrip("/") + "/api/v1"
KEY = os.environ["HELIX_API_KEY"]
ORG = os.environ.get("EVAL_ORG", "unmanned-org")
PRINT_LOCK = threading.Lock()


def log(*a):
    with PRINT_LOCK:
        print(datetime.now().strftime("%H:%M:%S"), *a, flush=True)


def api(method, path, body=None, timeout=60):
    req = urllib.request.Request(API + path, method=method,
                                 data=json.dumps(body).encode() if body is not None else None,
                                 headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read()
            return json.loads(raw) if raw.strip() else None
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {path}: HTTP {e.code}: {e.read()[:300]!r}") from None


def sql(q):
    out = subprocess.run(["docker", "exec", "helix-postgres-1", "psql", "-U", "postgres", "-d", "postgres",
                          "-tAc", q], capture_output=True, text=True, check=True).stdout.strip()
    return out


def sql_json(q):
    out = sql(q)
    return json.loads(out) if out else None


def bot_base(bot_id):
    return open(os.path.join(HERE, "run", "url.txt")).read().strip() + "/b/" + bot_id


def render_prompt(variant):
    base = bot_base(variant["id"])
    text = open(os.path.join(HERE, variant.get("prompt", "prompt_baseline.md"))).read()
    return text.replace("{{BASE}}", base)


def ensure_bot(v):
    body = {"name": v.get("name", v["id"]), "content": render_prompt(v), "code_agent_runtime": v["runtime"],
            "provider": v["provider"], "model": v["model"], "preserve_context": True}
    for k in ("reasoning_effort", "sandbox_runtime"):
        if v.get(k):
            body[k] = v[k]
    try:
        api("GET", f"/orgs/{ORG}/bots/{v['id']}")
        api("PATCH", f"/orgs/{ORG}/bots/{v['id']}", body)
    except RuntimeError as e:
        if "HTTP 404" not in str(e):
            raise
        api("POST", f"/orgs/{ORG}/bots", {"id": v["id"], **body})


def fresh_session(v, timeout=600):
    """Restart the bot and wait until its activation turn has finished."""
    bot = api("GET", f"/orgs/{ORG}/bots/{v['id']}")
    since = datetime.now(timezone.utc).isoformat()
    verb = "restart" if bot.get("project_id") else "activate"
    api("POST", f"/orgs/{ORG}/bots/{v['id']}/{verb}", timeout=120)
    deadline = time.time() + timeout
    while time.time() < deadline:
        time.sleep(5)
        bot = api("GET", f"/orgs/{ORG}/bots/{v['id']}")
        if not bot.get("project_id"):
            continue
        try:
            sid = (api("GET", f"/projects/{bot['project_id']}/exploratory-session", timeout=15) or {}).get("id")
        except RuntimeError:
            continue
        if sid and sql(f"""SELECT count(*) FROM interactions WHERE session_id='{sid}'
                AND updated > '{since}' AND state IN ('complete','error')""") not in ("", "0"):
            return sid
    raise RuntimeError(f"{v['id']}: no ready session after {timeout}s")


def fresh_instance(v, tag, timeout=600):
    """Start a new instance of the bot and warm it with one trivial turn.

    An instance has no activation turn, so the warm-up stands in for the
    bot's: it brings the harness and MCP servers up before the first
    measured question.
    """
    body = {"name": f"eval {tag}"}
    if v.get("sandbox_runtime"):
        body["sandbox_runtime"] = v["sandbox_runtime"]
    sid = api("POST", f"/orgs/{ORG}/bots/{v['id']}/instances", body)["session_id"]
    api("POST", "/sessions/chat", {"session_id": sid, "type": "text", "stream": False,
                                   "messages": [{"role": "user", "content": {"content_type": "text", "parts": ["Reply with OK."]}}]},
        timeout=timeout)
    return sid


def final_answer(entries):
    texts = [e.get("content", "") for e in entries or [] if e.get("type") == "text"]
    ans = texts[-1] if texts else ""
    ans = re.sub(r"<thinking>.*?</thinking>", "", ans, flags=re.S).strip()
    return ans


def grade(answer, must):
    norm = answer.lower().replace(" ", " ")
    groups = []
    for alts in must:
        ok = False
        for a in alts:
            if a.startswith("re:"):
                ok = ok or re.search(a[3:], norm) is not None
            else:
                ok = ok or a.lower() in norm
        groups.append(ok)
    return all(groups), groups


def access_window(bot, t0, t1):
    path = os.path.join(HERE, "run", "access.log")
    hits, uas = 0, {}
    if not os.path.exists(path):
        return {"hits": 0, "ua": {}}
    with open(path) as f:
        for line in f:
            r = json.loads(line)
            if r.get("bot") == bot and t0 <= r["ts"] <= t1:
                hits += 1
                ua = r["ua"]
                kind = "chrome" if "Chrome" in ua else ("curl" if "curl" in ua else (ua.split("/")[0] or "none"))
                uas[kind] = uas.get(kind, 0) + 1
    return {"hits": hits, "ua": uas}


def run_question(v, sid, q, timeout):
    t0 = time.time()
    ts0 = datetime.now(timezone.utc).isoformat()
    err = None
    try:
        api("POST", "/sessions/chat", {"session_id": sid, "type": "text", "stream": False,
                                       "messages": [{"role": "user", "content": {"content_type": "text", "parts": [q["question"]]}}]},
            timeout=timeout)
    except Exception as e:  # noqa: BLE001 - record and continue
        err = str(e)[:300]
    t1 = time.time()
    ts1 = datetime.now(timezone.utc).isoformat()
    qp = q["question"].replace("'", "''")
    row = sql_json(f"""SELECT json_build_object('id', id, 'state', state, 'error', error,
        'entries', response_entries) FROM interactions WHERE session_id='{sid}'
        AND prompt_message = '{qp}' ORDER BY created DESC LIMIT 1""") or {}
    usage = sql_json(f"""SELECT json_build_object('calls', count(*), 'prompt', coalesce(sum(prompt_tokens),0),
        'completion', coalesce(sum(completion_tokens),0), 'cache_read', coalesce(sum(cache_read_tokens),0),
        'llm_ms', coalesce(sum(duration_ms),0), 'errors', count(*) FILTER (WHERE error <> ''))
        FROM llm_calls WHERE session_id='{sid}' AND created BETWEEN '{ts0}' AND '{ts1}'""")
    entries = row.get("entries") or []
    tools = {}
    for e in entries:
        if e.get("type") == "tool_call":
            n = e.get("tool_name") or "?"
            tools[n] = tools.get(n, 0) + 1
    answer = final_answer(entries)
    ok, groups = grade(answer, q["must"])
    return {
        "variant": v["id"], "runtime": v["runtime"], "model": v["model"], "prompt": v.get("prompt", "prompt_baseline.md"),
        "qid": q["id"], "difficulty": q["difficulty"], "correct": ok, "groups": groups,
        "seconds": round(t1 - t0, 1), "state": row.get("state"), "error": err or row.get("error"),
        "answer": answer[:1500], "tool_calls": sum(tools.values()), "tools": tools, "usage": usage,
        "traffic": access_window(v["id"], t0, t1), "session": sid, "interaction": row.get("id"),
    }


def run_variant(v, questions, tag, out_path, timeout, instance=False):
    ensure_bot(v)
    log(v["id"], "starting fresh " + ("instance" if instance else "session"))
    t = time.time()
    sid = fresh_instance(v, tag) if instance else fresh_session(v)
    log(v["id"], f"ready {sid} in {time.time() - t:.0f}s")
    for q in questions:
        api("POST", f"/sessions/{sid}/clear", timeout=60)
        time.sleep(2)
        r = run_question(v, sid, q, timeout)
        r["tag"] = tag
        r["mode"] = "instance" if instance else "bot"
        r["ts"] = datetime.now(timezone.utc).isoformat()
        with PRINT_LOCK, open(out_path, "a") as f:
            f.write(json.dumps(r) + "\n")
        log(f"{v['id']:<28} {q['id']:<22} {'PASS' if r['correct'] else 'FAIL'} {r['seconds']:>6}s "
            f"tools={r['tool_calls']:<3} llm={r['usage']['calls']:<3} ptok={r['usage']['prompt']:<8} "
            f"err={(r['error'] or '')[:60]}")
    if instance:
        api("DELETE", f"/orgs/{ORG}/bots/{v['id']}/instances/{sid}", timeout=120)
    elif not v.get("keep_running"):
        api("POST", f"/orgs/{ORG}/bots/{v['id']}/stop", timeout=60)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--variants", default=os.path.join(HERE, "variants.json"))
    ap.add_argument("--questions", default=os.path.join(HERE, "questions.json"))
    ap.add_argument("--only", nargs="*")
    ap.add_argument("--qids", nargs="*")
    ap.add_argument("--tag", default="run")
    ap.add_argument("--parallel", type=int, default=2)
    ap.add_argument("--timeout", type=int, default=900)
    ap.add_argument("--out", default=os.path.join(HERE, "run", "results.jsonl"))
    ap.add_argument("--instance", action="store_true", help="run each variant on a new bot instance instead of the bot's own session")
    a = ap.parse_args()
    variants = json.load(open(a.variants))
    if a.only:
        variants = [v for v in variants if v["id"] in a.only]
    questions = json.load(open(a.questions))
    if a.qids:
        questions = [q for q in questions if q["id"] in a.qids]
    sem = threading.Semaphore(a.parallel)
    threads = []

    def worker(v):
        with sem:
            try:
                run_variant(v, questions, a.tag, a.out, a.timeout, instance=a.instance)
            except Exception as e:  # noqa: BLE001
                log(v["id"], "VARIANT FAILED:", e)

    for v in variants:
        th = threading.Thread(target=worker, args=(v,))
        th.start()
        threads.append(th)
        time.sleep(3)
    for th in threads:
        th.join()


if __name__ == "__main__":
    sys.exit(main())
