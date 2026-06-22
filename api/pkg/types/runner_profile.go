package types

import "time"

// GPUVendor identifies the silicon family a profile or runner GPU belongs to.
// Empty string means "any vendor" in profile compatibility specifications.
type GPUVendor string

const (
	GPUVendorNVIDIA GPUVendor = "nvidia"
	GPUVendorAMD    GPUVendor = "amd"
	// GPUVendorNeuron covers all AWS neuronx silicon (Inferentia2 inf2.*
	// SKUs and Trainium trn1/trn2) — same device-node family and serving
	// path, so one vendor value. inf1 (older neuron-cc runtime) is not
	// supported.
	GPUVendorNeuron GPUVendor = "neuron"
)

// ProfileModel describes one model exposed by a profile, derived by parsing
// the compose YAML on save. Names + container/port let the runner's
// model-aware reverse proxy route requests.
type ProfileModel struct {
	Name          string `json:"name"`           // --served-model-name (preferred) or --model basename
	ContainerName string `json:"container_name"` // service.container_name (or service key if absent)
	InternalPort  int    `json:"internal_port"`  // first published or exposed port
}

// ProfileGPURequirement is operator-declared compatibility metadata. Only
// Count is auto-derived from the compose YAML (union of device_ids); the
// other four fields are entered separately by the operator at profile-save
// time. All four optional fields compose with AND semantics.
type ProfileGPURequirement struct {
	Count         int       `json:"count"`                    // derived from compose: union of device_ids
	Vendor        GPUVendor `json:"vendor,omitempty"`         // optional
	Architectures []string  `json:"architectures,omitempty"`  // optional whitelist (canonical strings from gpuarch)
	ModelMatch    string    `json:"model_match,omitempty"`    // optional regex against GPU marketing name
	MinVRAMBytes  int64     `json:"min_vram_bytes,omitempty"` // optional, per-GPU minimum
}

// RunnerProfile is a named compose-based configuration applied to a Sandbox.
// The compose YAML is the source of truth; Models and GPURequirement.Count
// are derived from it on save. Vendor / Architectures / ModelMatch /
// MinVRAMBytes are operator-declared.
//
// Naming note: kept as `RunnerProfile` (rather than renamed to
// `SandboxProfile` or just `Profile`) to avoid churn on the GORM table
// name and on already-shipped tests. Conceptually it is "the compose
// profile assigned to a sandbox" — see Decision 12 in design.md for the
// architectural pivot that absorbs the runner role into Sandbox.
type RunnerProfile struct {
	ID             string                `json:"id" gorm:"primaryKey"`
	Name           string                `json:"name" gorm:"uniqueIndex;not null"`
	Description    string                `json:"description"`
	ComposeYAML    string                `json:"compose_yaml" gorm:"type:text;not null"`
	Models         []ProfileModel        `json:"models" gorm:"type:jsonb;serializer:json"`
	GPURequirement ProfileGPURequirement `json:"gpu_requirement" gorm:"type:jsonb;serializer:json"`
	CreatedAt      time.Time             `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time             `json:"updated_at" gorm:"autoUpdateTime"`
}

func (RunnerProfile) TableName() string { return "runner_profiles" }

// RunnerAssignment maps a runner to its currently-assigned profile. Persisted
// so that a runner reconnecting after restart re-applies its profile
// automatically. RunnerID is the primary key — a runner has at most one
// active assignment at any time.
type RunnerAssignment struct {
	RunnerID   string    `json:"runner_id" gorm:"primaryKey"`
	ProfileID  string    `json:"profile_id" gorm:"index"`
	AssignedAt time.Time `json:"assigned_at" gorm:"autoCreateTime"`
	AssignedBy string    `json:"assigned_by"` // user ID for audit
}

func (RunnerAssignment) TableName() string { return "runner_assignments" }
