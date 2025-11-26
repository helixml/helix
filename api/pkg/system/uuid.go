package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
