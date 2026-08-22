package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEffectiveAgentToolsUnionsAndDedupes(t *testing.T) {
	assert.Equal(t,
		[]string{"create_spectask", "get_spectask", "list_spectasks"},
		EffectiveAgentTools([]string{"list_spectasks", "create_spectask"}, []string{"create_spectask", "get_spectask"}))
}

func TestEffectiveAgentToolsDropsEmptyAndHandlesNil(t *testing.T) {
	assert.Equal(t, []string{"get_spectask"}, EffectiveAgentTools(nil, []string{"", "get_spectask"}))
	assert.Empty(t, EffectiveAgentTools(nil, nil))
}
