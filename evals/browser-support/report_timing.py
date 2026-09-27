#!/usr/bin/env python3
"""Summarise run/timing.jsonl: launch phases and per-turn time split.

    python3 report_timing.py run/timing.jsonl
"""

import json
import statistics as st
import sys
from collections import defaultdict

PHASES = [  # (label, from event, to event)
    ("schedule + create container", "request", "container_started"),
    ("workspace setup (clone, skills, config)", "container_started", "setup_complete"),
    ("Zed start → first chat delivered", "setup_complete", "first_chat_in"),
    ("harness + MCP start → first LLM call", "first_chat_in", "first_llm"),
]


def med(xs):
    xs = [x for x in xs if x is not None]
    return f"{st.median(xs):.1f}" if xs else "—"


def main():
    rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
    rows = [x for x in rows if not x["tag"].endswith("smoke")]

    launches = [x for x in rows if x.get("kind") in ("spectask", "bot-launch")]
    by = defaultdict(list)
    for x in launches:
        by[x["tag"]].append(x)
    print("## Launch (median seconds from start request)\n")
    tags = sorted(by)
    print("| phase | " + " | ".join(tags) + " |")
    print("|---|" + "---|" * len(tags))
    for label, a, b in PHASES:
        cells = []
        for t in tags:
            cells.append(med([x["events"].get(b, None) - x["events"].get(a, 0)
                              if x["events"].get(b) is not None and x["events"].get(a) is not None else None
                              for x in by[t]]))
        print(f"| {label} | " + " | ".join(cells) + " |")
    print("| **request → first LLM call** | " + " | ".join(med([x["events"].get("first_llm") for x in by[t]]) for t in tags) + " |")
    print("| bot activation turn (LLM, bots only) | " + " | ".join(
        med([x.get("activation_turn", {}).get("wall") for x in by[t]]) if by[t][0]["kind"] == "bot-launch" else "—" for t in tags) + " |")
    print("| **request → ready for a question** | " + " | ".join(
        med([x["events"].get("ready") if x["kind"] == "bot-launch" else x["events"].get("first_llm") for x in by[t]]) for t in tags) + " |")
    print(f"| samples | " + " | ".join(str(len(by[t])) for t in tags) + " |")

    turns = [x for x in rows if x.get("kind") in ("spectask", "bot-turn") and x.get("turn", {}).get("llm_calls")]
    print("\n## Question turn split (median seconds per question)\n")
    grp = defaultdict(list)
    for x in turns:
        grp[(x["tag"], x["variant"])].append(x)
    print("| tag | variant | pass | wall | LLM prefill | LLM decode | tools + harness | dispatch + finish | LLM calls | prompt tok (k) |")
    print("|---|---|---|---|---|---|---|---|---|---|")
    for (tag, variant), xs in sorted(grp.items()):
        t = [x["turn"] for x in xs]
        print(f"| {tag} | {variant} | {sum(x['correct'] for x in xs)}/{len(xs)} | {med([y['wall'] for y in t])} | "
              f"{med([y['llm_prefill'] for y in t])} | {med([y['llm'] - y['llm_prefill'] for y in t])} | "
              f"{med([y['tools_harness'] for y in t])} | {med([y['dispatch'] + y['finish'] for y in t])} | "
              f"{med([y['llm_calls'] for y in t])} | {med([y['prompt_tokens'] / 1000 for y in t])} |")

    print("\n## Share of total turn time (sum over all questions)\n")
    agg = defaultdict(lambda: defaultdict(float))
    for x in turns:
        k = x["tag"]
        y = x["turn"]
        agg[k]["wall"] += y["wall"]
        agg[k]["prefill"] += y["llm_prefill"]
        agg[k]["decode"] += y["llm"] - y["llm_prefill"]
        agg[k]["tools"] += y["tools_harness"]
        agg[k]["overhead"] += y["dispatch"] + y["finish"]
    print("| tag | LLM prefill | LLM decode | tools + harness | dispatch + finish |")
    print("|---|---|---|---|---|")
    for k, a in sorted(agg.items()):
        w = a["wall"] or 1
        print(f"| {k} | {a['prefill'] / w:.0%} | {a['decode'] / w:.0%} | {a['tools'] / w:.0%} | {a['overhead'] / w:.0%} |")

    print("\n## Time to answer one question (median seconds)\n")
    print("| tag | cold (launch + question) | warm (question only) |")
    print("|---|---|---|")
    for t in tags:
        if by[t][0]["kind"] == "spectask":
            print(f"| {t} | {med([x['total'] for x in by[t]])} | n/a — every question launches a sandbox |")
        else:
            ready = st.median([x["events"]["ready"] for x in by[t]])
            walls = [x["turn"]["wall"] for x in turns if x["tag"] == t]
            print(f"| {t} | {ready + st.median(walls):.1f} | {med(walls)} |")


if __name__ == "__main__" and len(sys.argv) == 2:
    main()


def first_call_prompt(session_id, t0):
    import run_eval as r
    v = r.sql(f"""SELECT prompt_tokens FROM llm_calls WHERE session_id='{session_id}'
        AND extract(epoch from created) >= {t0 - 1} AND prompt_tokens > 1000 ORDER BY created LIMIT 1""")
    return int(v) if v else None


def fixed_overhead(path):
    """Prompt tokens of each turn's first real LLM call = system prompt + tools + question."""
    from datetime import datetime
    rows = [json.loads(l) for l in open(path) if l.strip()]
    grp = defaultdict(list)
    for x in rows:
        if x.get("kind") not in ("spectask", "bot-turn") or not x.get("session"):
            continue
        end = datetime.fromisoformat(x["ts"]).timestamp()
        t0 = end - (x["turn"].get("wall") or 0) - (x["turn"].get("dispatch") or 0)
        v = first_call_prompt(x["session"], t0 if x["kind"] == "bot-turn" else end - x["total"])
        if v:
            grp[(x["tag"], x["variant"])].append(v)
    print("\n## Fixed prompt per LLM call (first call of each turn, median tokens)\n")
    print("| tag | variant | tokens |")
    print("|---|---|---|")
    for k, vs in sorted(grp.items()):
        print(f"| {k[0]} | {k[1]} | {st.median(vs):,.0f} |")


if __name__ == "__main__" and len(sys.argv) > 2 and sys.argv[2] == "--overhead":
    fixed_overhead(sys.argv[1])
