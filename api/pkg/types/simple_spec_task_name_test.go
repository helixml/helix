package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSpecTaskName(t *testing.T) {
	require.NoError(t, ValidateSpecTaskName(""))
	require.NoError(t, ValidateSpecTaskName(strings.Repeat("é", SpecTaskNameMaxRunes)))
	require.Error(t, ValidateSpecTaskName(strings.Repeat("x", SpecTaskNameMaxRunes+1)))
}

func TestTruncateSpecTaskName(t *testing.T) {
	require.Equal(t, "short", TruncateSpecTaskName("short"))
	long := strings.Repeat("é", SpecTaskNameMaxRunes+10)
	got := TruncateSpecTaskName(long)
	require.Equal(t, SpecTaskNameMaxRunes, len([]rune(got)))
	require.NoError(t, ValidateSpecTaskName(got))
}
