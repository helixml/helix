package evaluation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// LLMJudge is an interface for calling an LLM to judge response correctness
type LLMJudge interface {
	Judge(ctx context.Context, question, response, criteria string) (passed bool, reasoning string, err error)
}

// CheckAssertion evaluates a single assertion against a response
func CheckAssertion(ctx context.Context, assertion types.EvaluationAssertion, response string, skillsUsed []string, judge LLMJudge) types.EvaluationAssertionResult {
	switch assertion.Type {
	case types.EvaluationAssertionTypeContains:
		return checkContains(assertion, response)
	case types.EvaluationAssertionTypeNotContains:
		return checkNotContains(assertion, response)
	case types.EvaluationAssertionTypeRegex:
		return checkRegex(assertion, response)
	case types.EvaluationAssertionTypeLLMJudge:
		return checkLLMJudge(ctx, assertion, response, judge)
	case types.EvaluationAssertionTypeSkillUsed:
		return checkSkillUsed(assertion, skillsUsed)
	default:
		return types.EvaluationAssertionResult{
			AssertionType:  assertion.Type,
			AssertionValue: assertion.Value,
			Passed:         false,
			Details:        fmt.Sprintf("unknown assertion type: %s", assertion.Type),
		}
	}
}

// CheckAllAssertions evaluates all assertions for a question and returns results + overall pass
func CheckAllAssertions(ctx context.Context, assertions []types.EvaluationAssertion, response string, skillsUsed []string, judge LLMJudge) ([]types.EvaluationAssertionResult, bool) {
	if len(assertions) == 0 {
		return nil, true
	}

	results := make([]types.EvaluationAssertionResult, len(assertions))
	allPassed := true
	for i, assertion := range assertions {
		results[i] = CheckAssertion(ctx, assertion, response, skillsUsed, judge)
		if !results[i].Passed {
			allPassed = false
		}
	}
	return results, allPassed
}

func checkContains(assertion types.EvaluationAssertion, response string) types.EvaluationAssertionResult {
	passed := strings.Contains(strings.ToLower(response), strings.ToLower(assertion.Value))
	details := ""
	if !passed {
		details = fmt.Sprintf("response does not contain %q", assertion.Value)
	}
	return types.EvaluationAssertionResult{
		AssertionType:  assertion.Type,
		AssertionValue: assertion.Value,
		Passed:         passed,
		Details:        details,
	}
}

func checkNotContains(assertion types.EvaluationAssertion, response string) types.EvaluationAssertionResult {
	passed := !strings.Contains(strings.ToLower(response), strings.ToLower(assertion.Value))
	details := ""
	if !passed {
		details = fmt.Sprintf("response contains %q but should not", assertion.Value)
	}
	return types.EvaluationAssertionResult{
		AssertionType:  assertion.Type,
		AssertionValue: assertion.Value,
		Passed:         passed,
		Details:        details,
	}
}

func checkRegex(assertion types.EvaluationAssertion, response string) types.EvaluationAssertionResult {
	re, err := regexp.Compile(assertion.Value)
	if err != nil {
		return types.EvaluationAssertionResult{
			AssertionType:  assertion.Type,
			AssertionValue: assertion.Value,
			Passed:         false,
			Details:        fmt.Sprintf("invalid regex: %s", err),
		}
	}
	passed := re.MatchString(response)
	details := ""
	if !passed {
		details = fmt.Sprintf("response does not match regex %q", assertion.Value)
	}
	return types.EvaluationAssertionResult{
		AssertionType:  assertion.Type,
		AssertionValue: assertion.Value,
		Passed:         passed,
		Details:        details,
	}
}

func checkLLMJudge(ctx context.Context, assertion types.EvaluationAssertion, response string, judge LLMJudge) types.EvaluationAssertionResult {
	if judge == nil {
		return types.EvaluationAssertionResult{
			AssertionType:  assertion.Type,
			AssertionValue: assertion.Value,
			Passed:         false,
			Details:        "LLM judge not available",
		}
	}

	criteria := assertion.Value
	if assertion.LLMJudgePrompt != "" {
		criteria = assertion.LLMJudgePrompt
	}

	passed, reasoning, err := judge.Judge(ctx, "", response, criteria)
	if err != nil {
		return types.EvaluationAssertionResult{
			AssertionType:  assertion.Type,
			AssertionValue: assertion.Value,
			Passed:         false,
			Details:        fmt.Sprintf("LLM judge error: %s", err),
		}
	}

	return types.EvaluationAssertionResult{
		AssertionType:  assertion.Type,
		AssertionValue: assertion.Value,
		Passed:         passed,
		Details:        reasoning,
	}
}

func checkSkillUsed(assertion types.EvaluationAssertion, skillsUsed []string) types.EvaluationAssertionResult {
	expected := strings.ToLower(assertion.Value)
	for _, skill := range skillsUsed {
		if strings.ToLower(skill) == expected {
			return types.EvaluationAssertionResult{
				AssertionType:  assertion.Type,
				AssertionValue: assertion.Value,
				Passed:         true,
			}
		}
	}
	return types.EvaluationAssertionResult{
		AssertionType:  assertion.Type,
		AssertionValue: assertion.Value,
		Passed:         false,
		Details:        fmt.Sprintf("skill %q was not used; skills used: %v", assertion.Value, skillsUsed),
	}
}
