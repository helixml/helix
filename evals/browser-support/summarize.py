#!/usr/bin/env python3
"""Summarize results.jsonl into markdown tables.

    python3 summarize.py run/results.jsonl [--tag baseline --tag skills]
"""

import argparse
import json
import statistics
from collections import defaultdict


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("path")
    ap.add_argument("--tag", action="append")
    ap.add_argument("--per-question", action="store_true")
    a = ap.parse_args()
    rows = [json.loads(l) for l in open(a.path) if l.strip()]
    if a.tag:
        rows = [r for r in rows if r.get("tag") in a.tag]
    groups = defaultdict(list)
    for r in rows:
        groups[(r.get("tag"), r["variant"])].append(r)

    print("| tag | variant | harness | model | pass | median s | total s | tool calls | LLM calls | prompt tok (M) | non-chrome hits |")
    print("|---|---|---|---|---|---|---|---|---|---|---|")
    for (tag, variant), rs in sorted(groups.items(), key=lambda kv: (kv[0][0], -sum(r["correct"] for r in kv[1]), sum(r["seconds"] for r in kv[1]))):
        passed = sum(r["correct"] for r in rs)
        secs = [r["seconds"] for r in rs]
        other = sum(n for r in rs for k, n in r["traffic"]["ua"].items() if k != "chrome")
        print(f"| {tag} | {variant} | {rs[0]['runtime']} | {rs[0]['model']} | {passed}/{len(rs)} | "
              f"{statistics.median(secs):.0f} | {sum(secs):.0f} | {sum(r['tool_calls'] for r in rs)} | "
              f"{sum(r['usage']['calls'] for r in rs)} | {sum(r['usage']['prompt'] for r in rs) / 1e6:.2f} | {other} |")

    if a.per_question:
        print()
        qids = sorted({r["qid"] for r in rows})
        print("| tag | variant | " + " | ".join(qids) + " |")
        print("|---|---|" + "---|" * len(qids))
        for (tag, variant), rs in sorted(groups.items()):
            by = {r["qid"]: r for r in rs}
            cells = []
            for q in qids:
                r = by.get(q)
                cells.append("—" if not r else f"{'✅' if r['correct'] else '❌'} {r['seconds']:.0f}s")
            print(f"| {tag} | {variant} | " + " | ".join(cells) + " |")


if __name__ == "__main__":
    main()
