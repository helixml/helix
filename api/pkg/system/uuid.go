package system

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
)

const (
	ToolPrefix    = "tool_"
	SessionPrefix = "ses_"
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
