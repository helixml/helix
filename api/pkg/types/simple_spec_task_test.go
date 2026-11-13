package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSpecTaskTypeAndPriority(t *testing.T) {
	for _, typ := range []string{"", "feature", "bug", "refactor"} {
		for _, p := range []SpecTaskPriority{"", "low", "medium", "high", "critical"} {
			require.NoError(t, ValidateSpecTaskTypeAndPriority(typ, p), "type=%q priority=%q", typ, p)
		}
	}
	require.Error(t, ValidateSpecTaskTypeAndPriority("<script>alert(1)</script>", ""))
	require.Error(t, ValidateSpecTaskTypeAndPriority("Feature", ""))
	require.Error(t, ValidateSpecTaskTypeAndPriority("", "asap"))
	require.Error(t, ValidateSpecTaskTypeAndPriority("", "HIGH"))
}
