#!/usr/bin/env bash
# Spike: measure Zed startup CPU/RSS/process-spawn impact of configuring many agents.
# Runs as user `retro` inside a helix-ubuntu container under Xvfb.
#
# Compares three settings.json variants over a fixed observation window:
#   baseline    : 1 agent_server,   0 context_servers
#   agents100   : 100 agent_servers,0 context_servers   (the "configure all agents" cost)
#   mcp100      : 1 agent_server,  100 context_servers   (the per-agent MCP "npx storm" cost)
set -u
ZED=/zed-build/zed
CFG=/home/retro/.config/zed
LOG=/home/retro/.local/share/zed/logs/Zed.log
WORK=/home/retro/work
OBSERVE=25   # seconds to let Zed settle before sampling

export ZED_ALLOW_ROOT=false
export HOME=/home/retro
mkdir -p "$CFG" "$WORK" /home/retro/.local/share/zed/logs

gen_settings() {
  # $1 = number of agent_servers, $2 = number of context_servers
  local na=$1 nc=$2
  python3 - "$na" "$nc" <<'PY'
import json,sys
na,nc=int(sys.argv[1]),int(sys.argv[2])
s={"telemetry":{"diagnostics":False,"metrics":False}}
ag={}
for i in range(na):
    ag[f"agent{i}"]={"name":f"agent{i}","type":"custom","command":"qwen",
                     "args":["--experimental-acp"],
                     "env":{"OPENAI_BASE_URL":"http://api:8080/v1","OPENAI_MODEL":f"m{i}"}}
if na: s["agent_servers"]=ag
cs={}
for i in range(nc):
    # do-nothing stdio process: isolates spawn/registration cost from network/npx-download
    cs[f"mcp{i}"]={"command":{"path":"node","args":["-e","setInterval(()=>{},1e9)"]}}
if nc: s["context_servers"]=cs
print(json.dumps(s,indent=2))
PY
}

# cpu_rss_of_pattern PATTERN -> echoes "cpu_ticks rss_kb count"
cpu_rss_of_pattern() {
  local pat=$1 ticks=0 rss=0 cnt=0 p ut st r
  for p in $(pgrep -f "$pat"); do
    [ -r /proc/$p/stat ] || continue
    ut=$(awk '{print $14}' /proc/$p/stat); st=$(awk '{print $15}' /proc/$p/stat)
    ticks=$((ticks + ut + st))
    r=$(awk '/VmRSS/{print $2}' /proc/$p/status 2>/dev/null); rss=$((rss + ${r:-0}))
    cnt=$((cnt+1))
  done
  echo "$ticks $rss $cnt"
}

measure() {
  local label=$1 na=$2 nc=$3
  rm -f "$LOG"
  gen_settings "$na" "$nc" > "$CFG/settings.json"
  pkill -f "Xvfb :99" 2>/dev/null; sleep 1
  Xvfb :99 -screen 0 1280x1024x24 >/dev/null 2>&1 &
  local xvfb=$!
  sleep 1
  export DISPLAY=:99
  setsid $ZED "$WORK" >/tmp/zed-$label.out 2>&1 &
  sleep "$OBSERVE"
  local clk=$(getconf CLK_TCK)
  read zt zr zc < <(cpu_rss_of_pattern "$ZED")
  read mt mr mc < <(cpu_rss_of_pattern "node -e setInterval")
  local zed_cpu=$(echo "scale=1; $zt / $clk" | bc)
  local mcp_cpu=$(echo "scale=1; $mt / $clk" | bc)
  echo "=== $label : agent_servers=$na context_servers=$nc (window ${OBSERVE}s) ==="
  echo "  zed procs=$zc  zed_cpu=${zed_cpu}s  zed_rss=$((zr/1024))MB"
  echo "  mcp procs spawned=$mc (configured $nc)  mcp_cpu=${mcp_cpu}s  mcp_rss=$((mr/1024))MB"
  echo "  TOTAL rss=$(((zr+mr)/1024))MB  TOTAL cpu=$(echo "scale=1;($zt+$mt)/$clk"|bc)s"
  pkill -f "$ZED" 2>/dev/null; pkill -f "node -e setInterval" 2>/dev/null
  pkill -f "Xvfb :99" 2>/dev/null; sleep 2
}

echo "#### Zed agent-scaling spike ####"
echo "cores: $(nproc)  observe-window: ${OBSERVE}s  (NOTE: software renderer = high CPU noise floor)"
echo
echo ">>> run A"
measure baseline_a   1   0
measure agents100_a  100 0
echo ">>> run B"
measure baseline_b   1   0
measure agents100_b  100 0
echo ">>> MCP storm (per-agent MCP unioning)"
measure mcp100       1   100
echo
echo "#### done ####"
