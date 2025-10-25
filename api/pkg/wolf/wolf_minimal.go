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

	// Default display configuration for Personal Dev Environments
	// Used when auto_start_containers=true OR when no client preferences available
	DefaultDisplayWidth  *int `json:"default_display_width,omitempty"`  // Default streaming resolution width
	DefaultDisplayHeight *int `json:"default_display_height,omitempty"` // Default streaming resolution height
	DefaultDisplayFPS    *int `json:"default_display_fps,omitempty"`    // Default streaming framerate
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

// NewMinimalDockerApp creates a Docker app for Wolf apps mode
// Uses Wolf's virtual compositor with interpipe for video routing
func NewMinimalDockerApp(id, title, name, image string, env, mounts []string, baseCreateJSON string, displayWidth, displayHeight, displayFPS int) *MinimalWolfApp {
	return &MinimalWolfApp{
		// Core fields
		Title:                  title,
		ID:                     id,
		StartVirtualCompositor: true,  // Wolf provides virtual compositor

		// Let Wolf use its own default pipelines based on WOLF_USE_ZERO_COPY env var
		// Wolf will automatically choose correct pipelines based on zero-copy setting
		// DO NOT override these - Wolf's defaults handle DMABuf vs non-DMABuf correctly

		// Settings that match working config
		StartAudioServer: true,
		SupportHDR:       BoolPtr(false),
		RenderNode:       StringPtr("/dev/dri/renderD128"),

		// Default display configuration
		DefaultDisplayWidth:  IntPtr(displayWidth),
		DefaultDisplayHeight: IntPtr(displayHeight),
		DefaultDisplayFPS:    IntPtr(displayFPS),

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

// IntPtr creates a pointer to an int value
func IntPtr(i int) *int {
	return &i
}