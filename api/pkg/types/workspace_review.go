package types

import "time"

const (
	WorkspaceReviewSourceAll         = "all"
	WorkspaceReviewSourceBranch      = "branch"
	WorkspaceReviewSourceWorkingTree = "working_tree"

	CodeChangesStatusCapturing = "capturing"
	CodeChangesStatusReady     = "ready"
	CodeChangesStatusMissing   = "missing"
	CodeChangesStatusError     = "error"
)

type WorkspaceReviewResponse struct {
	Workspace   string                  `json:"workspace"`
	GeneratedAt time.Time               `json:"generated_at"`
	Sources     []WorkspaceReviewSource `json:"sources"`
}

type WorkspacesResponse struct {
	Workspaces []WorkspaceInfo `json:"workspaces"`
}

type WorkspaceInfo struct {
	Name          string `json:"name"`
	Path          string `json:"-" swaggerignore:"true"`
	AgentPath     string `json:"path"`
	CurrentBranch string `json:"current_branch"`
	IsPrimary     bool   `json:"is_primary"`
	HasHelixSpecs bool   `json:"has_helix_specs"`
}

type WorkspaceReviewSource struct {
	ID             string                      `json:"id"`
	Title          string                      `json:"title"`
	BaseRef        string                      `json:"base_ref,omitempty"`
	HeadRef        string                      `json:"head_ref,omitempty"`
	Patch          string                      `json:"patch"`
	PatchHash      string                      `json:"patch_hash"`
	Truncated      bool                        `json:"truncated"`
	Files          []InteractionCodeChangeFile `json:"files"`
	TotalAdditions int                         `json:"total_additions"`
	TotalDeletions int                         `json:"total_deletions"`
}

type WorkspaceFilesResponse struct {
	Workspace string               `json:"workspace"`
	Entries   []WorkspaceFileEntry `json:"entries"`
	Truncated bool                 `json:"truncated"`
}

type WorkspaceFileEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Size int64  `json:"size,omitempty"`
}

type WorkspaceSkillsResponse struct {
	Skills []WorkspaceSkillEntry `json:"skills"`
}

type WorkspaceSkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope"`
	Path        string `json:"path"`
}

type WorkspaceFileResponse struct {
	Workspace   string `json:"workspace"`
	Path        string `json:"path"`
	Contents    string `json:"contents,omitempty"`
	ByteLength  int64  `json:"byte_length"`
	ContentHash string `json:"content_hash,omitempty"`
	Truncated   bool   `json:"truncated"`
	Binary      bool   `json:"binary"`
}

type InteractionCodeChanges struct {
	Status         string                      `json:"status"`
	Workspace      string                      `json:"workspace,omitempty"`
	BeforeRef      string                      `json:"before_ref,omitempty"`
	AfterRef       string                      `json:"after_ref,omitempty"`
	PatchHash      string                      `json:"patch_hash,omitempty"`
	Files          []InteractionCodeChangeFile `json:"files"`
	TotalAdditions int                         `json:"total_additions"`
	TotalDeletions int                         `json:"total_deletions"`
	CapturedAt     time.Time                   `json:"captured_at,omitempty"`
	Error          string                      `json:"error,omitempty"`
}

type InteractionCodeChangeFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Kind      string `json:"kind"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
}

type WorkspaceCheckpointCaptureRequest struct {
	Workspace string `json:"workspace,omitempty"`
	Ref       string `json:"ref"`
}

type WorkspaceCheckpointCaptureResponse struct {
	Workspace string    `json:"workspace"`
	Ref       string    `json:"ref"`
	Commit    string    `json:"commit"`
	Captured  time.Time `json:"captured_at"`
}

type WorkspaceCheckpointDiffRequest struct {
	Workspace        string `json:"workspace,omitempty"`
	BeforeRef        string `json:"before_ref"`
	AfterRef         string `json:"after_ref"`
	IgnoreWhitespace bool   `json:"ignore_whitespace,omitempty"`
}

type WorkspaceCheckpointDiffResponse struct {
	Workspace      string                      `json:"workspace"`
	BeforeRef      string                      `json:"before_ref"`
	AfterRef       string                      `json:"after_ref"`
	Patch          string                      `json:"patch"`
	PatchHash      string                      `json:"patch_hash"`
	Truncated      bool                        `json:"truncated"`
	Files          []InteractionCodeChangeFile `json:"files"`
	TotalAdditions int                         `json:"total_additions"`
	TotalDeletions int                         `json:"total_deletions"`
}
