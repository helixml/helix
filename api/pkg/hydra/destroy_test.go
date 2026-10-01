package hydra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsResourceID(t *testing.T) {
	for id, want := range map[string]bool{
		"ses_01abc":    true,
		"ses_":         false,
		"spt_01abc":    false,
		"ses_../etc":   false,
		"ses_a\\b":     false,
		"":             false,
		"xses_01abc":   false,
		"ses_01abc/..": false,
	} {
		assert.Equal(t, want, isResourceID(id, "ses_"), id)
	}
}

func TestDestroyDevContainerRejectsUnsafeIDs(t *testing.T) {
	dm := &DevContainerManager{containers: map[string]*DevContainer{}}

	_, err := dm.DestroyDevContainer(context.Background(), "ses_../../etc", "")
	require.ErrorContains(t, err, "invalid session id")

	_, err = dm.DestroyDevContainer(context.Background(), "ses_01abc", "spt_../x")
	require.ErrorContains(t, err, "invalid spec task id")
}
