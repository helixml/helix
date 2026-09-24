"""Timeline extraction for launch and per-turn time breakdowns.

Sources: docker inspect of the sandbox container (created/started), the
workspace setup log and completion sentinel, Zed.log (ACP spawn, agent_ready,
first inbound chat), and llm_calls (created = request start, duration_ms).
"""

import json
import os
import re
import subprocess
from datetime import datetime, timezone

import run_eval as r


def _sandbox(*args):
    return subprocess.run(["docker", "exec", "helix-sandbox-nvidia-1", "docker", *args],
                          capture_output=True, text=True).stdout


def _ts(s):
    """Parse RFC3339 with up to nanosecond precision (docker) or Zed's log stamp."""
    if not s:
        return None
    m = re.match(r"^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)?$", s.strip())
    if not m:
        return None
    base, frac, tz = m.groups()
    tz = "+00:00" if tz in (None, "Z") else tz
    return datetime.fromisoformat(f"{base}.{(frac or '0')[:6]}{tz}").timestamp()


def container_for(session_id):
    names = _sandbox("ps", "-a", "--format", "{{.Names}}").split()
    suffix = session_id.removeprefix("ses_")
    for n in names:
        if n.endswith(suffix):
            return n
    return None


def save_container_log(name, path):
    """Keep the container's stdout log: releasing a task removes the container."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        subprocess.run(["docker", "exec", "helix-sandbox-nvidia-1", "docker", "logs", "-t", name],
                       stdout=f, stderr=subprocess.STDOUT)


def container_timeline(name):
    """Absolute unix timestamps for launch milestones inside one container."""
    out = {}
    info = json.loads(_sandbox("inspect", name) or "[]")
    if info:
        out["container_created"] = _ts(info[0]["Created"])
        out["container_started"] = _ts(info[0]["State"]["StartedAt"])
    script = r"""
set +e
stat -c %Y ~/.helix-setup-complete 2>/dev/null | sed 's/^/setup_complete /'
head -c 300000 ~/.local/share/zed/logs/Zed.log 2>/dev/null | awk '
  NR==1 {print "zed_first_log " $1}
  /\[ACP_SPAWN\] Spawned ACP wrapper/ && !a {print "acp_spawned " $1; a=1}
  /Sending agent_ready event/ && !b {print "agent_ready " $1; b=1}
  /WEBSOCKET-IN\] Received text: \{"type":"chat_message"/ && !c {print "first_chat_in " $1; c=1}
'
"""
    res = subprocess.run(["docker", "exec", "helix-sandbox-nvidia-1", "docker", "exec", "-u", "retro", name,
                          "bash", "-lc", script], capture_output=True, text=True).stdout
    for line in res.splitlines():
        k, _, v = line.partition(" ")
        if k == "setup_complete":
            out[k] = float(v)
        elif v:
            out[k] = _ts(v)
    return out


def llm_calls(session_id):
    rows = r.sql_json(f"""SELECT coalesce(json_agg(json_build_object(
        'start', extract(epoch from created), 'ms', duration_ms, 'ttft', time_to_first_token_ms,
        'prompt', prompt_tokens, 'completion', completion_tokens) ORDER BY created), '[]')
        FROM llm_calls WHERE session_id='{session_id}'""") or []
    for c in rows:
        c["end"] = c["start"] + (c["ms"] or 0) / 1000
    return rows


def turn_breakdown(session_id, t0, t1):
    """Split one turn's wall time into dispatch, LLM, tools/harness and finish."""
    calls = [c for c in llm_calls(session_id) if t0 - 1 <= c["start"] <= t1]
    if not calls:
        return {"wall": round(t1 - t0, 1), "llm_calls": 0}
    # Union of LLM intervals: harnesses fire side calls (titles, summaries) in parallel.
    spans = sorted((c["start"], c["end"]) for c in calls)
    busy, cur_s, cur_e = 0.0, *spans[0]
    for s, e in spans[1:]:
        if s > cur_e:
            busy += cur_e - cur_s
            cur_s, cur_e = s, e
        else:
            cur_e = max(cur_e, e)
    busy += cur_e - cur_s
    first, last = spans[0][0], max(e for _, e in spans)
    ttft = sum((c["ttft"] or 0) for c in calls) / 1000
    return {
        "wall": round(t1 - t0, 1),
        "dispatch": round(first - t0, 1),          # chat accepted -> first LLM request
        "llm": round(busy, 1),                    # time an LLM request was in flight
        "llm_prefill": round(min(ttft, busy), 1),  # sum of time-to-first-token
        "tools_harness": round(last - first - busy, 1),  # between LLM calls: tool exec + harness
        "finish": round(t1 - last, 1),            # last LLM response -> turn returned
        "llm_calls": len(calls),
        "prompt_tokens": sum(c["prompt"] or 0 for c in calls),
        "completion_tokens": sum(c["completion"] or 0 for c in calls),
    }


def now():
    return datetime.now(timezone.utc).timestamp()
