// Generated from corrected Wolf OpenAPI specification
// This file contains properly typed Wolf API structures based on the corrected
// OpenAPI spec that fixes the Docker runner field requirements.
//
// Key corrections from original Wolf OpenAPI spec:
// - Docker runner name and image fields are required strings (not optional pointers)
// - Docker runner mounts, env, devices, ports arrays are required (no omitempty)
// - Only base_create_json is optional for Docker runners
package wolf

// WolfAppCorrected represents a Wolf application configuration based on corrected OpenAPI spec
type WolfAppCorrected struct {
	// Required fields according to Wolf OpenAPI spec
	Title                  string             `json:"title"`
	ID                     string             `json:"id"`
	SupportHDR             bool               `json:"support_hdr"`
	H264GstPipeline        string             `json:"h264_gst_pipeline"`        // REQUIRED by Wolf API
	HEVCGstPipeline        string             `json:"hevc_gst_pipeline"`        // REQUIRED by Wolf API
	AV1GstPipeline         string             `json:"av1_gst_pipeline"`         // REQUIRED by Wolf API
	RenderNode             string             `json:"render_node"`              // REQUIRED by Wolf API
	OpusGstPipeline        string             `json:"opus_gst_pipeline"`        // REQUIRED by Wolf API
	StartVirtualCompositor bool               `json:"start_virtual_compositor"` // REQUIRED by Wolf API
	StartAudioServer       bool               `json:"start_audio_server"`       // REQUIRED by Wolf API
	Runner                 WolfRunnerCorrected `json:"runner"`                   // REQUIRED by Wolf API

	// Optional fields
	IconPngPath *string `json:"icon_png_path,omitempty"` // Can be null according to spec
}

// WolfRunnerCorrected represents a Wolf runner configuration with corrected field types
type WolfRunnerCorrected struct {
	// Common fields
	Type string `json:"type"` // "process", "docker", or "child_session"

	// Process runner fields
	RunCmd *string `json:"run_cmd,omitempty"`

	// Docker runner fields - CORRECTED: name and image are required strings, not pointers
	Name    string   `json:"name,omitempty"`           // Required string for docker type
	Image   string   `json:"image,omitempty"`          // Required string for docker type
	Mounts  []string `json:"mounts"`                   // Required array for docker type
	Env     []string `json:"env"`                      // Required array for docker type
	Devices []string `json:"devices"`                  // Required array for docker type
	Ports   []string `json:"ports"`                    // Required array for docker type
	BaseCreateJSON *string `json:"base_create_json,omitempty"` // Optional for docker type

	// Child session runner fields
	ParentSessionID *string `json:"parent_session_id,omitempty"`
}

// NewCorrectedDockerApp creates a new Docker-based Wolf app with corrected field types
func NewCorrectedDockerApp(id, title, image, name string, env, mounts, devices, ports []string, baseCreateJSON string) *WolfAppCorrected {
	app := &WolfAppCorrected{
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
		Runner: WolfRunnerCorrected{
			Type:    "docker",
			Name:    name,    // Required string, not pointer
			Image:   image,   // Required string, not pointer
			Mounts:  mounts,  // Required array
			Env:     env,     // Required array
			Devices: devices, // Required array
			Ports:   ports,   // Required array
		},
	}

	// Set optional BaseCreateJSON if provided
	if baseCreateJSON != "" {
		app.Runner.BaseCreateJSON = StringPtr(baseCreateJSON)
	}

	return app
}

// NewCorrectedProcessApp creates a new process-based Wolf app with corrected field types
func NewCorrectedProcessApp(id, title, runCmd string) *WolfAppCorrected {
	return &WolfAppCorrected{
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
		Runner: WolfRunnerCorrected{
			Type:   "process",
			RunCmd: StringPtr(runCmd),
		},
	}
}