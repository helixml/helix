package types

// ReplaceModelRequest moves every saved selection of one provider's model to
// another model on the same provider, for when a provider stops serving a
// model.
type ReplaceModelRequest struct {
	ProviderRef string `json:"provider_ref"`
	FromModel   string `json:"from_model"`
	ToModel     string `json:"to_model"`
	// DryRun reports what would change without writing anything.
	DryRun bool `json:"dry_run"`
}

// ModelReference is one saved selection of the replaced model.
type ModelReference struct {
	// Kind is org_default, agent, org_bot, project, spec_task or session.
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id,omitempty"`
	// Fields names the fields holding the model, e.g. "code_agent_config".
	Fields []string `json:"fields"`
}

type ReplaceModelResponse struct {
	DryRun     bool             `json:"dry_run"`
	References []ModelReference `json:"references"`
}
