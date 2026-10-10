#!/usr/bin/env python3
"""Print a one-line JSON summary of the local dockerd's BuildKit cache.

Golden builds record it before the snapshot and sessions log it after boot,
so cache lost across golden promotion shows up in logs.

Record count and total size come from the Engine API (`docker buildx du`).
Cache mount (RUN --mount=type=cache, exec.cachemount) sizes do not: BuildKit
measures a mutable snapshot once and caches that size forever, so later builds
writing into the mount never change it. Each cache mount is therefore resolved
to its snapshot directory through BuildKit's own metadata and measured on disk:

    record ID --metadata_v2.db _main/<id>/cache.snapshot--> snapshot key
    snapshot key --snapshots.db <key>/committed (else the key itself)--> graphdriver ID
    graphdriver ID --> <docker root>/overlay2/<ID>/diff

(the mapping of moby's builder-next snapshot adapter). Must run as root.
"""

import datetime
import http.client
import json
import mmap
import os
import socket
import struct
import sys

DOCKER_SOCK = "/var/run/docker.sock"
TOP_MOUNTS = 10


class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost")
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(self.path)


def engine_get(path):
    conn = UnixHTTPConnection(DOCKER_SOCK)
    conn.request("GET", path)
    resp = conn.getresponse()
    body = resp.read()
    if resp.status != 200:
        raise RuntimeError(f"GET {path}: HTTP {resp.status}")
    return json.loads(body)


class Bolt:
    """Minimal read-only bbolt reader.

    Reads without the file lock (dockerd holds it); bbolt never overwrites
    pages reachable from the newest committed meta page, so this sees the last
    committed transaction.
    """

    PAGE_HEADER = 16
    BRANCH, LEAF = 0x01, 0x02
    BUCKET_LEAF = 0x01

    def __init__(self, path):
        with open(path, "rb") as f:
            self.buf = mmap.mmap(f.fileno(), 0, access=mmap.ACCESS_READ)
        self.page_size, self.root = self._meta()

    def _meta(self):
        best = None
        page_size = struct.unpack_from("<I", self.buf, self.PAGE_HEADER + 8)[0]
        for off in (0, page_size, 4096):
            m = off + self.PAGE_HEADER
            if m + 64 > len(self.buf):
                continue
            magic, _, psize, _, root, _, _, _, txid, checksum = struct.unpack_from("<IIIIQQQQQQ", self.buf, m)
            if magic != 0xED0CDAED or checksum != fnv64a(self.buf[m:m + 56]):
                continue
            if best is None or txid > best[0]:
                best = (txid, psize, root)
        if best is None:
            raise RuntimeError("no valid bbolt meta page")
        return best[1], best[2]

    def _items(self, buf, off):
        _, flags, count, _ = struct.unpack_from("<QHHI", buf, off)
        base = off + self.PAGE_HEADER
        if flags & self.BRANCH:
            for i in range(count):
                _, _, pgid = struct.unpack_from("<IIQ", buf, base + i * 16)
                yield from self._items(self.buf, pgid * self.page_size)
        elif flags & self.LEAF:
            for i in range(count):
                e = base + i * 16
                eflags, pos, ksize, vsize = struct.unpack_from("<IIII", buf, e)
                k = bytes(buf[e + pos:e + pos + ksize])
                v = bytes(buf[e + pos + ksize:e + pos + ksize + vsize])
                yield k, v, bool(eflags & self.BUCKET_LEAF)

    def _bucket_items(self, value):
        root = struct.unpack_from("<Q", value)[0]
        if root == 0:  # inline bucket: its page follows the header
            return self._items(value, 16)
        return self._items(self.buf, root * self.page_size)

    def children(self, value=None):
        """{key: (value, is_bucket)} of the root, or of the bucket whose
        header is value."""
        items = self._bucket_items(value) if value is not None else self._items(self.buf, self.root * self.page_size)
        return {k: (v, b) for k, v, b in items}

    def sub(self, children, key):
        """Key/values of the sub-bucket key in children, or None."""
        v = children.get(key)
        if not v or not v[1]:
            return None
        return {k: val for k, (val, b) in self.children(v[0]).items() if not b}


def fnv64a(data):
    h = 0xCBF29CE484222325
    for byte in data:
        h = ((h ^ byte) * 0x100000001B3) & 0xFFFFFFFFFFFFFFFF
    return h


def disk_usage(path):
    """Apparent bytes of everything but directories, hard links counted once
    (what `du -sb` reports), and the newest mtime."""
    total, newest, seen = 0, None, set()
    stack = [path]
    while stack:
        with os.scandir(stack.pop()) as it:
            for entry in it:
                st = entry.stat(follow_symlinks=False)
                if newest is None or st.st_mtime > newest:
                    newest = st.st_mtime
                if entry.is_dir(follow_symlinks=False):
                    stack.append(entry.path)
                    continue
                if st.st_nlink > 1:
                    if (st.st_dev, st.st_ino) in seen:
                        continue
                    seen.add((st.st_dev, st.st_ino))
                total += st.st_size
    return total, newest


def iso(ts):
    if ts is None:
        return None
    return datetime.datetime.fromtimestamp(int(ts), datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def mount_name(description):
    prefix = "cached mount "
    if description.startswith(prefix):
        return description[len(prefix):].split(" ", 1)[0]
    return description


def snapshot_dirs(root, driver, record_ids):
    """{record ID: snapshot directory} for every ID that resolves."""
    if driver != "overlay2":
        return {}
    meta = Bolt(os.path.join(root, "buildkit", "metadata_v2.db"))
    snaps = Bolt(os.path.join(root, "buildkit", "snapshots.db"))
    main = meta.children()
    main = meta.children(main[b"_main"][0]) if b"_main" in main else {}
    snapshots = snaps.children()
    dirs = {}
    for rid in record_ids:
        rec = meta.sub(main, rid.encode())
        if not rec or not rec.get(b"cache.snapshot"):
            continue
        key = json.loads(rec[b"cache.snapshot"])["value"]
        snap = snaps.sub(snapshots, key.encode())
        if snap is None or snap.get(b"chainid"):  # committed image layer, not a mount
            continue
        gd_id = (snap.get(b"committed") or key.encode()).decode()
        if "/" in gd_id or gd_id in ("", ".", ".."):
            continue
        d = os.path.join(root, "overlay2", gd_id, "diff")
        if os.path.isdir(d):
            dirs[rid] = d
    return dirs


def main():
    info = engine_get("/info")
    records = engine_get("/system/df?type=build-cache").get("BuildCache") or []
    mounts = [r for r in records if r.get("Type") == "exec.cachemount"]

    try:
        dirs = snapshot_dirs(info["DockerRootDir"], info.get("Driver"), [m["ID"] for m in mounts])
        resolve_error = None
    except Exception as e:  # report, don't hide the record counts
        dirs, resolve_error = {}, str(e)

    out_mounts = []
    for m in mounts:
        entry = {"mount": mount_name(m.get("Description", "")), "id": m["ID"],
                 "bytes": None, "last_written": None}
        d = dirs.get(m["ID"])
        if d:
            try:
                size, newest = disk_usage(d)
                entry["bytes"], entry["last_written"] = size, iso(newest)
            except OSError:
                pass
        out_mounts.append(entry)

    measured = [m for m in out_mounts if m["bytes"] is not None]
    stats = {
        "records": len(records),
        "total_bytes": sum(r.get("Size", 0) for r in records),
        "cache_mount_count": len(mounts),
        "cache_mount_bytes": sum(m["bytes"] for m in measured),
        "cache_mount_last_written": max((m["last_written"] for m in measured if m["last_written"]), default=None),
        "cache_mount_unmeasured": len(mounts) - len(measured),
        "cache_mounts": sorted(out_mounts, key=lambda m: -(m["bytes"] or 0))[:TOP_MOUNTS],
    }
    if resolve_error:
        stats["cache_mount_error"] = resolve_error
    print(json.dumps(stats, separators=(",", ":")))


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        sys.exit(f"helix-buildkit-cache-stats: {e}")
