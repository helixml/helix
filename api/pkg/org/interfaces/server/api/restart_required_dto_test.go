package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The frontend keys off restart_required; renaming the JSON tag silently
// breaks the banner, so pin it.
func TestBotDTO_RestartRequiredJSONTag(t *testing.T) {
	raw, err := json.Marshal(BotDTO{ID: "b-one", RestartRequired: true})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, true, decoded["restart_required"])
}

func TestBotDTO_RestartRequiredOmittedWhenFalse(t *testing.T) {
	raw, err := json.Marshal(BotDTO{ID: "b-one"})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, present := decoded["restart_required"]
	require.False(t, present)
}
