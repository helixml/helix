package pricing

import (
	"strconv"

	"github.com/helixml/helix/api/pkg/types"
)

// CalculateTokenPrice calculates the price of a token based on the model info for LLM calls
func CalculateTokenPrice(mf *types.ModelInfo, promptTokens int64, completionTokens int64) (float64, error) {
	var tokenPrice float64

	if mf.Pricing.Prompt != "" {
		promptPrice, err := strconv.ParseFloat(mf.Pricing.Prompt, 64)
		if err != nil {
			return 0, err
		}
		tokenPrice += promptPrice * float64(promptTokens)
	}

	if mf.Pricing.Completion != "" {
		completionPrice, err := strconv.ParseFloat(mf.Pricing.Completion, 64)
		if err != nil {
			return 0, err
		}
		tokenPrice += completionPrice * float64(completionTokens)
	}

	return tokenPrice, nil
}
