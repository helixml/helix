#!/usr/bin/env python3
"""Deterministic mtimes for freshly cloned Helix workspace repos.

A clone gives every file the clone time, so mtime-based build tools (cargo,
make, ...) see every source as newer than the outputs cached in a golden
snapshot and rebuild everything. `restore` sets each tracked file's mtime to the
committer time of the last commit that touched it (one `git log` walk from
HEAD, first sighting wins; paths never reached get the oldest walked commit's
time), so two clones of the same commit get identical mtimes.

Commit times alone are not safe against a cache: a commit merged after the
golden build can carry a committer time from before it, and a file with that
time would look older than the stale output built from its previous content.
So a golden build `record`s each repo's HEAD, its dirty tracked files and the
time its build finished. `restore --golden` then raises every file that differs
from the golden checkout to at least one second after that time; only files
identical to what the golden compiled keep their (older) commit time.

Untracked files, submodules and files with working-tree changes are never
touched. Failures are reported and never fatal.

    helix-git-mtime restore [--golden RECORD] REPO...
    helix-git-mtime record OUTPUT REPO...
"""

import argparse
import json
import os
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor

GIT = ["git", "-c", "log.showSignature=false", "-c", "core.quotePath=false"]


def git(repo, *args):
    return subprocess.run(
        GIT + list(args), cwd=repo, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE
    ).stdout


def nul_split(out):
    return [p for p in out.split(b"\0") if p]


def tracked_files(repo):
    files = set()
    for entry in nul_split(git(repo, "ls-files", "-z", "--stage")):
        meta, path = entry.split(b"\t", 1)
        if not meta.startswith(b"160000"):  # submodule
            files.add(path)
    return files


def dirty_files(repo):
    return set(nul_split(git(repo, "diff", "--name-only", "-z", "--no-renames", "HEAD")))


def last_commit_times(repo, pending):
    """Walk history from HEAD once. Returns ({path: ct}, oldest ct, commits)."""
    pending = set(pending)
    times = {}
    oldest = None
    commits = 0
    proc = subprocess.Popen(
        GIT + ["log", "-z", "--no-renames", "--name-only", "--format=%x01%ct", "HEAD"],
        cwd=repo, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
    )
    buf = b""
    try:
        while pending:
            chunk = proc.stdout.read(1 << 20)
            if not chunk:
                break
            tokens = (buf + chunk).split(b"\0")
            buf = tokens.pop()
            for tok in tokens:
                if tok.startswith(b"\x01"):
                    oldest = int(tok[1:])
                    commits += 1
                    continue
                # The file list of a commit starts with a newline.
                path = tok[1:] if tok.startswith(b"\n") else tok
                if path in pending:
                    pending.discard(path)
                    times[path] = oldest
    finally:
        proc.kill()
        proc.wait()
    if oldest is None:
        raise RuntimeError("git log returned no commits")
    return times, oldest, commits


def restore_repo(repo, golden):
    start = time.monotonic()
    name = os.path.basename(os.path.abspath(repo))
    head = git(repo, "rev-parse", "HEAD").strip().decode()
    files = tracked_files(repo) - dirty_files(repo)
    times, oldest, commits = last_commit_times(repo, files)

    changed = set()
    floor = None
    note = ""
    if golden is not None:
        floor = int(golden["built_at"]) + 1
        rec = golden.get("repos", {}).get(name)
        base = rec.get("head") if rec else None
        try:
            if not base:
                raise ValueError("repo not in golden record")
            git(repo, "cat-file", "-e", base + "^{commit}")
            changed = set(nul_split(git(repo, "diff", "--name-only", "-z", "--no-renames", base, head)))
            changed |= {os.fsencode(p) for p in rec.get("dirty", [])}
            note = f", {len(changed)} differ from golden {base[:10]}"
        except (subprocess.CalledProcessError, ValueError) as e:
            changed = files
            note = f", golden checkout unusable ({e.__class__.__name__}), all files after golden"

    root = os.fsencode(os.path.abspath(repo)) + b"/"
    errors = 0
    # Directories get the newest time of the tracked files beneath them. A
    # clone leaves every directory at the clone time, and build tools that
    # watch a directory (cargo's rerun-if-changed=<dir> takes the newest mtime
    # of the directory and everything in it) then see a change on every fresh
    # clone. Deriving directory times from file times keeps them identical
    # across clones of the same commit and still newer when a file below
    # changed.
    dir_times = {}
    for path in files:
        t = times.get(path, oldest)
        if floor is not None and path in changed and t < floor:
            t = floor
        try:
            os.utime(root + path, (t, t), follow_symlinks=False)
        except OSError:
            errors += 1
        parent = os.path.dirname(path)
        while True:
            if dir_times.get(parent, -1) >= t:
                break
            dir_times[parent] = t
            if not parent:
                break
            parent = os.path.dirname(parent)
    for d, t in dir_times.items():
        try:
            os.utime(root + d if d else root, (t, t), follow_symlinks=False)
        except OSError:
            errors += 1

    # Re-stat the index so `git status` stays clean and fast.
    subprocess.run(GIT + ["update-index", "--refresh", "-q"], cwd=repo,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return (f"{name}: {len(files)} files, {len(dir_times)} dirs, {commits} commits walked, "
            f"{len(files) - len(times)} unseen{note}"
            + (f", {errors} utime errors" if errors else "")
            + f", {time.monotonic() - start:.1f}s")


def cmd_restore(args):
    golden = None
    if args.golden:
        with open(args.golden) as f:
            golden = json.load(f)
        if not isinstance(golden.get("built_at"), int):
            sys.exit("  mtime: golden record has no built_at; keeping clone mtimes")

    def run(repo):
        try:
            return "  mtime: " + restore_repo(repo, golden)
        except Exception as e:  # never fail workspace setup
            return f"  mtime: {repo}: skipped ({e})"

    with ThreadPoolExecutor(max_workers=max(1, len(args.repos))) as pool:
        for line in pool.map(run, args.repos):
            print(line, flush=True)


def cmd_record(args):
    record = {"version": 1, "built_at": int(time.time()), "repos": {}}
    for repo in args.repos:
        try:
            head = git(repo, "rev-parse", "HEAD").strip().decode()
            dirty = sorted(os.fsdecode(p) for p in dirty_files(repo))
        except Exception as e:
            print(f"  mtime: {repo}: not recorded ({e})", file=sys.stderr)
            continue
        record["repos"][os.path.basename(os.path.abspath(repo))] = {"head": head, "dirty": dirty}
    data = json.dumps(record, indent=1, sort_keys=True)
    if args.output == "-":
        print(data)
    else:
        with open(args.output, "w") as f:
            f.write(data + "\n")


def main():
    p = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    sub = p.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("restore")
    r.add_argument("--golden", help="record written by `record` in the golden build")
    r.add_argument("repos", nargs="+")
    r.set_defaults(func=cmd_restore)
    w = sub.add_parser("record")
    w.add_argument("output", help="file to write, or - for stdout")
    w.add_argument("repos", nargs="+")
    w.set_defaults(func=cmd_record)
    args = p.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
