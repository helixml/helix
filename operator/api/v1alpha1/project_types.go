/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectRepositorySpec describes a repository attachment.
type ProjectRepositorySpec struct {
	URL           string `json:"url"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Primary       bool   `json:"primary,omitempty"`
}

// ProjectStartup describes startup commands for the primary repository.
type ProjectStartup struct {
	Install string `json:"install,omitempty"`
	Start   string `json:"start,omitempty"`
}

// ProjectWIPLimits holds per-column WIP limit values.
type ProjectWIPLimits struct {
	Planning       int `json:"planning,omitempty"`
	Implementation int `json:"implementation,omitempty"`
	Review         int `json:"review,omitempty"`
}

// ProjectKanban holds Kanban board settings.
type ProjectKanban struct {
	WIPLimits *ProjectWIPLimits `json:"wip_limits,omitempty"`
}

// ProjectTaskSpec is a task to seed onto the Kanban board.
type ProjectTaskSpec struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// ProjectAgentTools lists the built-in tools to enable for the project agent.
type ProjectAgentTools struct {
	WebSearch  bool `json:"web_search,omitempty"`
	Browser    bool `json:"browser,omitempty"`
	Calculator bool `json:"calculator,omitempty"`
}

// ProjectAgentDisplay configures the virtual desktop for the agent container.
type ProjectAgentDisplay struct {
	// Resolution preset: "1080p" (default), "4k", or "5k"
	Resolution string `json:"resolution,omitempty"`
	// Desktop environment: "ubuntu" (default GNOME) or "sway"
	DesktopType string `json:"desktop_type,omitempty"`
	// Display refresh rate in Hz (default 60)
	FPS int `json:"fps,omitempty"`
}

// ProjectAgentSpec is the simplified agent configuration.
//
// Runtime selects the code agent inside the Zed desktop container.
// Defaults to "claude_code" when omitted (recommended — handles context compaction).
//   - "claude_code" — Claude Code CLI (default)
//   - "zed"         — Zed's built-in agent panel
//   - "qwen_code"   — Qwen Code CLI
//   - "gemini_cli"  — Gemini CLI
//   - "codex_cli"   — OpenAI Codex CLI
type ProjectAgentSpec struct {
	Name        string             `json:"name,omitempty"`
	Runtime     string             `json:"runtime,omitempty"`
	Model       string             `json:"model,omitempty"`
	Provider    string             `json:"provider,omitempty"`
	Credentials string             `json:"credentials,omitempty"`
	Tools       *ProjectAgentTools `json:"tools,omitempty"`
	Display     *ProjectAgentDisplay `json:"display,omitempty"`
}

// ProjectSpec defines the desired state of a Project.
type ProjectSpec struct {
	Description  string                  `json:"description,omitempty"`
	Guidelines   string                  `json:"guidelines,omitempty"`
	Repository   *ProjectRepositorySpec  `json:"repository,omitempty"`
	Repositories []ProjectRepositorySpec `json:"repositories,omitempty"`
	Startup      *ProjectStartup         `json:"startup,omitempty"`
	Kanban       *ProjectKanban          `json:"kanban,omitempty"`
	Tasks        []ProjectTaskSpec       `json:"tasks,omitempty"`
	Agent        *ProjectAgentSpec       `json:"agent,omitempty"`
}

// ProjectStatus defines the observed state of a Project.
type ProjectStatus struct {
	// Ready indicates whether the project was successfully applied.
	Ready bool `json:"ready,omitempty"`
	// ProjectID is the Helix project ID.
	ProjectID string `json:"project_id,omitempty"`
	// AgentAppID is the linked Helix agent app ID.
	AgentAppID string `json:"agent_app_id,omitempty"`
	// LastSynced is the timestamp of the last successful sync.
	LastSynced *metav1.Time `json:"last_synced,omitempty"`
	// Message holds any error or status message.
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="ProjectID",type="string",JSONPath=".status.project_id"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Project is the Schema for the projects API.
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec,omitempty"`
	Status ProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
