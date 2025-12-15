# Haystack CPU Profiling Guide

## Overview

This document describes how to profile CPU usage in the Haystack RAG service running in a Docker container.

## The Challenge

The Haystack service runs in a minimal Docker container without standard debugging tools (`ps`, `top`, etc.). We need to use `py-spy` to profile Python CPU usage, but it requires:

1. Access to the container's PID namespace
2. `SYS_PTRACE` capability
3. Knowing the correct Python process PID inside the container

## Finding the Python Process PID

The main process (PID 1) in the Haystack container is `uv run`, not Python directly. The container is minimal and doesn't include `ps`, so we need to install `procps` first:

```bash
docker run --rm -it \
  --pid=container:$(docker compose ps -q haystack | head -1) \
  --cap-add SYS_PTRACE \
  python:3.11-slim bash -c "apt update && apt install -y procps && pip install py-spy && ps aux"
```

Note: We tried `py-spy top --pid 1` first but got "Error: Failed to find python version from target process" because PID 1 is `uv run`, not Python. Installing `procps` to get `ps aux` revealed the actual Python process.

Example output:
```
USER         PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root           1  0.0  0.2  79208 48328 ?        Ssl  14:52   0:00 uv run --frozen --no-de
root          11  0.5  1.9 809784 326112 ?       Sl   14:52   0:11 /app/.venv/bin/python /
root          78  0.2  0.0   6792  3936 pts/0    Rs+  15:25   0:00 ps aux
```

The Python process is **PID 11** (`/app/.venv/bin/python`).

## Profiling Commands

### Live CPU Monitoring (like `top` for Python)

```bash
docker run --rm -it \
  --pid=container:$(docker compose ps -q haystack | head -1) \
  --cap-add SYS_PTRACE \
  python:3.11-slim bash -c "pip install py-spy && py-spy top --pid 11"
```

### Record a Flame Graph

Record CPU usage for 60 seconds and generate an SVG flame graph:

```bash
docker run --rm -it \
  --pid=container:$(docker compose ps -q haystack | head -1) \
  --cap-add SYS_PTRACE \
  -v /tmp:/tmp \
  python:3.11-slim bash -c "pip install py-spy && py-spy record -o /tmp/haystack-profile.svg --pid 11 --duration 60"
```

The flame graph will be saved to `/tmp/haystack-profile.svg` on the host.

## Interpreting Results

When running `py-spy top`, you'll see output like:

```
  %Own   %Total  OwnTime  TotalTime  Function (filename)
```

- **%Own**: CPU time spent in this function (not including calls to other functions)
- **%Total**: CPU time spent in this function and all functions it calls
- **OwnTime**: Total seconds spent in this function
- **TotalTime**: Total seconds including child calls
- **Function**: The function name and source file

### What to Look For

1. Functions with high `%Own` at the top are direct CPU consumers
2. High `%Total` but low `%Own` means the function calls other expensive functions
3. During idle, CPU should be near 0% - high idle CPU indicates a bug

### Common CPU-Intensive Operations in Haystack

Based on code review (`haystack_service/app/service.py`):

1. `splitter.warm_up()` (line 100) - Loads NLTK models at startup
2. Embedding generation - Every query runs through the embedder
3. Debug logging in `query()` method - Heavy logging on every query
4. `_analyze_scores()` - Score analysis runs on every query
5. BM25 + Vector retrieval running in parallel

## Troubleshooting

### "Error: Failed to find python version from target process"

This means you're profiling the wrong PID. Use the `ps aux` command above to find the correct Python PID.

### "Permission denied (os error 13)"

Make sure you're using `--cap-add SYS_PTRACE` in the docker run command.

### PID 1 doesn't work

PID 1 is typically the entrypoint script (`uv run`), not Python. Find the actual Python process using `ps aux`.
