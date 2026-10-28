package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemSettingsResponsePreservesDesktopSandboxOverride(t *testing.T) {
	for _, limit := range []int{0, 17} {
		settings := &SystemSettings{MaxConcurrentDesktopSandboxes: limit}

		response := settings.ToResponseWithSource("", "")

		require.Equal(t, limit, response.MaxConcurrentDesktopSandboxes)
	}
}
