package pricing

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
)

func TestCalculateTokenPrice(t *testing.T) {

	mf := &types.ModelInfo{
		Pricing: types.Pricing{
			Prompt:     "0.000000075",
			Completion: "0.00000015",
		},
	}

	promptCost, completionCost, err := CalculateTokenPrice(mf, 10000000, 10000000)
	assert.NoError(t, err)
	assert.Equal(t, 0.75, promptCost)
	assert.Equal(t, 1.5, completionCost)
}
