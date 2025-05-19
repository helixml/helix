package agent

type TokenRates struct {
	Input  float64
	Output float64
}

// Pricing constants for GPT-4o and GPT-4o-mini and O3-mini(in dollars per million tokens)
const (
	GPT4oInputRate      = 2.5
	GPT4oOutputRate     = 10.0
	GPT4oMiniInputRate  = 0.15
	GPT4oMiniOutputRate = 0.60
	O3MiniInputRate     = 1.10
	O3MiniOutputRate    = 4.40
	O1InputRate         = 15.0
	O1OutputRate        = 60.0
	O3InputRate         = 10.0
	O3OutputRate        = 40.0
	O4MiniInputRate     = 1.10
	O4MiniOutputRate    = 4.40
	GPT41InputRate      = 2.0
	GPT41OutputRate     = 8.0
	GPT41MiniInputRate  = 0.40
	GPT41MiniOutputRate = 1.60
	GPT41NanoInputRate  = 0.10
	GPT41NanoOutputRate = 0.40
)

// ModelPricings is a map of model names to their pricing information
var ModelPricings = map[string]TokenRates{
	"gpt-4o": {
		Input:  GPT4oInputRate,
		Output: GPT4oOutputRate,
	},
	"gpt-4o-mini": {
		Input:  GPT4oMiniInputRate,
		Output: GPT4oMiniOutputRate,
	},
	"o3-mini": {
		Input:  O3MiniInputRate,
		Output: O3MiniOutputRate,
	},
	"o1": {
		Input:  O1InputRate,
		Output: O1OutputRate,
	},
	"o3": {
		Input:  O3InputRate,
		Output: O3OutputRate,
	},
	"o4-mini": {
		Input:  O4MiniInputRate,
		Output: O4MiniOutputRate,
	},
	"gpt-4.1": {
		Input:  GPT41InputRate,
		Output: GPT41OutputRate,
	},
	"gpt-4.1-mini": {
		Input:  GPT41MiniInputRate,
		Output: GPT41MiniOutputRate,
	},
	"gpt-4.1-nano": {
		Input:  GPT41NanoInputRate,
		Output: GPT41NanoOutputRate,
	},
	"azure/gpt-4o": {
		Input:  GPT4oInputRate,
		Output: GPT4oOutputRate,
	},
	"azure/gpt-4o-mini": {
		Input:  GPT4oMiniInputRate,
		Output: GPT4oMiniOutputRate,
	},
	"azure/o3-mini": {
		Input:  O3MiniInputRate,
		Output: O3MiniOutputRate,
	},
	"azure/o1": {
		Input:  O1InputRate,
		Output: O1OutputRate,
	},
	"azure/o3": {
		Input:  O3InputRate,
		Output: O3OutputRate,
	},
	"azure/o4-mini": {
		Input:  O4MiniInputRate,
		Output: O4MiniOutputRate,
	},
	"azure/gpt-4.1": {
		Input:  GPT41InputRate,
		Output: GPT41OutputRate,
	},
	"azure/gpt-4.1-mini": {
		Input:  GPT41MiniInputRate,
		Output: GPT41MiniOutputRate,
	},
	"azure/gpt-4.1-nano": {
		Input:  GPT41NanoInputRate,
		Output: GPT41NanoOutputRate,
	},
}

// CostDetails represents detailed cost information for a session
type CostDetails struct {
	InputTokens  int64
	OutputTokens int64
	TotalCost    float64
}

// Cost returns the accumulated cost of the session.
// It calculates the cost based on the total input and output tokens and the pricing for the session's model.
func (s *Session) Cost() (*CostDetails, bool) {
	pricing, exists := ModelPricings[s.llm.ReasoningModel]
	if !exists {
		return nil, false
	}

	inputCost := float64(0) * pricing.Input / 1000000
	outputCost := float64(0) * pricing.Output / 1000000
	totalCost := inputCost + outputCost

	return &CostDetails{
		InputTokens:  0,
		OutputTokens: 0,
		TotalCost:    totalCost,
	}, true
}
