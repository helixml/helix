package types

import (
	"time"

	"gorm.io/datatypes"
)

// SpecDrivenTask represents a task created using spec-driven development methodology
// Following Kiro.dev's approach: prompt -> requirements -> design -> implementation tasks
type SpecDrivenTask struct {
	ID          string `json:"id" gorm:"primaryKey"`
	ProjectID   string `json:"project_id" gorm:"index"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`     // "feature", "bug", "refactor", "epic"
	Priority    string `json:"priority"` // "low", "medium", "high", "critical"
	Status      string `json:"status"`   // "backlog", "ready", "in_progress", "review", "done"

	// Spec-driven development artifacts
	OriginalPrompt     string                  `json:"original_prompt"`     // The initial user request
	Requirements       *TaskRequirements       `json:"requirements"`        // Generated requirements spec
	TechnicalDesign    *TaskTechnicalDesign    `json:"technical_design"`    // Generated design document
	ImplementationPlan *TaskImplementationPlan `json:"implementation_plan"` // Breakdown of implementation tasks
	AcceptanceCriteria []AcceptanceCriterion   `json:"acceptance_criteria"` // EARS-format acceptance criteria

	// Agent assignment and tracking
	AssignedAgent string `json:"assigned_agent,omitempty"` // "zed", "cursor", etc.
	SessionID     string `json:"session_id,omitempty"`     // Active agent session
	BranchName    string `json:"branch_name,omitempty"`    // Git branch for this task

	// Estimation and tracking
	EstimatedHours int        `json:"estimated_hours,omitempty"`
	ActualHours    int        `json:"actual_hours,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`

	// Metadata
	CreatedBy string         `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DueDate   *time.Time     `json:"due_date,omitempty"`
	Labels    []string       `json:"labels" gorm:"-"`
	LabelsDB  datatypes.JSON `json:"-" gorm:"column:labels"`
	Metadata  datatypes.JSON `json:"metadata,omitempty"`
}

// TaskRequirements represents the structured requirements generated from the original prompt
// This replaces vague descriptions with clear, testable requirements
type TaskRequirements struct {
	UserStories   []UserStory      `json:"user_stories"`   // Who, what, why format
	Stakeholders  []string         `json:"stakeholders"`   // Who is affected by this feature
	Assumptions   []string         `json:"assumptions"`    // Explicit assumptions being made
	Constraints   []string         `json:"constraints"`    // Technical or business constraints
	Dependencies  []TaskDependency `json:"dependencies"`   // What this task depends on
	OutOfScope    []string         `json:"out_of_scope"`   // Explicitly what won't be done
	BusinessRules []BusinessRule   `json:"business_rules"` // Business logic requirements
	NonFunctional []NFRequirement  `json:"non_functional"` // Performance, security, etc.
}

// UserStory follows the standard "As a <user>, I want <goal> so that <benefit>" format
type UserStory struct {
	ID          string `json:"id"`
	Role        string `json:"role"`     // As a <role>
	Goal        string `json:"goal"`     // I want <goal>
	Benefit     string `json:"benefit"`  // So that <benefit>
	Priority    string `json:"priority"` // MoSCoW: Must/Should/Could/Won't
	StoryPoints int    `json:"story_points,omitempty"`
}

// AcceptanceCriterion uses EARS (Easy Approach to Requirements Syntax) format
// "When <trigger>, the system shall <response> where <constraint>"
type AcceptanceCriterion struct {
	ID          string `json:"id"`
	UserStoryID string `json:"user_story_id"`
	Given       string `json:"given"`       // Precondition
	When        string `json:"when"`        // Trigger/event
	Then        string `json:"then"`        // Expected behavior
	Where       string `json:"where"`       // Constraints/conditions
	Rationale   string `json:"rationale"`   // Why this criterion exists
	TestMethod  string `json:"test_method"` // How to verify this criterion
}

// TaskTechnicalDesign represents the technical design generated from requirements
// This ensures agents understand the system architecture before coding
type TaskTechnicalDesign struct {
	Overview        string                `json:"overview"`         // High-level design approach
	Architecture    ArchitectureDesign    `json:"architecture"`     // System architecture decisions
	DataModel       DataModelDesign       `json:"data_model"`       // Database schema changes
	APIDesign       APIDesign             `json:"api_design"`       // REST/GraphQL endpoints
	UIDesign        UIDesign              `json:"ui_design"`        // Frontend component structure
	SecurityDesign  SecurityDesign        `json:"security_design"`  // Security considerations
	TestingStrategy TestingStrategy       `json:"testing_strategy"` // How to test this feature
	Alternatives    []AlternativeApproach `json:"alternatives"`     // Other approaches considered
}

// ArchitectureDesign captures high-level system design decisions
type ArchitectureDesign struct {
	Components   []SystemComponent      `json:"components"`   // New/modified components
	DataFlow     []DataFlowStep         `json:"data_flow"`    // How data moves through system
	Interactions []ComponentInteraction `json:"interactions"` // How components communicate
	Patterns     []string               `json:"patterns"`     // Design patterns being used
	Technologies []string               `json:"technologies"` // Tech stack decisions
	Performance  PerformanceProfile     `json:"performance"`  // Expected performance characteristics
}

// DataModelDesign captures database and data structure changes
type DataModelDesign struct {
	Tables        []TableSchema       `json:"tables"`        // New/modified database tables
	Relationships []TableRelationship `json:"relationships"` // Foreign key relationships
	Indexes       []IndexDefinition   `json:"indexes"`       // Database indexes needed
	Migrations    []MigrationStep     `json:"migrations"`    // Step-by-step migration plan
	Constraints   []DataConstraint    `json:"constraints"`   // Data validation rules
}

// APIDesign captures REST/GraphQL API changes
type APIDesign struct {
	Endpoints      []APIEndpoint     `json:"endpoints"`       // New/modified API endpoints
	RequestModels  []RequestModel    `json:"request_models"`  // Request payload structures
	ResponseModels []ResponseModel   `json:"response_models"` // Response payload structures
	ErrorHandling  []ErrorScenario   `json:"error_handling"`  // Error cases and responses
	Authentication []AuthRequirement `json:"authentication"`  // Auth requirements
	RateLimiting   []RateLimitRule   `json:"rate_limiting"`   // Rate limiting rules
}

// UIDesign captures frontend component design
type UIDesign struct {
	Components      []UIComponent     `json:"components"`       // React/Vue components needed
	StateManagement StateManagement   `json:"state_management"` // How state is managed
	UserFlows       []UserFlow        `json:"user_flows"`       // User interaction flows
	Responsive      ResponsiveDesign  `json:"responsive"`       // Mobile/tablet considerations
	Accessibility   []A11yRequirement `json:"accessibility"`    // Accessibility requirements
}

// TaskImplementationPlan breaks down the design into discrete, executable tasks
// This replaces vague "implement feature X" with specific, measurable tasks
type TaskImplementationPlan struct {
	Tasks       []ImplementationTask `json:"tasks"`        // Ordered list of implementation tasks
	Milestones  []ProjectMilestone   `json:"milestones"`   // Key delivery milestones
	Timeline    Timeline             `json:"timeline"`     // Estimated timeline
	RiskFactors []RiskFactor         `json:"risk_factors"` // Potential risks and mitigations
}

// ImplementationTask represents a single, discrete piece of work
type ImplementationTask struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Description       string             `json:"description"`
	Type              string             `json:"type"` // "backend", "frontend", "database", "test"
	EstimatedHours    int                `json:"estimated_hours"`
	Dependencies      []string           `json:"dependencies"`       // Other task IDs this depends on
	RequirementIDs    []string           `json:"requirement_ids"`    // Requirements this task implements
	VerificationSteps []VerificationStep `json:"verification_steps"` // How to verify completion
	AgentInstructions string             `json:"agent_instructions"` // Specific instructions for AI agent
	Status            string             `json:"status"`             // "pending", "in_progress", "completed"
	StartedAt         *time.Time         `json:"started_at,omitempty"`
	CompletedAt       *time.Time         `json:"completed_at,omitempty"`
	AgentNotes        string             `json:"agent_notes,omitempty"` // Notes from the AI agent
}

// Supporting types for the design structures
type SystemComponent struct {
	Name             string   `json:"name"`
	Purpose          string   `json:"purpose"`
	Responsibilities []string `json:"responsibilities"`
	Interfaces       []string `json:"interfaces"`
}

type DataFlowStep struct {
	Step        int    `json:"step"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Data        string `json:"data"`
	Transform   string `json:"transform,omitempty"`
}

type TableSchema struct {
	Name    string      `json:"name"`
	Columns []ColumnDef `json:"columns"`
	Purpose string      `json:"purpose"`
}

type ColumnDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Nullable    bool   `json:"nullable"`
	Default     string `json:"default,omitempty"`
	Constraints string `json:"constraints,omitempty"`
}

type APIEndpoint struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Purpose     string            `json:"purpose"`
	Parameters  map[string]string `json:"parameters"`
	RequestBody string            `json:"request_body,omitempty"`
	Responses   map[string]string `json:"responses"`
}

type UserFlow struct {
	Name  string     `json:"name"`
	Steps []FlowStep `json:"steps"`
}

type FlowStep struct {
	Step        int    `json:"step"`
	Action      string `json:"action"`
	Expected    string `json:"expected"`
	Alternative string `json:"alternative,omitempty"`
}

type VerificationStep struct {
	Step     string `json:"step"`
	Method   string `json:"method"`          // "test", "manual", "automated"
	Criteria string `json:"criteria"`        // What constitutes success
	Tools    string `json:"tools,omitempty"` // Tools needed for verification
}

// TaskDependency represents what this task depends on
type TaskDependency struct {
	Type        string `json:"type"`        // "task", "service", "data"
	Name        string `json:"name"`        // What we depend on
	Description string `json:"description"` // Why we depend on it
	Critical    bool   `json:"critical"`    // Is this a blocking dependency?
}

// BusinessRule captures business logic requirements
type BusinessRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Source      string `json:"source"` // Who requested this rule
}

// NFRequirement captures non-functional requirements
type NFRequirement struct {
	Type        string `json:"type"`        // "performance", "security", "usability"
	Requirement string `json:"requirement"` // Specific requirement
	Metric      string `json:"metric"`      // How to measure
	Target      string `json:"target"`      // Target value
}

// Timeline captures project timeline estimates
type Timeline struct {
	TotalEstimatedHours int                 `json:"total_estimated_hours"`
	CriticalPath        []string            `json:"critical_path"` // Task IDs on critical path
	Milestones          []TimelineMilestone `json:"milestones"`
	BufferTime          int                 `json:"buffer_time_hours"` // Extra time for unknowns
}

type TimelineMilestone struct {
	Name        string    `json:"name"`
	Date        time.Time `json:"date"`
	TaskIDs     []string  `json:"task_ids"`
	Description string    `json:"description"`
}

// RiskFactor captures potential risks and mitigations
type RiskFactor struct {
	Risk        string `json:"risk"`
	Probability string `json:"probability"` // "low", "medium", "high"
	Impact      string `json:"impact"`      // "low", "medium", "high"
	Mitigation  string `json:"mitigation"`
}

// Additional supporting types
type ComponentInteraction struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Method  string `json:"method"`
	Purpose string `json:"purpose"`
}

type PerformanceProfile struct {
	ResponseTime string `json:"response_time"`
	Throughput   string `json:"throughput"`
	Memory       string `json:"memory"`
	CPU          string `json:"cpu"`
}

type TableRelationship struct {
	FromTable  string `json:"from_table"`
	ToTable    string `json:"to_table"`
	Type       string `json:"type"` // "one-to-one", "one-to-many", "many-to-many"
	ForeignKey string `json:"foreign_key"`
}

type IndexDefinition struct {
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
	Type    string   `json:"type"` // "btree", "hash", "partial"
	Purpose string   `json:"purpose"`
}

type MigrationStep struct {
	Order       int    `json:"order"`
	Description string `json:"description"`
	SQL         string `json:"sql,omitempty"`
	Rollback    string `json:"rollback,omitempty"`
}

type DataConstraint struct {
	Type        string `json:"type"` // "check", "unique", "foreign_key"
	Description string `json:"description"`
	SQL         string `json:"sql"`
}

type RequestModel struct {
	Name       string                 `json:"name"`
	Properties map[string]PropertyDef `json:"properties"`
	Required   []string               `json:"required"`
}

type ResponseModel struct {
	Name       string                 `json:"name"`
	Properties map[string]PropertyDef `json:"properties"`
	Examples   []string               `json:"examples"`
}

type PropertyDef struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Format      string `json:"format,omitempty"`
	Validation  string `json:"validation,omitempty"`
}

type ErrorScenario struct {
	Code        int    `json:"code"`
	Scenario    string `json:"scenario"`
	Response    string `json:"response"`
	UserMessage string `json:"user_message"`
}

type AuthRequirement struct {
	Endpoint string   `json:"endpoint"`
	Method   string   `json:"method"`
	Roles    []string `json:"roles"`
	Scopes   []string `json:"scopes"`
}

type RateLimitRule struct {
	Endpoint string `json:"endpoint"`
	Limit    int    `json:"limit"`
	Window   string `json:"window"`
	Scope    string `json:"scope"` // "user", "ip", "global"
}

type UIComponent struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"` // "page", "component", "hook"
	Purpose      string                 `json:"purpose"`
	Props        map[string]PropDef     `json:"props"`
	State        []StateDef             `json:"state"`
	Children     []string               `json:"children"`
	Interactions []ComponentInteraction `json:"interactions"`
}

type PropDef struct {
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

type StateDef struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Purpose      string `json:"purpose"`
	InitialValue string `json:"initial_value,omitempty"`
}

type StateManagement struct {
	Pattern string        `json:"pattern"` // "redux", "context", "zustand"
	Stores  []StateStore  `json:"stores"`
	Actions []StateAction `json:"actions"`
}

type StateStore struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Schema  string `json:"schema"`
}

type StateAction struct {
	Name        string `json:"name"`
	Purpose     string `json:"purpose"`
	Parameters  string `json:"parameters"`
	SideEffects string `json:"side_effects"`
}

type ResponsiveDesign struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
	Approach    string       `json:"approach"` // "mobile-first", "desktop-first"
}

type Breakpoint struct {
	Name   string `json:"name"`
	Width  string `json:"width"`
	Layout string `json:"layout"`
}

type A11yRequirement struct {
	Type        string `json:"type"` // "keyboard", "screen-reader", "contrast"
	Requirement string `json:"requirement"`
	Testing     string `json:"testing"`
}

type ProjectMilestone struct {
	Name         string    `json:"name"`
	Date         time.Time `json:"date"`
	Deliverables []string  `json:"deliverables"`
	Criteria     []string  `json:"criteria"`
}

type SecurityDesign struct {
	Threats    []ThreatModel     `json:"threats"`
	Controls   []SecurityControl `json:"controls"`
	Compliance []ComplianceReq   `json:"compliance"`
}

type ThreatModel struct {
	Threat     string `json:"threat"`
	Asset      string `json:"asset"`
	Impact     string `json:"impact"`
	Likelihood string `json:"likelihood"`
	Mitigation string `json:"mitigation"`
}

type SecurityControl struct {
	Type           string `json:"type"`
	Description    string `json:"description"`
	Implementation string `json:"implementation"`
}

type ComplianceReq struct {
	Standard    string `json:"standard"`
	Requirement string `json:"requirement"`
	Evidence    string `json:"evidence"`
}

type TestingStrategy struct {
	Approach    string     `json:"approach"`
	UnitTests   []TestPlan `json:"unit_tests"`
	Integration []TestPlan `json:"integration_tests"`
	E2E         []TestPlan `json:"e2e_tests"`
	Performance []PerfTest `json:"performance_tests"`
}

type TestPlan struct {
	Name       string   `json:"name"`
	Purpose    string   `json:"purpose"`
	Setup      string   `json:"setup"`
	Steps      []string `json:"steps"`
	Assertions []string `json:"assertions"`
	Cleanup    string   `json:"cleanup"`
}

type PerfTest struct {
	Name        string `json:"name"`
	Scenario    string `json:"scenario"`
	LoadPattern string `json:"load_pattern"`
	Criteria    string `json:"criteria"`
	Tools       string `json:"tools"`
}

type AlternativeApproach struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Pros        []string `json:"pros"`
	Cons        []string `json:"cons"`
	WhyRejected string   `json:"why_rejected"`
}
