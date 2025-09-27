// Complete Wolf app structure with working GStreamer pipelines from XFCE
package wolf

// MinimalWolfApp represents a complete Wolf app structure with working GStreamer pipelines
// Uses exact pipeline strings from the working XFCE configuration to avoid empty pipeline errors
type MinimalWolfApp struct {
	// Core required fields
	Title                  string            `json:"title"`
	ID                     string            `json:"id"`
	Runner                 MinimalWolfRunner `json:"runner"`
	StartVirtualCompositor bool              `json:"start_virtual_compositor"`

	// Optional fields
	RenderNode  *string `json:"render_node,omitempty"`
	IconPngPath *string `json:"icon_png_path,omitempty"`
	SupportHDR  *bool   `json:"support_hdr,omitempty"`

	// Video pipeline fields - use working XFCE pipelines to avoid empty pipeline errors
	VideoProducerBufferCaps *string `json:"video_producer_buffer_caps,omitempty"`
	H264GstPipeline         *string `json:"h264_gst_pipeline,omitempty"`
	HEVCGstPipeline         *string `json:"hevc_gst_pipeline,omitempty"`
	AV1GstPipeline          *string `json:"av1_gst_pipeline,omitempty"`
	OpusGstPipeline         *string `json:"opus_gst_pipeline,omitempty"`

	// Audio/compositor settings
	StartAudioServer bool `json:"start_audio_server"` // Always send this field as direct bool
}

// MinimalWolfRunner represents the minimal Docker runner based on working wolf-ui implementation
// Uses nullable fields with omitempty to match working client behavior
type MinimalWolfRunner struct {
	Type           string    `json:"type"`
	Name           *string   `json:"name,omitempty"`
	Image          *string   `json:"image,omitempty"`
	Mounts         *[]string `json:"mounts,omitempty"`
	Env            *[]string `json:"env,omitempty"`
	Devices        *[]string `json:"devices,omitempty"`
	Ports          *[]string `json:"ports,omitempty"`
	BaseCreateJSON *string   `json:"base_create_json,omitempty"`
	RunCmd         *string   `json:"run_cmd,omitempty"`
}

// NewMinimalDockerApp creates a Docker app with complete working GStreamer pipelines
// Uses exact pipeline strings from working XFCE configuration to avoid empty pipeline errors
func NewMinimalDockerApp(id, title, name, image string, env, mounts []string, baseCreateJSON string) *MinimalWolfApp {
	return &MinimalWolfApp{
		// Core fields
		Title:                  title,
		ID:                     id,
		StartVirtualCompositor: true,

		// Complete GStreamer pipeline configuration (copied from working XFCE app)
		VideoProducerBufferCaps: StringPtr("video/x-raw(memory:DMABuf)"),
		H264GstPipeline: StringPtr("interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false !\nh264parse !\nvideo/x-h264, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n"),
		HEVCGstPipeline: StringPtr("interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvh265enc gop-size=-1 bitrate={bitrate} aud=false rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nh265parse !\nvideo/x-h265, profile=main, stream-format=byte-stream !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n"),
		AV1GstPipeline: StringPtr("interpipesrc listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream !\nglupload !\nglcolorconvert !\nvideo/x-raw(memory:GLMemory),format=NV12, width={width}, height={height}, chroma-site={color_range}, colorimetry={color_space}, pixel-aspect-ratio=1/1 !\nnvav1enc gop-size=-1 bitrate={bitrate} rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !\nav1parse !\nvideo/x-av1, stream-format=obu-stream, alignment=frame, profile=main !\nrtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !\nappsink sync=false name=wolf_udp_sink\n"),
		OpusGstPipeline: StringPtr("interpipesrc listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false !\nqueue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert !\nopusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400 !\nrtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key=\"{aes_key}\" aes_iv=\"{aes_iv}\" !\nappsink name=wolf_udp_sink"),

		// Settings that match XFCE working config
		StartAudioServer: true,
		SupportHDR:       BoolPtr(false),
		RenderNode:       StringPtr("/dev/dri/renderD128"),
		// IconPngPath:      StringPtr("https://games-on-whales.github.io/wildlife/apps/xfce/assets/icon.png"), // Removed icon to avoid conflicts

		Runner: MinimalWolfRunner{
			Type:           "docker",
			Name:           StringPtr(name),
			Image:          StringPtr(image),
			Mounts:         &mounts,
			Env:            &env,
			Devices:        &[]string{},
			Ports:          &[]string{},
			BaseCreateJSON: StringPtr(baseCreateJSON),
		},
	}
}