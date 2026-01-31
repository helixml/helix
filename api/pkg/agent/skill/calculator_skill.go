package skill

import (
	"context"
	"fmt"
	"strconv"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"

	"github.com/dop251/goja"
)

const calculatorMainPrompt = `You are an expert calculator that can evaluate mathematical expressions and perform complex calculations. Your role is to help users with mathematical computations by evaluating JavaScript expressions.

Key responsibilities:
1. Expression Evaluation:
   - Accept JavaScript mathematical expressions from users
   - Evaluate expressions safely using a JavaScript engine
   - Handle basic arithmetic operations (+, -, *, /, %)
   - Support mathematical functions (Math.sqrt, Math.pow, Math.sin, Math.cos, etc.)
   - Handle parentheses and operator precedence correctly

2. Result Presentation:
   - Present results in a clear, readable format
   - Include the original expression for reference
   - Format numbers appropriately (integers vs decimals)
   - Handle special cases like division by zero or invalid expressions

3. Safety and Validation:
   - Only evaluate mathematical expressions
   - Avoid executing arbitrary code or accessing system resources
   - Provide helpful error messages for invalid expressions
   - Ensure calculations are accurate and reliable

Best Practices:
- Always verify the expression is mathematical in nature
- Use appropriate precision for decimal calculations
- Provide clear explanations for complex calculations
- Handle edge cases gracefully (division by zero, invalid syntax, etc.)
- When using the calculator tool, choose the most appropriate mathematical expression to solve the user's problem

When using the calculator tool:
- Use the tool for any mathematical computation needed
- Express the calculation as a valid JavaScript mathematical expression
- Ensure the expression is safe and doesn't contain arbitrary code execution
- Present results clearly with the original expression for context

Remember: Your goal is to provide accurate mathematical results while maintaining safety and clarity for the user. 
You might receive a long conversation, identify the last required calculation and return the result.`

var calculatorSkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"expression": {
			Type:        jsonschema.String,
			Description: "The JavaScript mathematical expression to evaluate (e.g., '2 + 2', 'Math.sqrt(16)', '10 * (5 + 3)')",
		},
	},
	Required: []string{"expression"},
}

// NewCalculatorSkill creates a new calculator skill that can evaluate JavaScript expressions
func NewCalculatorSkill() agent.Skill {
	return agent.Skill{
		Name:         "Calculator",
		Description:  "Evaluate mathematical expressions using JavaScript",
		SystemPrompt: calculatorMainPrompt,
		Parameters:   calculatorSkillParameters,
		Direct:       true,
		Tools: []agent.Tool{
			&CalculatorTool{},
		},
	}
}

type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
	return "Calculator"
}

func (t *CalculatorTool) Description() string {
	return "Evaluate mathematical expressions using JavaScript"
}

func (t *CalculatorTool) String() string {
	return "Calculator"
}

func (t *CalculatorTool) StatusMessage() string {
	return "Calculating mathematical expression"
}

func (t *CalculatorTool) Icon() string {
	return "CalculateIcon"
}

func (t *CalculatorTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "Calculator",
				Description: "Evaluate mathematical expressions using JavaScript",
				Parameters:  calculatorSkillParameters,
			},
		},
	}
}

func (t *CalculatorTool) Execute(_ context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	expression, ok := args["expression"].(string)
	if !ok {
		return "", fmt.Errorf("expression is required")
	}

	log.Info().Str("expression", expression).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Str("interaction_id", meta.InteractionID).
		Str("app_id", meta.AppID).
		Msg("Executing calculator tool")

	if expression == "" {
		return "", fmt.Errorf("expression is required")
	}

	// Create a new JavaScript runtime
	vm := goja.New()

	// Evaluate the expression
	result, err := vm.RunString(expression)
	if err != nil {
		log.Warn().Err(err).Str("expression", expression).Msg("Error evaluating expression")
		return "", fmt.Errorf("error evaluating expression '%s': %w", expression, err)
	}

	// Convert the result to a string representation
	var resultStr string
	if goja.IsNumber(result) {
		// Handle numbers with appropriate formatting
		if result.ToFloat() == float64(int(result.ToFloat())) {
			// It's an integer
			resultStr = strconv.FormatInt(int64(result.ToFloat()), 10)
		} else {
			// It's a decimal
			resultStr = strconv.FormatFloat(result.ToFloat(), 'f', -1, 64)
		}
	} else if goja.IsString(result) {
		resultStr = result.String()
	} else {
		// For boolean and other types, use the default string representation
		resultStr = result.String()
	}

	// Format the response
	response := resultStr

	return response, nil
}
