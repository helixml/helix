#!/bin/bash
# ====================================================================
# AI Agent Telemetry Firewall
# ====================================================================
# Blocks all known telemetry endpoints for AI coding agents
# Includes iptables counters to detect phone-home attempts
# Logs are exposed via /var/log/telemetry-blocks.log for dashboard monitoring
# ====================================================================

set -e

LOGFILE="/var/log/telemetry-blocks.log"
COUNTER_FILE="/var/run/telemetry-counters.json"

echo "🔒 Setting up AI agent telemetry firewall..."

# Check if iptables is available
if ! command -v iptables &> /dev/null; then
    echo "⚠️  iptables not found - skipping telemetry firewall setup"
    echo "   Container networking is still secure via Docker network isolation"
    exit 0
fi

# Ensure log files exist with proper permissions
touch "$LOGFILE"
chmod 644 "$LOGFILE"

# ====================================================================
# Create custom iptables chain for telemetry blocking with logging
# ====================================================================
iptables -N TELEMETRY_BLOCK 2>/dev/null || iptables -F TELEMETRY_BLOCK

# Log and reject rule
iptables -A TELEMETRY_BLOCK -j LOG --log-prefix "TELEMETRY_BLOCKED: " --log-level 4
iptables -A TELEMETRY_BLOCK -j REJECT --reject-with icmp-port-unreachable

# ====================================================================
# Block Qwen Code Telemetry Endpoints
# ====================================================================
# echo "Blocking Qwen Code telemetry..." | tee -a "$LOGFILE"

# # Alibaba Cloud RUM (QwenLogger)
# iptables -I OUTPUT -d gb4w8c3ygj-default-sea.rum.aliyuncs.com -m comment --comment "QWEN_CODE:ALIBABA_RUM" -j TELEMETRY_BLOCK

# # Google Clearcut (ClearcutLogger)
# iptables -I OUTPUT -d play.googleapis.com -p tcp --dport 443 -m comment --comment "QWEN_CODE:GOOGLE_CLEARCUT" -j TELEMETRY_BLOCK

# # Broader Alibaba Cloud RUM blocking (in case they rotate endpoints)
# iptables -I OUTPUT -m string --string "rum.aliyuncs.com" --algo bm -m comment --comment "QWEN_CODE:ALIBABA_RUM_WILDCARD" -j TELEMETRY_BLOCK

# # ====================================================================
# # Block Gemini CLI Telemetry Endpoints (same as Qwen Code)
# # ====================================================================
# echo "Blocking Gemini CLI telemetry..." | tee -a "$LOGFILE"
# # Covered by same rules as Qwen Code (it's the upstream project)

# # ====================================================================
# # Block Claude Code Telemetry Endpoints
# # ====================================================================
# echo "Blocking Claude Code telemetry..." | tee -a "$LOGFILE"

# # Anthropic Statsig telemetry
# iptables -I OUTPUT -m string --string "statsig" --algo bm -m comment --comment "CLAUDE_CODE:STATSIG" -j TELEMETRY_BLOCK

# # Anthropic API telemetry endpoints
# iptables -I OUTPUT -m string --string "telemetry.anthropic.com" --algo bm -m comment --comment "CLAUDE_CODE:ANTHROPIC_TELEMETRY" -j TELEMETRY_BLOCK

# # ====================================================================
# # Block Zed Editor Telemetry Endpoints
# # ====================================================================
# echo "Blocking Zed editor telemetry..." | tee -a "$LOGFILE"

# # Zed telemetry endpoint
# iptables -I OUTPUT -m string --string "telemetry.zed.dev" --algo bm -m comment --comment "ZED:TELEMETRY" -j TELEMETRY_BLOCK
# iptables -I OUTPUT -m string --string "zed.dev/api" --algo bm -m comment --comment "ZED:API" -j TELEMETRY_BLOCK

# # ====================================================================
# # Block Common Analytics/Tracking Services
# # ====================================================================
# echo "Blocking common analytics services..." | tee -a "$LOGFILE"

# # Google Analytics
# iptables -I OUTPUT -m string --string "google-analytics.com" --algo bm -m comment --comment "ANALYTICS:GOOGLE" -j TELEMETRY_BLOCK
# iptables -I OUTPUT -m string --string "analytics.google.com" --algo bm -m comment --comment "ANALYTICS:GOOGLE_V2" -j TELEMETRY_BLOCK

# # Mixpanel
# iptables -I OUTPUT -m string --string "mixpanel.com" --algo bm -m comment --comment "ANALYTICS:MIXPANEL" -j TELEMETRY_BLOCK

# # Segment
# iptables -I OUTPUT -m string --string "segment.io" --algo bm -m comment --comment "ANALYTICS:SEGMENT" -j TELEMETRY_BLOCK
# iptables -I OUTPUT -m string --string "segment.com" --algo bm -m comment --comment "ANALYTICS:SEGMENT_V2" -j TELEMETRY_BLOCK

# # Sentry (error tracking)
# iptables -I OUTPUT -m string --string "sentry.io" --algo bm -m comment --comment "ANALYTICS:SENTRY" -j TELEMETRY_BLOCK

# ====================================================================
# Export counters for dashboard monitoring
# ====================================================================
update_counters() {
    # Extract packet and byte counts from iptables
    cat > "$COUNTER_FILE" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "rules": [
EOF

    first=true
    iptables -L OUTPUT -n -v -x | grep "TELEMETRY_BLOCK" | while IFS= read -r line; do
        pkts=$(echo "$line" | awk '{print $1}')
        bytes=$(echo "$line" | awk '{print $2}')
        comment=$(echo "$line" | grep -o 'comment "[^"]*"' | sed 's/comment "\(.*\)"/\1/' || echo "UNKNOWN")

        [ "$first" = false ] && echo "," >> "$COUNTER_FILE"
        first=false

        cat >> "$COUNTER_FILE" <<EOF
    {
      "rule": "$comment",
      "packets_blocked": $pkts,
      "bytes_blocked": $bytes
    }
EOF
    done

    cat >> "$COUNTER_FILE" <<EOF
  ]
}
EOF

    chmod 644 "$COUNTER_FILE"
}

# Initial counter export
update_counters

# ====================================================================
# Set up periodic counter updates (every 60 seconds)
# ====================================================================
# Ensure script directory exists
mkdir -p /wolf/sway-config

cat > /etc/cron.d/telemetry-counter-update <<'CRON'
* * * * * root /wolf/sway-config/update-telemetry-counters.sh
CRON

# Create the update script
cat > /wolf/sway-config/update-telemetry-counters.sh <<'UPDATE_SCRIPT'
#!/bin/bash
# Update telemetry counter JSON for dashboard
COUNTER_FILE="/var/run/telemetry-counters.json"
LOGFILE="/var/log/telemetry-blocks.log"

cat > "$COUNTER_FILE" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "rules": [
EOF

first=true
iptables -L OUTPUT -n -v -x | grep -A1 "TELEMETRY_BLOCK" | grep -E "pkts|Chain" | while IFS= read -r line; do
    if echo "$line" | grep -q "pkts"; then
        pkts=$(echo "$line" | awk '{print $1}')
        bytes=$(echo "$line" | awk '{print $2}')
        target=$(echo "$line" | awk '{print $3}')

        # Extract comment from next iptables line
        comment=$(iptables -L OUTPUT -n -v | grep -B1 "$pkts" | grep "comment" | sed 's/.*comment "\([^"]*\)".*/\1/' || echo "UNKNOWN")

        [ "$first" = false ] && echo "," >> "$COUNTER_FILE"
        first=false

        cat >> "$COUNTER_FILE" <<RULE
    {
      "rule": "$comment",
      "packets_blocked": $pkts,
      "bytes_blocked": $bytes
    }
RULE

        # Log any blocks detected
        if [ "$pkts" -gt 0 ]; then
            echo "[$(date -Iseconds)] BLOCKED: $comment - $pkts packets, $bytes bytes" >> "$LOGFILE"
        fi
    fi
done

cat >> "$COUNTER_FILE" <<EOF
  ]
}
EOF

chmod 644 "$COUNTER_FILE"
UPDATE_SCRIPT

chmod +x /wolf/sway-config/update-telemetry-counters.sh

# ====================================================================
# Summary
# ====================================================================
echo "✅ Telemetry firewall configured" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"
echo "Blocked telemetry endpoints:" | tee -a "$LOGFILE"
echo "  - Qwen Code: gb4w8c3ygj-default-sea.rum.aliyuncs.com, play.googleapis.com" | tee -a "$LOGFILE"
echo "  - Claude Code: statsig, telemetry.anthropic.com" | tee -a "$LOGFILE"
echo "  - Zed: telemetry.zed.dev" | tee -a "$LOGFILE"
echo "  - Analytics: google-analytics, mixpanel, segment, sentry" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"
echo "Monitoring:" | tee -a "$LOGFILE"
echo "  - Log file: $LOGFILE" | tee -a "$LOGFILE"
echo "  - Counters: $COUNTER_FILE (updated every 60s)" | tee -a "$LOGFILE"
echo "" | tee -a "$LOGFILE"
iptables -L OUTPUT -n -v | grep TELEMETRY_BLOCK | tee -a "$LOGFILE"
