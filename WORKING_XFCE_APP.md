# Working XFCE App Configuration from Wolf API

This file contains the complete Wolf API response showing the working XFCE app configuration that we can use as reference for our Personal Dev environments.

## API Response

```json
{
  "success": true,
  "apps": [
    {
      "title": "Desktop (xfce)",
      "id": "846410152",
      "support_hdr": false,
      "icon_png_path": "https://games-on-whales.github.io/wildlife/apps/xfce/assets/icon.png",
      "h264_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false !\nh264parse !\nvideo/x-h264, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "hevc_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh265enc gop-size=-1 bitrate={bitrate} aud=false rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nh265parse !\nvideo/x-h265, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "av1_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvav1enc gop-size=-1 bitrate={bitrate} rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nav1parse !\nvideo/x-av1, stream-format=obu-stream, alignment=frame, profile=main !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "render_node": "/dev/dri/renderD128",
      "opus_gst_pipeline": "interpipesrc listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false !\nqueue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert !\nopusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 !\nrtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" !\nappsink name=wolf_udp_sink",
      "start_virtual_compositor": true,
      "start_audio_server": true,
      "runner": {
        "type": "docker",
        "name": "WolfXFCE",
        "image": "ghcr.io/games-on-whales/xfce:edge",
        "mounts": [],
        "env": ["GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"],
        "devices": [],
        "ports": [],
        "base_create_json": "{\n  \"HostConfig\": {\n    \"IpcMode\": \"host\",\n    \"Privileged\": false,\n    \"CapAdd\": [\"SYS_ADMIN\", \"SYS_NICE\", \"SYS_PTRACE\", \"NET_RAW\", \"MKNOD\", \"NET_ADMIN\"],\n    \"SecurityOpt\": [\"seccomp=unconfined\", \"apparmor=unconfined\"],\n    \"DeviceCgroupRules\": [\"c 13:* rmw\", \"c 244:* rmw\"]\n  }\n}\n"
      }
    },
    {
      "title": "Personal Dev Environment: 1",
      "id": "919262682",
      "support_hdr": false,
      "h264_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false !\nh264parse !\nvideo/x-h264, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "hevc_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh265enc gop-size=-1 bitrate={bitrate} aud=false rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nh265parse !\nvideo/x-h265, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "av1_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvav1enc gop-size=-1 bitrate={bitrate} rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nav1parse !\nvideo/x-av1, stream-format=obu-stream, alignment=frame, profile=main !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "render_node": "/dev/dri/renderD128",
      "opus_gst_pipeline": "interpipesrc listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false !\nqueue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert !\nopusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 !\nrtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" !\nappsink name=wolf_udp_sink",
      "start_virtual_compositor": true,
      "start_audio_server": false,
      "runner": {
        "type": "docker",
        "name": "PersonalDev_919262682",
        "image": "ghcr.io/games-on-whales/xfce:edge",
        "mounts": ["/opt/helix/filestore/workspaces/personal-dev-15414340-f065-46bc-a188-715074133e8b-1758987419:/home/user/work:rw"],
        "env": ["GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"],
        "devices": [],
        "ports": [],
        "base_create_json": "{\n  \"HostConfig\": {\n    \"IpcMode\": \"host\",\n    \"Privileged\": false,\n    \"CapAdd\": [\"SYS_ADMIN\", \"SYS_NICE\", \"SYS_PTRACE\", \"NET_RAW\", \"MKNOD\", \"NET_ADMIN\"],\n    \"SecurityOpt\": [\"seccomp=unconfined\", \"apparmor=unconfined\"],\n    \"DeviceCgroupRules\": [\"c 13:* rmw\", \"c 244:* rmw\"]\n  }\n}\n"
      }
    }
  ]
}
```

## Key Differences Analysis

Comparing the working XFCE app vs our Personal Dev Environment:

### Working XFCE App:
- **Container name**: `WolfXFCE` (short, simple)
- **Mounts**: `[]` (empty array)
- **start_audio_server**: `true`
- **Icon**: Has icon_png_path

### Our Personal Dev Environment:
- **Container name**: `PersonalDev_919262682` (longer but still reasonable)
- **Mounts**: `["/opt/helix/filestore/workspaces/...:/home/user/work:rw"]` (has workspace mount)
- **start_audio_server**: `false` (we set this to false)
- **Icon**: No icon_png_path (we removed it)

### Pipeline Configurations (Both Identical):
All GStreamer pipelines match exactly between working XFCE and our Personal Dev environment.

## Next Steps

The configurations look very similar. The main differences are:
1. We have a workspace mount (which is expected)
2. We set start_audio_server to false
3. We don't have an icon

These differences shouldn't cause streaming issues. The GStreamer pipelines are identical, which suggests our implementation is correct.