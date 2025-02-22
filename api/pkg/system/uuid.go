package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

const (
	ToolPrefix                = "tool_"
	SessionPrefix             = "ses_"
	AppPrefix                 = "app_"
	GptScriptRunnerTaskPrefix = "gst_"
	RequestPrefix             = "req_"
	DataEntityPrefix          = "dent_"
	LLMCallPrefix             = "llmc_"
	KnowledgePrefix           = "kno_"
	KnowledgeVersionPrefix    = "knov_"
	SecretPrefix              = "sec_"
	TestRunPrefix             = "testrun_"
	OpenAIResponsePrefix      = "oai_"
	ProviderEndpointPrefix    = "pe_"
	OrganizationPrefix        = "org_"
	TeamPrefix                = "team_"
	UserPrefix                = "usr_"
	RolePrefix                = "role_"
	AccessGrantPrefix         = "ag_"
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

func GenerateToolID() string {
	return fmt.Sprintf("%s%s", ToolPrefix, newID())
}

func GenerateSessionID() string {
	return fmt.Sprintf("%s%s", SessionPrefix, newID())
}

func GenerateAppID() string {
	return fmt.Sprintf("%s%s", AppPrefix, newID())
}

func GenerateDataEntityID() string {
	return fmt.Sprintf("%s%s", DataEntityPrefix, newID())
}

func GenerateGptScriptTaskID() string {
	return fmt.Sprintf("%s%s", GptScriptRunnerTaskPrefix, newID())
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
