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
      "video_producer_buffer_caps": "video/x-raw(memory:DMABuf)",
      "opus_gst_pipeline": "interpipesrc listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false !\nqueue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert !\nopusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 !\nrtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" !\nappsink name=wolf_udp_sink",
      "start_virtual_compositor": true,
      "start_audio_server": true,
      "runner": {
        "type": "docker",
        "name": "WolfXFCE",
        "image": "ghcr.io/games-on-whales/xfce:edge",
        "mounts": [],
        "env": [
          "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"
        ],
        "devices": [],
        "ports": [],
        "base_create_json": "{\n  \"HostConfig\": {\n    \"IpcMode\": \"host\",\n    \"Privileged\": false,\n    \"CapAdd\": [\"SYS_ADMIN\", \"SYS_NICE\", \"SYS_PTRACE\", \"NET_RAW\", \"MKNOD\", \"NET_ADMIN\"],\n    \"SecurityOpt\": [\"seccomp=unconfined\", \"apparmor=unconfined\"],\n    \"DeviceCgroupRules\": [\"c 13:* rmw\", \"c 244:* rmw\"]\n  }\n}\n"
      }
    },
    {
      "title": "Test ball",
      "id": "378473508",
      "support_hdr": false,
      "h264_gst_pipeline": "videotestsrc pattern=ball flip=true is-live=true !\nvideo/x-raw, framerate={fps}/1\n !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false !\nh264parse !\nvideo/x-h264, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "hevc_gst_pipeline": "videotestsrc pattern=ball flip=true is-live=true !\nvideo/x-raw, framerate={fps}/1\n !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh265enc gop-size=-1 bitrate={bitrate} aud=false rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nh265parse !\nvideo/x-h265, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "av1_gst_pipeline": "videotestsrc pattern=ball flip=true is-live=true !\nvideo/x-raw, framerate={fps}/1\n !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvav1enc gop-size=-1 bitrate={bitrate} rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nav1parse !\nvideo/x-av1, stream-format=obu-stream, alignment=frame, profile=main !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "render_node": "/dev/dri/renderD128",
      "video_producer_buffer_caps": "video/x-raw(memory:DMABuf)",
      "opus_gst_pipeline": "audiotestsrc wave=ticks is-live=true !\nqueue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert !\nopusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 !\nrtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" !\nappsink name=wolf_udp_sink",
      "start_virtual_compositor": false,
      "start_audio_server": false,
      "runner": {
        "type": "process",
        "run_cmd": "sh -c \"while :; do echo 'running...'; sleep 10; done\""
      }
    },
    {
      "title": "Test Integration",
      "id": "181069715",
      "support_hdr": false,
      "h264_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false !\nh264parse !\nvideo/x-h264, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "hevc_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh265enc gop-size=-1 bitrate={bitrate} aud=false rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nh265parse !\nvideo/x-h265, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "av1_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvav1enc gop-size=-1 bitrate={bitrate} rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nav1parse !\nvideo/x-av1, stream-format=obu-stream, alignment=frame, profile=main !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n",
      "render_node": "/dev/dri/renderD128",
      "video_producer_buffer_caps": "video/x-raw(memory:DMABuf)",
      "opus_gst_pipeline": "interpipesrc listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false !\nqueue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert !\nopusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 !\nrtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" !\nappsink name=wolf_udp_sink",
      "start_virtual_compositor": true,
      "start_audio_server": true,
      "runner": {
        "type": "docker",
        "name": "test-container",
        "image": "ubuntu:latest",
        "mounts": [],
        "env": [],
        "devices": [],
        "ports": [],
        "base_create_json": "{\n\"HostConfig\": {\n  \"IpcMode\": \"host\"\n}\n}"
      }
    },
    {
      "title": "Personal Dev 1",
      "id": "919262682",
      "support_hdr": false,
      "h264_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream ! video/x-raw, width={width}, height={height}, framerate={fps}/1 ! nvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false ! h264parse ! video/x-h264, profile=main, stream-format=byte-stream ! rtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} ! appsink sync=false name=wolf_udp_sink",
      "hevc_gst_pipeline": "",
      "av1_gst_pipeline": "",
      "render_node": "/dev/dri/renderD128",
      "video_producer_buffer_caps": "video/x-raw",
      "opus_gst_pipeline": "interpipesrc listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false ! queue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert ! opusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 ! rtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" ! appsink name=wolf_udp_sink",
      "start_virtual_compositor": true,
      "start_audio_server": true,
      "runner": {
        "type": "docker",
        "name": "WolfXFCE_personal-dev-15414340-f065-46bc-a188-715074133e8b-1758984694",
        "image": "ghcr.io/games-on-whales/xfce:edge",
        "mounts": [
          "/app/api/zed-build:/zed-build:rw",
          "/opt/helix/filestore/workspaces/personal-dev-15414340-f065-46bc-a188-715074133e8b-1758984694:/home/user/work:rw"
        ],
        "env": [
          "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"
        ],
        "devices": [],
        "ports": [],
        "base_create_json": "{\n  \"HostConfig\": {\n    \"IpcMode\": \"host\",\n    \"Privileged\": false,\n    \"CapAdd\": [\"SYS_ADMIN\", \"SYS_NICE\", \"SYS_PTRACE\", \"NET_RAW\", \"MKNOD\", \"NET_ADMIN\"],\n    \"SecurityOpt\": [\"seccomp=unconfined\", \"apparmor=unconfined\"],\n    \"DeviceCgroupRules\": [\"c 13:* rmw\", \"c 244:* rmw\"]\n  }\n}"
      }
    },
    {
      "title": "Personal Dev Debug Test",
      "id": "46706422",
      "support_hdr": false,
      "h264_gst_pipeline": "interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream ! video/x-raw, width={width}, height={height}, framerate={fps}/1 ! nvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false ! h264parse ! video/x-h264, profile=main, stream-format=byte-stream ! rtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} ! appsink sync=false name=wolf_udp_sink",
      "hevc_gst_pipeline": "",
      "av1_gst_pipeline": "",
      "render_node": "/dev/dri/renderD128",
      "video_producer_buffer_caps": "video/x-raw",
      "opus_gst_pipeline": "interpipesrc listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false ! queue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert ! opusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 ! rtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" ! appsink name=wolf_udp_sink",
      "start_virtual_compositor": true,
      "start_audio_server": true,
      "runner": {
        "type": "docker",
        "name": "WolfXFCE_personal-dev-15414340-f065-46bc-a188-715074133e8b-1758984914",
        "image": "ghcr.io/games-on-whales/xfce:edge",
        "mounts": [
          "/app/api/zed-build:/zed-build:rw",
          "/opt/helix/filestore/workspaces/personal-dev-15414340-f065-46bc-a188-715074133e8b-1758984914:/home/user/work:rw"
        ],
        "env": [
          "GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"
        ],
        "devices": [],
        "ports": [],
        "base_create_json": "{\n  \"HostConfig\": {\n    \"IpcMode\": \"host\",\n    \"Privileged\": false,\n    \"CapAdd\": [\"SYS_ADMIN\", \"SYS_NICE\", \"SYS_PTRACE\", \"NET_RAW\", \"MKNOD\", \"NET_ADMIN\"],\n    \"SecurityOpt\": [\"seccomp=unconfined\", \"apparmor=unconfined\"],\n    \"DeviceCgroupRules\": [\"c 13:* rmw\", \"c 244:* rmw\"]\n  }\n}"
      }
    }
  ]
}
