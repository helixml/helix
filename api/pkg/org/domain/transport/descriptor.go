package transport

import (
	"fmt"
	"strings"
)

// FieldType tells the UI which control to render for a Field. Types
// beyond the primitives name a Helix-specific picker whose options come
// from a live org-scoped list.
type FieldType string

const (
	FieldString         FieldType = "string"
	FieldURL            FieldType = "url"
	FieldStringList     FieldType = "string_list"
	FieldCron           FieldType = "cron"
	FieldGitHubRepo     FieldType = "github_repo"
	FieldGitHubEvents   FieldType = "github_events"
	FieldGitLabRepo     FieldType = "gitlab_repo"
	FieldGitLabEvents   FieldType = "gitlab_events"
	FieldSlackWorkspace FieldType = "slack_workspace"
	FieldSlackChannel   FieldType = "slack_channel"
)

// Direction says whether a Field affects data coming IN to the Trigger
// or going OUT of it. It exists because the webhook Kind's only field is
// outbound, which every user reads as inbound.
type Direction string

const (
	Inbound  Direction = "inbound"
	Outbound Direction = "outbound"
)

// Field is one configurable key inside a Transport's Config blob.
type Field struct {
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Help        string    `json:"help,omitempty"`
	Placeholder string    `json:"placeholder,omitempty"`
	Type        FieldType `json:"type"`
	Required    bool      `json:"required,omitempty"`
	ReadOnly    bool      `json:"read_only,omitempty"`
	Direction   Direction `json:"direction"`
}

// Activation describes how a Trigger of this Kind is fired. Templates use
// {public}, {org}, {id} and {field:<name>} placeholders, resolved per
// Trigger by ResolveActivation.
type Activation struct {
	Summary         string `json:"summary"`
	Verb            string `json:"verb,omitempty"`
	URLTemplate     string `json:"url_template,omitempty"`
	AuthHeader      string `json:"auth_header,omitempty"`
	AddressTemplate string `json:"address_template,omitempty"`
	Note            string `json:"note,omitempty"`
}

// SecretRef names where a Kind's credentials live. Kinds needing no
// credentials return none.
type SecretRef struct {
	Label      string `json:"label"`
	SettingKey string `json:"setting_key,omitempty"`
	Location   string `json:"location"`
}

// Descriptor is everything the UI needs to render and explain one Kind's
// settings. It is returned by that Kind's Strategy, so it lives in the
// same file as the Config type and its Validate rules.
type Descriptor struct {
	Kind          Kind        `json:"kind"`
	Label         string      `json:"label"`
	Summary       string      `json:"summary"`
	Fields        []Field     `json:"fields,omitempty"`
	Activation    Activation  `json:"activation"`
	Secrets       []SecretRef `json:"secrets,omitempty"`
	SystemManaged bool        `json:"system_managed,omitempty"`
}

// DescribeAll returns every registered Kind's Descriptor in canonical
// display order (see kindOrder).
func DescribeAll() []Descriptor {
	out := make([]Descriptor, 0, len(kindOrder))
	for _, k := range kindOrder {
		out = append(out, strategies[k].Describe())
	}
	return out
}

// ActivationParams carries the per-Trigger values that fill an
// Activation's templates.
type ActivationParams struct {
	PublicURL string
	OrgHandle string
	TriggerID string
	Config    map[string]any
}

// ResolvedActivation is an Activation with every template filled in for
// one specific Trigger.
type ResolvedActivation struct {
	Summary    string `json:"summary"`
	Verb       string `json:"verb,omitempty"`
	URL        string `json:"url,omitempty"`
	AuthHeader string `json:"auth_header,omitempty"`
	Address    string `json:"address,omitempty"`
	Note       string `json:"note,omitempty"`
}

// ResolveActivation fills {public}, {org}, {id} and {field:<name>} in the
// Descriptor's Activation templates. An unset field renders as <name> so
// the UI shows a readable gap rather than a raw placeholder.
func ResolveActivation(d Descriptor, p ActivationParams) ResolvedActivation {
	fill := func(tmpl string) string {
		if tmpl == "" {
			return ""
		}
		out := tmpl
		out = strings.ReplaceAll(out, "{public}", strings.TrimRight(p.PublicURL, "/"))
		out = strings.ReplaceAll(out, "{org}", p.OrgHandle)
		out = strings.ReplaceAll(out, "{id}", p.TriggerID)
		for _, f := range d.Fields {
			placeholder := "{field:" + f.Name + "}"
			if !strings.Contains(out, placeholder) {
				continue
			}
			value := "<" + f.Name + ">"
			if raw, ok := p.Config[f.Name]; ok {
				if s := fmt.Sprintf("%v", raw); s != "" {
					value = s
				}
			}
			out = strings.ReplaceAll(out, placeholder, value)
		}
		return out
	}

	return ResolvedActivation{
		Summary:    d.Activation.Summary,
		Verb:       d.Activation.Verb,
		URL:        fill(d.Activation.URLTemplate),
		AuthHeader: d.Activation.AuthHeader,
		Address:    fill(d.Activation.AddressTemplate),
		Note:       d.Activation.Note,
	}
}
