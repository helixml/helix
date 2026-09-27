#!/usr/bin/env python3
"""Compare two tags over the questions both ran, per variant.

    python3 compare.py run/results.jsonl baseline playbook
"""

import json
import statistics
import sys
from collections import defaultdict


def main():
    path, a, b = sys.argv[1:4]
    rows = [json.loads(l) for l in open(path) if l.strip()]
    by = defaultdict(dict)
    for r in rows:
        if r.get("tag") in (a, b):
            by[(r["variant"], r["qid"])][r["tag"]] = r
    per = defaultdict(list)
    for (variant, _), d in by.items():
        if a in d and b in d:
            per[variant].append((d[a], d[b]))
    print(f"| variant | n | pass {a} → {b} | total s {a} → {b} | median s | tool calls | prompt tok (M) |")
    print("|---|---|---|---|---|---|---|")
    tot = defaultdict(float)
    for variant, pairs in sorted(per.items()):
        pa = sum(x["correct"] for x, _ in pairs)
        pb = sum(y["correct"] for _, y in pairs)
        sa = sum(x["seconds"] for x, _ in pairs)
        sb = sum(y["seconds"] for _, y in pairs)
        ma = statistics.median(x["seconds"] for x, _ in pairs)
        mb = statistics.median(y["seconds"] for _, y in pairs)
        ta = sum(x["tool_calls"] for x, _ in pairs)
        tb = sum(y["tool_calls"] for _, y in pairs)
        ka = sum(x["usage"]["prompt"] for x, _ in pairs) / 1e6
        kb = sum(y["usage"]["prompt"] for _, y in pairs) / 1e6
        for k, v in (("sa", sa), ("sb", sb), ("ka", ka), ("kb", kb), ("pa", pa), ("pb", pb), ("n", len(pairs))):
            tot[k] += v
        print(f"| {variant} | {len(pairs)} | {pa} → {pb} | {sa:.0f} → {sb:.0f} ({(sb / sa - 1) * 100:+.0f}%) | "
              f"{ma:.0f} → {mb:.0f} | {ta} → {tb} | {ka:.2f} → {kb:.2f} ({(kb / ka - 1) * 100:+.0f}%) |")
    if tot["n"]:
        print(f"| **all** | {tot['n']:.0f} | {tot['pa']:.0f} → {tot['pb']:.0f} | {tot['sa']:.0f} → {tot['sb']:.0f} "
              f"({(tot['sb'] / tot['sa'] - 1) * 100:+.0f}%) | | | {tot['ka']:.1f} → {tot['kb']:.1f} "
              f"({(tot['kb'] / tot['ka'] - 1) * 100:+.0f}%) |")


if __name__ == "__main__":
    main()
