#!/usr/bin/env python3
"""Can a warm bot be frozen (docker pause) and answer right after resume?

    . run/env.sh && python3 pause_test.py sup-dsh-glm 90 600
"""

import json
import subprocess
import sys
import time

import run_eval as r
import timing

QS = {q["id"]: q for q in json.load(open("questions_all.json"))}


def sandbox(*args):
    return subprocess.run(["docker", "exec", "helix-sandbox-nvidia-1", "docker", *args],
                          capture_output=True, text=True).stdout.strip()


def ask(v, sid, qid):
    r.api("POST", f"/sessions/{sid}/clear", timeout=60)
    time.sleep(2)
    t0 = timing.now()
    res = r.run_question(v, sid, QS[qid], 600)
    t1 = timing.now()
    b = timing.turn_breakdown(sid, t0, t1)
    print(f"  {qid:<22} {'PASS' if res['correct'] else 'FAIL'} wall={b['wall']}s llm={b.get('llm')} "
          f"tools={b.get('tools_harness')} calls={b.get('llm_calls')} err={res['error']}", flush=True)
    return b


def main():
    bot, pauses = sys.argv[1], [int(x) for x in sys.argv[2:]]
    v = [x for x in json.load(open("variants.json")) if x["id"] == bot][0]
    v = {**v, "prompt": "prompt_playbook.md", "sandbox_runtime": "headless-ubuntu", "keep_running": True}
    r.ensure_bot(v)
    t = time.time()
    sid = r.fresh_session(v)
    print(f"ready {sid} in {time.time() - t:.0f}s", flush=True)
    name = timing.container_for(sid)
    print("warm-up:")
    ask(v, sid, "q1_crm_owner")
    for qid in ("q3_billing_balance", "q1_crm_owner", "q5_helpdesk_history"):
        ask(v, sid, qid)
    print("memory:", sandbox("stats", "--no-stream", "--format", "{{.MemUsage}} cpu={{.CPUPerc}}", name))
    for secs, qid in zip(pauses, ("q3_billing_balance", "q5_helpdesk_history", "q1_crm_owner")):
        sandbox("pause", name)
        print(f"paused {secs}s; state={sandbox('inspect', '-f', '{{.State.Status}}', name)}", flush=True)
        time.sleep(secs)
        t = time.time()
        sandbox("unpause", name)
        print(f"unpaused in {time.time() - t:.2f}s", flush=True)
        ask(v, sid, qid)
    r.api("POST", f"/orgs/{r.ORG}/bots/{bot}/stop", timeout=60)


if __name__ == "__main__":
    main()
