package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/oklog/ulid/v2"
)

const (
	ToolPrefix                 = "tool_"
	SessionPrefix              = "ses_"
	InteractionPrefix          = "int_"
	AppPrefix                  = "app_"
	RequestPrefix              = "req_"
	DataEntityPrefix           = "dent_"
	LLMCallPrefix              = "llmc_"
	KnowledgePrefix            = "kno_"
	KnowledgeVersionPrefix     = "knov_"
	SecretPrefix               = "sec_"
	TestRunPrefix              = "testrun_"
	OpenAIResponsePrefix       = "oai_"
	ProviderEndpointPrefix     = "pe_"
	OrganizationPrefix         = "org_"
	TeamPrefix                 = "team_"
	UserPrefix                 = "usr_"
	RolePrefix                 = "role_"
	AccessGrantPrefix          = "ag_"
	UsageMetricPrefix          = "um_"
	StepInfoPrefix             = "step_"
	CallPrefix                 = "call_"
	TriggerConfigurationPrefix = "trgc_"
	TriggerExecutionPrefix     = "trex_"
	WalletPrefix               = "wal_"
	TransactionPrefix          = "txn_"
	TopUpPrefix                = "top_"
	MemoryPrefix               = "mem_"
	QuestionSetPrefix          = "qs_"
	QuestionSetExecutionPrefix = "qsex_"
	SpecTaskPrefix             = "spt_"
	ProjectPrefix              = "prj_"
	CloneGroupPrefix           = "clg_"
	UserSessionPrefix          = "uss_"
	ClaudeSubscriptionPrefix   = "csub_"
	AttentionEventPrefix       = "atev_"
	EvaluationSuitePrefix      = "evs_"
	EvaluationRunPrefix        = "evr_"
	SandboxPrefix              = "sbx_"
	SandboxCommandPrefix       = "sbcmd_"
	RunnerProfilePrefix        = "rprof_"
	SpecTaskAttachmentPrefix   = "att_"
	OrgInvitationPrefix        = "oin_"
)

func GenerateUUID() string {
	return uuid.New().String()
}

func GenerateID() string {
	return newID()
}

func newID() string {
	return strings.ToLower(ulid.Make().String())
}

func GenerateStepInfoID() string {
	return fmt.Sprintf("%s%s", StepInfoPrefix, newID())
}

func GenerateToolID() string {
	return fmt.Sprintf("%s%s", ToolPrefix, newID())
}

func GenerateSessionID() string {
	return fmt.Sprintf("%s%s", SessionPrefix, newID())
}

func GenerateInteractionID() string {
	return fmt.Sprintf("%s%s", InteractionPrefix, newID())
}

func GenerateAppID() string {
	return fmt.Sprintf("%s%s", AppPrefix, newID())
}

func GenerateDataEntityID() string {
	return fmt.Sprintf("%s%s", DataEntityPrefix, newID())
}

func GenerateRequestID() string {
	return fmt.Sprintf("%s%s", RequestPrefix, newID())
}

func GenerateLLMCallID() string {
	return fmt.Sprintf("%s%s", LLMCallPrefix, newID())
}

func GenerateKnowledgeID() string {
	return fmt.Sprintf("%s%s", KnowledgePrefix, newID())
}

func GenerateKnowledgeVersionID() string {
	return fmt.Sprintf("%s%s", KnowledgeVersionPrefix, newID())
}

func GenerateSecretID() string {
	return fmt.Sprintf("%s%s", SecretPrefix, newID())
}

func GenerateOrganizationID() string {
	return fmt.Sprintf("%s%s", OrganizationPrefix, newID())
}

func GenerateTeamID() string {
	return fmt.Sprintf("%s%s", TeamPrefix, newID())
}

func GenerateUserID() string {
	return fmt.Sprintf("%s%s", UserPrefix, newID())
}

func GenerateRoleID() string {
	return fmt.Sprintf("%s%s", RolePrefix, newID())
}

func GenerateAccessGrantID() string {
	return fmt.Sprintf("%s%s", AccessGrantPrefix, newID())
}

func GenerateProjectID() string {
	return fmt.Sprintf("%s%s", ProjectPrefix, newID())
}

// GenerateVersion generates a version string for the knowledge
// This is used to identify the version of the knowledge
// and to determine if the knowledge has been updated
func GenerateVersion() string {
	return time.Now().Format("2006-01-02_15-04-05")
}

func GenerateTestRunID() string {
	return fmt.Sprintf("%s%s", TestRunPrefix, newID())
}

func GenerateOpenAIResponseID() string {
	return fmt.Sprintf("%s%s", OpenAIResponsePrefix, newID())
}

func GenerateProviderEndpointID() string {
	return fmt.Sprintf("%s%s", ProviderEndpointPrefix, newID())
}

func GenerateUsageMetricID() string {
	return fmt.Sprintf("%s%s", UsageMetricPrefix, newID())
}

func GenerateCallID() string {
	return fmt.Sprintf("%s%s", CallPrefix, newID())
}

func GenerateTriggerConfigurationID() string {
	return fmt.Sprintf("%s%s", TriggerConfigurationPrefix, newID())
}

func GenerateTriggerExecutionID() string {
	return fmt.Sprintf("%s%s", TriggerExecutionPrefix, newID())
}

func GenerateWalletID() string {
	return fmt.Sprintf("%s%s", WalletPrefix, newID())
}

func GenerateTransactionID() string {
	return fmt.Sprintf("%s%s", TransactionPrefix, newID())
}

func GenerateTopUpID() string {
	return fmt.Sprintf("%s%s", TopUpPrefix, newID())
}

func GenerateMemoryID() string {
	return fmt.Sprintf("%s%s", MemoryPrefix, newID())
}

func GenerateQuestionSetID() string {
	return fmt.Sprintf("%s%s", QuestionSetPrefix, newID())
}

func GenerateQuestionSetExecutionID() string {
	return fmt.Sprintf("%s%s", QuestionSetExecutionPrefix, newID())
}

func GenerateSpecTaskID() string {
	return fmt.Sprintf("%s%s", SpecTaskPrefix, newID())
}

func GenerateCloneGroupID() string {
	return fmt.Sprintf("%s%s", CloneGroupPrefix, newID())
}

func GenerateUserSessionID() string {
	return fmt.Sprintf("%s%s", UserSessionPrefix, newID())
}

func GenerateClaudeSubscriptionID() string {
	return fmt.Sprintf("%s%s", ClaudeSubscriptionPrefix, newID())
}

func GenerateAttentionEventID() string {
	return fmt.Sprintf("%s%s", AttentionEventPrefix, newID())
}

func GenerateEvaluationSuiteID() string {
	return fmt.Sprintf("%s%s", EvaluationSuitePrefix, newID())
}

func GenerateEvaluationRunID() string {
	return fmt.Sprintf("%s%s", EvaluationRunPrefix, newID())
}

func GenerateSandboxID() string {
	return fmt.Sprintf("%s%s", SandboxPrefix, newID())
}

func GenerateSandboxCommandID() string {
	return fmt.Sprintf("%s%s", SandboxCommandPrefix, newID())
}

func GenerateRunnerProfileID() string {
	return fmt.Sprintf("%s%s", RunnerProfilePrefix, newID())
}

func GenerateSpecTaskAttachmentID() string {
	return fmt.Sprintf("%s%s", SpecTaskAttachmentPrefix, newID())
}

func GenerateOrgInvitationID() string {
	return fmt.Sprintf("%s%s", OrgInvitationPrefix, newID())
}

// GenerateGitRepositoryID mints a unique id for a git repository row.
//
// Format: `<repoType>-<sanitizedName>-<ulid>`, e.g.
// `code-w-mt-01jx3vqz2j4n8m9p0r5t6w7x8y`.
//
// The leading `<repoType>-<sanitizedName>-` segment is preserved (rather
// than the short prefix-and-ulid shape used by other entities) for log
// greppability — operators searching for a worker's repo across services
// rely on `code-w-mt-` matching.
//
// The trailing ULID closes the cross-tenant collision class: two callers
// in different orgs minting a repo for identically-named entities (e.g.
// per-Worker repos for `w-mt` hired into two orgs in the same second)
// previously collided on the global `git_repositories_pkey` constraint
// with SQLSTATE 23505 because the suffix was `time.Now().Unix()`. ULIDs
// give 80 random bits + monotonic-within-millisecond ordering, removing
// the second-granularity collision window without a schema change.
func GenerateGitRepositoryID(repoType types.GitRepositoryType, name string) string {
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	sanitizedName = strings.ReplaceAll(sanitizedName, "_", "-")
	return fmt.Sprintf("%s-%s-%s", repoType, sanitizedName, newID())
}
