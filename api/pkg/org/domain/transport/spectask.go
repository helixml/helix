package transport

import (
	"encoding/json"
	"errors"
)

// KindSpecTask is an inbound-only, project-scoped transport: a Topic of
// this Kind carries a Helix project's spec-task attention events (spec
// ready for review, PR opened, CI passed/failed, …) so subscribed
// Workers are triggered when a human would be notified. The event source
// is the AttentionService — the same set that drives the Helix UI
// notifications — not the raw per-change pubsub.
const KindSpecTask Kind = "spectask"

// SpecTaskConfig binds the Topic to the project whose spec-task events it
// carries. ProjectID is required.
type SpecTaskConfig struct {
	ProjectID string `json:"project_id"`
}

// Validate requires a project id — a spec-task topic with no project has
// nothing to ingest.
func (c SpecTaskConfig) Validate() error {
	if c.ProjectID == "" {
		return errors.New("spectask transport requires project_id")
	}
	return nil
}

// specTask is the Strategy for KindSpecTask.
type specTask struct{}

// ParseConfig decodes the project-scoped config blob.
func (specTask) ParseConfig(raw json.RawMessage) (Config, error) {
	var c SpecTaskConfig
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// SpecTaskConfig returns the typed config for a KindSpecTask Transport.
func (t Transport) SpecTaskConfig() (SpecTaskConfig, error) {
	if t.Kind != KindSpecTask {
		return SpecTaskConfig{}, errors.New("transport kind is not spectask")
	}
	c, err := specTask{}.ParseConfig(t.Config)
	if err != nil {
		return SpecTaskConfig{}, err
	}
	return c.(SpecTaskConfig), nil
}
