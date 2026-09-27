package instances_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/instances"
	"github.com/helixml/helix/api/pkg/types"
)

func TestNormalizeDiskSize(t *testing.T) {
	for requested, want := range map[int]int{
		0:                              types.DefaultBotInstanceDiskSizeGB,
		1:                              1,
		24:                             24,
		types.MaxBotInstanceDiskSizeGB: types.MaxBotInstanceDiskSizeGB,
	} {
		got, err := instances.NormalizeDiskSize(requested)
		if err != nil || got != want {
			t.Fatalf("NormalizeDiskSize(%d) = %d, %v; want %d", requested, got, err, want)
		}
	}
	for _, invalid := range []int{-1, types.MaxBotInstanceDiskSizeGB + 1} {
		if _, err := instances.NormalizeDiskSize(invalid); !errors.Is(err, instances.ErrInvalidRequest) {
			t.Fatalf("NormalizeDiskSize(%d) err = %v, want ErrInvalidRequest", invalid, err)
		}
	}
}

func TestValidateCredentials(t *testing.T) {
	if err := instances.ValidateCredentials(&types.AssistantConfig{CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey}); err != nil {
		t.Fatal(err)
	}
	err := instances.ValidateCredentials(&types.AssistantConfig{CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription})
	if !errors.Is(err, instances.ErrInvalidRequest) || !errors.Is(err, types.ErrBotInstanceSubscriptionCredentials) {
		t.Fatalf("err = %v", err)
	}
}

func TestSelectSecrets(t *testing.T) {
	project := []*types.Secret{
		{Name: "DEV", Scope: types.SecretScopeDev},
		{Name: "LEGACY"},
		{Name: "BOTH", Scope: types.SecretScopeBoth},
		{Name: "PROD", Scope: types.SecretScopeProd},
	}
	selected, err := instances.SelectSecrets([]string{"BOTH", "DEV", "LEGACY"}, project)
	if err != nil || !slices.Equal(selected, []string{"BOTH", "DEV", "LEGACY"}) {
		t.Fatalf("selected = %v, %v", selected, err)
	}
	for _, invalid := range [][]string{{"PROD"}, {"MISSING"}, {"DEV", "DEV"}, {""}} {
		if _, err := instances.SelectSecrets(invalid, project); !errors.Is(err, instances.ErrInvalidRequest) {
			t.Fatalf("SelectSecrets(%v) err = %v, want ErrInvalidRequest", invalid, err)
		}
	}
}
