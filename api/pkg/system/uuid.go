package system

import (
	"fmt"
	"strings"

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
)

func GenerateUUID() string {
	return uuid.New().String()
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
