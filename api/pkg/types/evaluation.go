package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// EvaluationAssertionType defines how to check an agent's response
type EvaluationAssertionType string

const (
	EvaluationAssertionTypeContains    EvaluationAssertionType = "contains"
	EvaluationAssertionTypeNotContains EvaluationAssertionType = "not_contains"
	EvaluationAssertionTypeRegex       EvaluationAssertionType = "regex"
	EvaluationAssertionTypeLLMJudge    EvaluationAssertionType = "llm_judge"
	EvaluationAssertionTypeSkillUsed   EvaluationAssertionType = "skill_used"
)

// EvaluationAssertion defines a single check to apply to a question's response
type EvaluationAssertion struct {
	Type           EvaluationAssertionType `json:"type"`
	Value          string                  `json:"value"`                      // Expected string, regex pattern, or skill name
	LLMJudgePrompt string                 `json:"llm_judge_prompt,omitempty"` // Custom prompt for LLM judge mode
}

// EvaluationQuestion is a question with assertions to check the response against
type EvaluationQuestion struct {
	ID         string                `json:"id"`
	Question   string                `json:"question"`
	Assertions []EvaluationAssertion `json:"assertions"`
}

// EvaluationSuite represents a test suite for evaluating an agent
type EvaluationSuite struct {
	ID             string               `json:"id" gorm:"primaryKey"`
	Created        time.Time            `json:"created"`
	Updated        time.Time            `json:"updated"`
	UserID         string               `json:"user_id" gorm:"index"`
	OrganizationID string               `json:"organization_id" gorm:"index"`
	AppID          string               `json:"app_id" gorm:"index"`
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Questions      EvaluationQuestions   `json:"questions" gorm:"type:jsonb;serializer:json"`
}

// EvaluationQuestions is a slice type for GORM JSON serialization
type EvaluationQuestions []EvaluationQuestion

func (EvaluationQuestions) GormDataType() string { return "json" }

// EvaluationRunStatus tracks the lifecycle of an evaluation run
type EvaluationRunStatus string

const (
	EvaluationRunStatusPending   EvaluationRunStatus = "pending"
	EvaluationRunStatusRunning   EvaluationRunStatus = "running"
	EvaluationRunStatusCompleted EvaluationRunStatus = "completed"
	EvaluationRunStatusFailed    EvaluationRunStatus = "failed"
	EvaluationRunStatusCancelled EvaluationRunStatus = "cancelled"
)

// EvaluationAssertionResult is the outcome of a single assertion check
type EvaluationAssertionResult struct {
	AssertionType  EvaluationAssertionType `json:"assertion_type"`
	AssertionValue string                  `json:"assertion_value"`
	Passed         bool                    `json:"passed"`
	Details        string                  `json:"details,omitempty"` // e.g. LLM judge reasoning
}

// EvaluationQuestionResult captures the full result for a single question
type EvaluationQuestionResult struct {
	QuestionID       string                      `json:"question_id"`
	Question         string                      `json:"question"`
	Response         string                      `json:"response"`
	SessionID        string                      `json:"session_id"`
	InteractionID    string                      `json:"interaction_id"`
	DurationMs       int64                       `json:"duration_ms"`
	TokensUsed       Usage                       `json:"tokens_used"`
	Cost             float64                     `json:"cost"`
	SkillsUsed       []string                    `json:"skills_used"`
	AssertionResults []EvaluationAssertionResult `json:"assertion_results"`
	Passed           bool                        `json:"passed"`
	Error            string                      `json:"error,omitempty"`
}

// EvaluationRunSummary aggregates metrics across all questions in a run
type EvaluationRunSummary struct {
	TotalQuestions  int      `json:"total_questions"`
	Passed          int      `json:"passed"`
	Failed          int      `json:"failed"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	TotalTokens     int      `json:"total_tokens"`
	TotalCost       float64  `json:"total_cost"`
	SkillsUsed      []string `json:"skills_used"`
}

// EvaluationRun stores a single execution of an evaluation suite
type EvaluationRun struct {
	ID                string                     `json:"id" gorm:"primaryKey"`
	Created           time.Time                  `json:"created"`
	Updated           time.Time                  `json:"updated"`
	SuiteID           string                     `json:"suite_id" gorm:"index"`
	AppID             string                     `json:"app_id" gorm:"index"`
	UserID            string                     `json:"user_id" gorm:"index"`
	OrganizationID    string                     `json:"organization_id" gorm:"index"`
	Status            EvaluationRunStatus        `json:"status"`
	AppConfigSnapshot AppConfig                  `json:"app_config_snapshot" gorm:"type:jsonb;serializer:json"`
	Summary           EvaluationRunSummary       `json:"summary" gorm:"type:jsonb;serializer:json"`
	Results           EvaluationQuestionResults  `json:"results" gorm:"type:jsonb;serializer:json"`
	Error             string                     `json:"error,omitempty"`
}

// EvaluationQuestionResults is a slice type for GORM JSON serialization
type EvaluationQuestionResults []EvaluationQuestionResult

func (EvaluationQuestionResults) GormDataType() string { return "json" }

// EvaluationRunProgress is sent via SSE during a run
type EvaluationRunProgress struct {
	RunID            string                    `json:"run_id"`
	Status           EvaluationRunStatus       `json:"status"`
	CurrentQuestion  int                       `json:"current_question"`
	TotalQuestions   int                       `json:"total_questions"`
	LatestResult     *EvaluationQuestionResult `json:"latest_result,omitempty"`
	Summary          *EvaluationRunSummary     `json:"summary,omitempty"`
	Error            string                    `json:"error,omitempty"`
}

// ListEvaluationSuitesRequest defines filters for listing suites
type ListEvaluationSuitesRequest struct {
	UserID         string
	OrganizationID string
	AppID          string
}

// ListEvaluationRunsRequest defines filters for listing runs
type ListEvaluationRunsRequest struct {
	SuiteID        string
	AppID          string
	UserID         string
	OrganizationID string
	Offset         int
	Limit          int
}

// StartEvaluationRunRequest is the API request body to start a run
type StartEvaluationRunRequest struct {
	SuiteID string `json:"suite_id"`
}

// --- GORM Value/Scan for AppConfig used in EvaluationRun ---
// AppConfig already has Value/Scan/GormDataType in types.go

// --- GORM serialization helpers ---

func (s EvaluationRunSummary) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *EvaluationRunSummary) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed")
	}
	return json.Unmarshal(source, s)
}

func (EvaluationRunSummary) GormDataType() string { return "json" }
