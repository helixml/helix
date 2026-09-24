#!/usr/bin/env python3
"""Org-bot counterpart of run_spectasks.py: time a cold bot launch, then break
down each question turn on the warm bot.

    . run/env.sh
    python3 run_bots_timing.py --variants variants_top3.json --qids ... --tag bot-desktop
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


def run_variant(v, questions, tag, out_path, timeout):
    v = {**v, "prompt": "prompt_playbook.md"}
    r.ensure_bot(v)
    t_req = timing.now()
    sid = r.fresh_session(v)
    t_ready = timing.now()
    events = {"request": t_req, "ready": t_ready}
    events["session_created"] = r.sql_json(f"SELECT to_json(extract(epoch from created)) FROM sessions WHERE id='{sid}'")
    name = timing.container_for(sid)
    if name:
        events.update(timing.container_timeline(name))
    calls = timing.llm_calls(sid)
    if calls:
        events["first_llm"] = calls[0]["start"]
    launch = {"kind": "bot-launch", "tag": tag, "variant": v["id"], "runtime": v["runtime"], "model": v["model"],
              "runtime_env": v.get("sandbox_runtime", "ubuntu-desktop"), "session": sid,
              "events": {k: round(val - t_req, 1) for k, val in events.items() if isinstance(val, (int, float))},
              "activation_turn": timing.turn_breakdown(sid, events.get("first_llm", t_req), t_ready),
              "ts": datetime.now(timezone.utc).isoformat()}
    with r.PRINT_LOCK, open(out_path, "a") as f:
        f.write(json.dumps(launch) + "\n")
    r.log(f"{v['id']:<20} launch ready in {t_ready - t_req:.0f}s")
    for q in questions:
        r.api("POST", f"/sessions/{sid}/clear", timeout=60)
        time.sleep(2)
        t0 = timing.now()
        res = r.run_question(v, sid, q, timeout)
        t1 = timing.now()
        rec = {"kind": "bot-turn", "tag": tag, "variant": v["id"], "runtime": v["runtime"], "model": v["model"],
               "runtime_env": launch["runtime_env"], "qid": q["id"], "correct": res["correct"],
               "answer": res["answer"], "session": sid, "turn": timing.turn_breakdown(sid, t0, t1),
               "ts": datetime.now(timezone.utc).isoformat()}
        with r.PRINT_LOCK, open(out_path, "a") as f:
            f.write(json.dumps(rec) + "\n")
        r.log(f"{v['id']:<20} {q['id']:<22} {'PASS' if rec['correct'] else 'FAIL'} turn={rec['turn']['wall']}s")
    r.api("POST", f"/orgs/{r.ORG}/bots/{v['id']}/stop", timeout=60)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--variants", default=os.path.join(HERE, "variants_top3.json"))
    ap.add_argument("--questions", default=os.path.join(HERE, "questions_all.json"))
    ap.add_argument("--qids", nargs="*")
    ap.add_argument("--sandbox-runtime", default="")
    ap.add_argument("--tag", default="bot")
    ap.add_argument("--parallel", type=int, default=3)
    ap.add_argument("--timeout", type=int, default=1200)
    ap.add_argument("--out", default=os.path.join(HERE, "run", "timing.jsonl"))
    a = ap.parse_args()
    variants = json.load(open(a.variants))
    if a.sandbox_runtime:
        variants = [{**v, "sandbox_runtime": a.sandbox_runtime} for v in variants]
    questions = [q for q in json.load(open(a.questions)) if not a.qids or q["id"] in a.qids]
    sem = threading.Semaphore(a.parallel)

    def worker(v):
        with sem:
            try:
                run_variant(v, questions, a.tag, a.out, a.timeout)
            except Exception as ex:  # noqa: BLE001
                r.log(v["id"], "VARIANT FAILED:", ex)

    threads = [threading.Thread(target=worker, args=(v,)) for v in variants]
    for th in threads:
        th.start()
        time.sleep(3)
    for th in threads:
        th.join()


if __name__ == "__main__":
    main()
