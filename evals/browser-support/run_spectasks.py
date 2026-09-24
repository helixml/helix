#!/usr/bin/env python3
"""Answer support questions with one just-do-it spec task per question and
record where the time goes: sandbox launch milestones plus the turn split.

    . run/env.sh
    python3 run_spectasks.py --variants variants_top3.json --qids q1_crm_owner ... \
        --runtime headless-ubuntu --tag st-headless
"""

import argparse
import json
import os
import threading
import time
from datetime import datetime, timezone

import run_eval as r
import timing

HERE = os.path.dirname(os.path.abspath(__file__))
PROJECT = os.environ.get("EVAL_SPECTASK_PROJECT", "prj_01m38z89xvc20jh59f5gh7y26n")

TASK_PREAMBLE = """This task is a customer-support lookup, not a code change. Do not edit, commit or push anything and do not open a pull request. Use the browser to find the answer, then reply with it.

"""


def task_prompt(v, q):
    base = r.bot_base("st-" + v["id"].removeprefix("sup-"))
    playbook = open(os.path.join(HERE, "prompt_playbook.md")).read().replace("{{BASE}}", base)
    return f"{TASK_PREAMBLE}{playbook}\n\n## Question\n\n{q['question']}\n"


def interaction(session_id):
    return r.sql_json(f"""SELECT json_build_object('id', id, 'state', state, 'error', error,
        'created', extract(epoch from created), 'completed', extract(epoch from completed),
        'entries', response_entries) FROM interactions WHERE session_id='{session_id}'
        ORDER BY created LIMIT 1""")


def run_one(v, q, runtime, tag, out_path, timeout):
    cfg = {"runtime": v["runtime"], "credential_type": "api_key", "provider_ref": v["provider"], "model": v["model"]}
    task = r.api("POST", "/spec-tasks/from-prompt", {
        "project_id": PROJECT, "name": f"{v['id']} {q['id']}", "prompt": task_prompt(v, q),
        "just_do_it_mode": True, "code_agent_config": cfg, "sandbox_runtime": runtime})
    t_req = timing.now()
    r.api("POST", f"/spec-tasks/{task['id']}/start-planning", timeout=120)
    sid, row, deadline = None, None, t_req + timeout
    while time.time() < deadline:
        time.sleep(3)
        if not sid:
            sid = (r.api("GET", f"/spec-tasks/{task['id']}") or {}).get("planning_session_id")
            continue
        row = interaction(sid)
        if row and row["state"] in ("complete", "error"):
            break
    t_end = timing.now()
    events = {"request": t_req}
    if sid:
        events["session_created"] = r.sql_json(
            f"SELECT to_json(extract(epoch from created)) FROM sessions WHERE id='{sid}'")
        name = timing.container_for(sid)
        if name:
            events.update(timing.container_timeline(name))
        calls = timing.llm_calls(sid)
        if calls:
            events["first_llm"] = calls[0]["start"]
    done = (row or {}).get("completed") if (row or {}).get("state") in ("complete", "error") else None
    events["answered"] = done
    turn = timing.turn_breakdown(sid, events.get("first_llm", t_req), done or t_end) if sid else {}
    answer = r.final_answer((row or {}).get("entries"))
    ok, groups = r.grade(answer, q["must"])
    rec = {
        "tag": tag, "kind": "spectask", "runtime_env": runtime, "variant": v["id"], "runtime": v["runtime"],
        "model": v["model"], "qid": q["id"], "correct": ok, "groups": groups, "task": task["id"], "session": sid,
        "state": (row or {}).get("state"), "error": (row or {}).get("error"), "answer": answer[:1500],
        "events": {k: round(val - t_req, 1) for k, val in events.items() if isinstance(val, (int, float))},
        "turn": turn, "total": round((done or t_end) - t_req, 1),
        "ts": datetime.now(timezone.utc).isoformat(),
    }
    with r.PRINT_LOCK, open(out_path, "a") as f:
        f.write(json.dumps(rec) + "\n")
    e = rec["events"]
    r.log(f"{v['id']:<20} {q['id']:<22} {'PASS' if ok else 'FAIL'} total={rec['total']:>6}s "
          f"llm_first=+{e.get('first_llm', '?')}s turn={turn.get('wall', '?')}s")
    release(task["id"])


def release(task_id):
    """Stop the sandbox and archive the task: a stopped task still occupies an
    implementation WIP slot, so later tasks would queue behind it."""
    for method, path, body in (("POST", f"/spec-tasks/{task_id}/stop-agent", None),
                               ("PATCH", f"/spec-tasks/{task_id}/archive", {"archived": True})):
        try:
            r.api(method, path, body, timeout=60)
        except Exception as ex:  # noqa: BLE001
            r.log("release failed", task_id, method, path, ex)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--variants", default=os.path.join(HERE, "variants_top3.json"))
    ap.add_argument("--questions", default=os.path.join(HERE, "questions_all.json"))
    ap.add_argument("--qids", nargs="*")
    ap.add_argument("--runtime", default="ubuntu-desktop")
    ap.add_argument("--tag", default="spectask")
    ap.add_argument("--parallel", type=int, default=3)
    ap.add_argument("--timeout", type=int, default=1200)
    ap.add_argument("--out", default=os.path.join(HERE, "run", "timing.jsonl"))
    a = ap.parse_args()
    variants = json.load(open(a.variants))
    questions = [q for q in json.load(open(a.questions)) if not a.qids or q["id"] in a.qids]
    sem = threading.Semaphore(a.parallel)

    def worker(v, q):
        with sem:
            try:
                run_one(v, q, a.runtime, a.tag, a.out, a.timeout)
            except Exception as ex:  # noqa: BLE001
                r.log(v["id"], q["id"], "FAILED:", ex)

    threads = []
    for q in questions:
        for v in variants:
            th = threading.Thread(target=worker, args=(v, q))
            th.start()
            threads.append(th)
            time.sleep(2)
    for th in threads:
        th.join()


if __name__ == "__main__":
    main()
