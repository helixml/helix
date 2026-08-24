package transport_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Every Kind must be described, in canonical order.
func TestDescribeAll_CoversEveryKindInOrder(t *testing.T) {
	t.Parallel()

	kinds := transport.KindValues()
	got := transport.DescribeAll()
	if len(got) != len(kinds) {
		t.Fatalf("DescribeAll returned %d descriptors, want %d", len(got), len(kinds))
	}
	for i, k := range kinds {
		if got[i].Kind != k {
			t.Errorf("descriptor %d is %q, want %q", i, got[i].Kind, k)
		}
		if got[i].Label == "" {
			t.Errorf("kind %q has an empty Label", k)
		}
		if got[i].Summary == "" {
			t.Errorf("kind %q has an empty Summary", k)
		}
	}
}

// Descriptor-vs-validator parity: a field the descriptor marks Required
// must actually be demanded by Validate(). This guards drift between the
// two, which sit in the same file precisely so they stay in step.
func TestDescriptorRequiredFieldsAreEnforcedByValidate(t *testing.T) {
	t.Parallel()

	for _, d := range transport.DescribeAll() {
		for _, f := range d.Fields {
			if !f.Required {
				continue
			}
			cfg := map[string]any{}
			for _, other := range d.Fields {
				if other.Required && other.Name != f.Name {
					cfg[other.Name] = sampleFor(other)
				}
			}
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("kind %q: marshal config: %v", d.Kind, err)
			}
			tr := transport.Transport{Kind: d.Kind, Config: raw}
			if err := tr.Validate(); err == nil {
				t.Errorf("kind %q: field %q is Required in the descriptor but Validate() accepts a config without it",
					d.Kind, f.Name)
			}
		}
	}
}

// sampleFor returns a value that satisfies the field's own validation, so
// omitting a DIFFERENT field is the only reason Validate can fail.
func sampleFor(f transport.Field) any {
	switch f.Type {
	case transport.FieldGitHubRepo:
		return "helixml/helix"
	case transport.FieldGitHubEvents:
		return []string{"*"}
	case transport.FieldGitLabEvents:
		return []string{"Merge Request Hook"}
	case transport.FieldStringList:
		return []string{"*"}
	case transport.FieldCron:
		return "0 9 * * 1"
	case transport.FieldURL:
		return "https://example.com/hook"
	default:
		return "sample"
	}
}

// --- ResolveActivation ---------------------------------------------------

func descriptorFor(t *testing.T, kind transport.Kind) transport.Descriptor {
	t.Helper()
	for _, d := range transport.DescribeAll() {
		if d.Kind == kind {
			return d
		}
	}
	t.Fatalf("no descriptor for kind %q", kind)
	return transport.Descriptor{}
}

func TestResolveActivation_FillsTemplates(t *testing.T) {
	t.Parallel()

	got := transport.ResolveActivation(descriptorFor(t, transport.KindWebhook), transport.ActivationParams{
		PublicURL: "https://helix.example.com",
		OrgHandle: "acme",
		TriggerID: "tr-123",
	})

	wantURL := "https://helix.example.com/api/v1/orgs/acme/webhooks/tr-123"
	if got.URL != wantURL {
		t.Errorf("URL = %q, want %q", got.URL, wantURL)
	}
	if got.Verb != "POST" {
		t.Errorf("Verb = %q, want POST", got.Verb)
	}
	if got.AuthHeader == "" {
		t.Error("AuthHeader is empty, want the API key header")
	}
}

func TestResolveActivation_FillsFieldPlaceholdersFromConfig(t *testing.T) {
	t.Parallel()

	got := transport.ResolveActivation(descriptorFor(t, transport.KindEmail), transport.ActivationParams{
		Config: map[string]any{"alias": "support"},
	})

	if got.Address != "support@<your inbound domain>" {
		t.Errorf("Address = %q, want the alias substituted", got.Address)
	}
}

func TestResolveActivation_TrailingSlashOnPublicURLDoesNotDouble(t *testing.T) {
	t.Parallel()

	got := transport.ResolveActivation(descriptorFor(t, transport.KindWebhook), transport.ActivationParams{
		PublicURL: "https://helix.example.com/",
		OrgHandle: "acme",
		TriggerID: "tr-123",
	})

	if strings.Contains(got.URL, "//api/") {
		t.Errorf("URL = %q, want no doubled slash", got.URL)
	}
}

func TestResolveActivation_MissingFieldLeavesReadablePlaceholder(t *testing.T) {
	t.Parallel()

	got := transport.ResolveActivation(descriptorFor(t, transport.KindEmail), transport.ActivationParams{})

	if got.Address != "<alias>@<your inbound domain>" {
		t.Errorf("Address = %q, want an unset alias rendered as <alias>", got.Address)
	}
}

func TestResolveActivation_LocalHasSummaryButNoURL(t *testing.T) {
	t.Parallel()

	got := transport.ResolveActivation(descriptorFor(t, transport.KindLocal), transport.ActivationParams{
		PublicURL: "https://helix.example.com",
		OrgHandle: "acme",
		TriggerID: "s-general",
	})

	if got.Summary == "" {
		t.Error("Summary is empty")
	}
	if got.URL != "" {
		t.Errorf("URL = %q, want empty for a local trigger", got.URL)
	}
}
