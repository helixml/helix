#!/bin/bash
set -e

# Script to generate Wolf Moonlight config dynamically based on active Helix sessions
# This allows each Helix session to appear as a separate "app" in Moonlight

HELIX_API_URL="${API_HOST:-http://api:8080}"
WOLF_CFG_DIR="${WOLF_CFG_FOLDER:-/etc/wolf/cfg}"
HOSTNAME="${HOSTNAME:-helix-moonlight}"

echo "Generating Wolf config for active Helix sessions..."

# Create base config
cat > "$WOLF_CFG_DIR/config.toml" <<EOF
# Wolf Moonlight Server Configuration for Helix
hostname = "$HOSTNAME"

EOF

# Function to get active sessions from Helix API
get_active_sessions() {
    # Try to get active work sessions from Helix API
    # Note: This would need to be adapted based on actual API endpoints
    curl -s "$HELIX_API_URL/api/v1/work-sessions" 2>/dev/null || echo "[]"
}

# Function to sanitize session name for config
sanitize_name() {
    echo "$1" | sed 's/[^a-zA-Z0-9 ]//g' | sed 's/  */ /g' | sed 's/^ *//' | sed 's/ *$//'
}

# Get active sessions and generate apps
sessions_json=$(get_active_sessions)

# Parse sessions and create app entries
app_id=1

# Always include a default desktop app
cat >> "$WOLF_CFG_DIR/config.toml" <<EOF
[[apps]]
title = "Helix Desktop"
id = $app_id
start_virtual_compositor = false
support_hdr = false
runner = "fake-udev"

[apps.env]

EOF

app_id=$((app_id + 1))

# Add active sessions as apps (if we can parse them)
if command -v jq >/dev/null 2>&1 && [ "$sessions_json" != "[]" ]; then
    echo "$sessions_json" | jq -r '.[] | select(.status == "active" or .status == "running") | "\(.id)|\(.name // "Unnamed Session")|\(.description // "")"' | while IFS='|' read -r session_id session_name session_desc; do
        if [ -n "$session_id" ]; then
            clean_name=$(sanitize_name "$session_name")
            [ -z "$clean_name" ] && clean_name="Session $session_id"
            
            cat >> "$WOLF_CFG_DIR/config.toml" <<EOF
[[apps]]
title = "$clean_name"
id = $app_id
start_virtual_compositor = false
support_hdr = false
runner = "fake-udev"

[apps.env]
HELIX_SESSION_ID = "$session_id"

EOF
            app_id=$((app_id + 1))
        fi
    done
fi

# Add the rest of the configuration
cat >> "$WOLF_CFG_DIR/config.toml" <<EOF
[video]
resolution = ["2360x1640", "1920x1080", "1280x720"]
fps = [60, 120]

# GPU configuration - matches your NVIDIA setup  
render_node = "/dev/dri/renderD128"

# GStreamer pipeline for NVIDIA encoding
video_producer_buffer_caps = "video/x-raw,format=NV12"

[audio]
mic = []
speakers = []

[input]
# Virtual input devices - Wolf will create uinput devices
mouse = []
keyboard = []
joypad = []

[streaming]
# Network configuration
address_family = "BOTH"

[gstreamer]
# Enable hardware acceleration
video_encoder = "nvh264enc"
audio_encoder = "opusenc"

# Video encoding pipeline
[gstreamer.video]
default_source = 'waylanddisplaysrc render_node=/dev/dri/renderD128 ! video/x-raw,format=NV12'
default_sink = '''
rtpmoonlightpay_video name=moonlight_pay 
payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !
appsink sync=false name=wolf_udp_sink
'''

# H264 encoders
[[gstreamer.video.h264_encoders]]
plugin_name = "nvcodec"
check_elements = ["nvh264enc", "cudaconvertscale", "cudaupload"]
encoder_pipeline = '''
nvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false !
h264parse !
video/x-h264, profile=main, stream-format=byte-stream
'''

# Audio configuration
[gstreamer.audio]
default_source = 'pulsesrc device=auto ! audioconvert'
default_audio_params = "queue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert"
default_opus_encoder = '''
opusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband 
audio-type=restricted-lowdelay max-payload-size=1400
'''
default_sink = '''
rtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} 
aes_key="{aes_key}" aes_iv="{aes_iv}" !
appsink name=wolf_udp_sink
'''
EOF

echo "Wolf config generated at $WOLF_CFG_DIR/config.toml"
echo "Found $(($app_id - 1)) apps total (including default desktop)"