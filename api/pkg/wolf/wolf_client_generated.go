// Generated from Wolf OpenAPI specification
// This file contains properly typed Wolf API structures based on the authoritative
// OpenAPI 3.1 specification fetched from Wolf at runtime via Unix socket.
//
// Key discoveries from OpenAPI spec analysis:
// - ALL GStreamer pipeline fields are REQUIRED by Wolf API (not optional)
// - Empty strings should be sent for default GStreamer pipeline configuration
// - Docker runner fields (mounts, env, devices, ports) are required arrays, even if empty
// - IconPngPath can be null and should use pointer with omitempty
//
// The main client.go file uses type aliases pointing to these generated types
// for backward compatibility while ensuring OpenAPI spec compliance.
package wolf

// WolfApp represents a Wolf application configuration based on OpenAPI spec
// Schema: rfl__Reflector_wolf__core__events__App___ReflType
type WolfApp struct {
	// Required fields according to Wolf OpenAPI spec
	Title                  string    `json:"title"`
	ID                     string    `json:"id"`
	SupportHDR             bool      `json:"support_hdr"`
	H264GstPipeline        string    `json:"h264_gst_pipeline"`        // REQUIRED by Wolf API
	HEVCGstPipeline        string    `json:"hevc_gst_pipeline"`        // REQUIRED by Wolf API
	AV1GstPipeline         string    `json:"av1_gst_pipeline"`         // REQUIRED by Wolf API
	RenderNode             string    `json:"render_node"`              // REQUIRED by Wolf API
	OpusGstPipeline        string    `json:"opus_gst_pipeline"`        // REQUIRED by Wolf API
	StartVirtualCompositor bool      `json:"start_virtual_compositor"` // REQUIRED by Wolf API
	StartAudioServer       bool      `json:"start_audio_server"`       // REQUIRED by Wolf API
	Runner                 WolfRunner `json:"runner"`                   // REQUIRED by Wolf API

	// Optional fields
	IconPngPath *string `json:"icon_png_path,omitempty"` // Can be null according to anyOf
}

// WolfRunner represents a Wolf runner configuration based on OpenAPI spec
// Union type supporting process, docker, or child_session
type WolfRunner struct {
	// Common fields
	Type string `json:"type"` // "process", "docker", or "child_session"

	// Process runner fields (wolf__config__AppCMD__tagged)
	RunCmd *string `json:"run_cmd,omitempty"`

	// Docker runner fields (wolf__config__AppDocker__tagged)
	Name           *string  `json:"name,omitempty"`
	Image          *string  `json:"image,omitempty"`
	Mounts         []string `json:"mounts,omitempty"`         // REQUIRED for docker type
	Env            []string `json:"env,omitempty"`            // REQUIRED for docker type
	Devices        []string `json:"devices,omitempty"`        // REQUIRED for docker type
	Ports          []string `json:"ports,omitempty"`          // REQUIRED for docker type
	BaseCreateJSON *string  `json:"base_create_json,omitempty"` // Can be null

	// Child session runner fields (wolf__config__AppChildSession__tagged)
	ParentSessionID *string `json:"parent_session_id,omitempty"`
}

// WolfClientSettings represents client settings based on OpenAPI spec
type WolfClientSettings struct {
	RunUID               int      `json:"run_uid"`
	RunGID               int      `json:"run_gid"`
	ControllersOverride  []string `json:"controllers_override"`  // Array of "XBOX", "PS", "NINTENDO", "AUTO"
	MouseAcceleration    float64  `json:"mouse_acceleration"`
	VScrollAcceleration  float64  `json:"v_scroll_acceleration"`
	HScrollAcceleration  float64  `json:"h_scroll_acceleration"`
}

// WolfStreamSession represents a stream session based on OpenAPI spec
// Schema: rfl__Reflector_wolf__core__events__StreamSession___ReflType
type WolfStreamSession struct {
	AppID             string             `json:"app_id"`
	ClientID          string             `json:"client_id"`
	ClientIP          string             `json:"client_ip"`
	ClientUniqueID    string             `json:"client_unique_id"` // Moonlight client unique ID
	AESKey            string             `json:"aes_key"`
	AESIV             string             `json:"aes_iv"`
	RTSPFakeIP        string             `json:"rtsp_fake_ip"`
	VideoWidth        int                `json:"video_width"`
	VideoHeight       int                `json:"video_height"`
	VideoRefreshRate  int                `json:"video_refresh_rate"`
	AudioChannelCount int                `json:"audio_channel_count"`
	ClientSettings    WolfClientSettings `json:"client_settings"`
}

// Response types from Wolf API

// WolfGenericResponse represents Wolf's generic success/error response
type WolfGenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// WolfSessionResponse represents Wolf's session creation response
type WolfSessionResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id"`
}

// WolfAppListResponse represents Wolf's app list response
type WolfAppListResponse struct {
	Success bool       `json:"success"`
	Apps    []WolfApp  `json:"apps"`
}

// Request types for Wolf API

// WolfAppDeleteRequest represents Wolf's app deletion request
type WolfAppDeleteRequest struct {
	ID string `json:"id"`
}

// Helper functions

// StringPtr returns a pointer to the given string value (for optional fields)
func StringPtr(s string) *string {
	return &s
}

func BoolPtr(b bool) *bool {
	return &b
}

// Helper constructors to ensure required fields are set

// NewDockerApp creates a new Docker-based Wolf app with all required fields
func NewDockerApp(id, title, image, name string) *WolfApp {
	return &WolfApp{
		ID:                     id,
		Title:                  title,
		SupportHDR:             false,
		H264GstPipeline:        "", // Required by Wolf - empty string uses defaults
		HEVCGstPipeline:        "", // Required by Wolf - empty string uses defaults
		AV1GstPipeline:         "", // Required by Wolf - empty string uses defaults
		RenderNode:             "", // Required by Wolf - empty string uses defaults
		OpusGstPipeline:        "", // Required by Wolf - empty string uses defaults
		StartVirtualCompositor: true,
		StartAudioServer:       true,
		Runner: WolfRunner{
			Type:    "docker",
			Name:    StringPtr(name),
			Image:   StringPtr(image),
			Mounts:  []string{}, // Required for docker type
			Env:     []string{}, // Required for docker type
			Devices: []string{}, // Required for docker type
			Ports:   []string{}, // Required for docker type
		},
	}
}

// NewProcessApp creates a new process-based Wolf app with all required fields
func NewProcessApp(id, title, runCmd string) *WolfApp {
	return &WolfApp{
		ID:                     id,
		Title:                  title,
		SupportHDR:             false,
		H264GstPipeline:        "", // Required by Wolf - empty string uses defaults
		HEVCGstPipeline:        "", // Required by Wolf - empty string uses defaults
		AV1GstPipeline:         "", // Required by Wolf - empty string uses defaults
		RenderNode:             "", // Required by Wolf - empty string uses defaults
		OpusGstPipeline:        "", // Required by Wolf - empty string uses defaults
		StartVirtualCompositor: true,
		StartAudioServer:       true,
		Runner: WolfRunner{
			Type:   "process",
			RunCmd: StringPtr(runCmd),
		},
	}
}

// NewCustomDockerApp creates a Docker-based Wolf app with custom configuration
func NewCustomDockerApp(id, title, image, name string, env, mounts, devices, ports []string, baseCreateJSON string) *WolfApp {
	app := NewDockerApp(id, title, image, name)

	// Customize the Docker runner configuration
	app.Runner.Env = env
	app.Runner.Mounts = mounts
	app.Runner.Devices = devices
	app.Runner.Ports = ports
	if baseCreateJSON != "" {
		app.Runner.BaseCreateJSON = StringPtr(baseCreateJSON)
	}

	return app
}