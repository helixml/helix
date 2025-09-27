// Minimal Wolf app types that exactly match the working XFCE configuration
package wolf

// MinimalWolfApp represents the minimal Wolf app structure that actually works
// Based on the working XFCE configuration from Wolf's config.toml
// Includes required top-level fields but uses minimal runner
// MinimalWolfApp exactly matches wolf-ui's working App model
// Omits problematic GStreamer pipeline fields that cause syntax errors
type MinimalWolfApp struct {
	// Core required fields (wolf-ui pattern)
	Title                  string            `json:"title"`
	ID                     string            `json:"id"`
	Runner                 MinimalWolfRunner `json:"runner"`
	StartVirtualCompositor bool              `json:"start_virtual_compositor"` // NOT nullable in wolf-ui

	// Optional fields that wolf-ui includes
	RenderNode  *string `json:"render_node,omitempty"`  // Nullable like wolf-ui
	IconPngPath *string `json:"icon_png_path,omitempty"`

	// Required field for proper GStreamer pipeline generation
	VideoProducerBufferCaps *string `json:"video_producer_buffer_caps,omitempty"`

	// OMITTED: h264_gst_pipeline, hevc_gst_pipeline, av1_gst_pipeline, opus_gst_pipeline, support_hdr
	// Wolf-ui successfully omits these fields and Wolf uses internal defaults
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

// NewMinimalDockerApp creates a minimal Docker app using wolf-ui's successful pattern
// Omits problematic GStreamer pipeline fields that wolf-ui doesn't send
func NewMinimalDockerApp(id, title, name, image string, env, mounts []string, baseCreateJSON string) *MinimalWolfApp {
	return &MinimalWolfApp{
		// Core fields that wolf-ui always sends
		Title:                  title,
		ID:                     id, // Use the provided consistent ID
		StartVirtualCompositor: true, // Required non-nullable field

		// Set video buffer caps - use simple format to avoid GPU issues
		VideoProducerBufferCaps: StringPtr("video/x-raw"),
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
		// Optional fields (wolf-ui leaves these as nil for defaults)
		RenderNode:  nil, // Let Wolf use internal defaults
		IconPngPath: nil, // Let Wolf use internal defaults

		// OMITTED (like wolf-ui): h264_gst_pipeline, hevc_gst_pipeline, av1_gst_pipeline,
		// opus_gst_pipeline, support_hdr, start_audio_server
		// Wolf will use its internal configuration defaults for these
	}
}