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

	price, err := CalculateTokenPrice(mf, 1000, 1000)
	assert.NoError(t, err)
	assert.Equal(t, 0.000225, price)
}
