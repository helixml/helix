package data

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// Version is set by the build process
var Version string

func GetHelixVersion() string {
	if Version != "" {
		return Version
	}

	helixVersion := "<unknown>"
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, kv := range info.Settings {
			if kv.Value == "" {
				continue
			}
			switch kv.Key {
			case "vcs.revision":
				helixVersion = kv.Value
			}
		}
	}
	return helixVersion
}

func GetSessionSummary(session *types.Session) (*types.SessionSummary, error) {

	prompt := ""
	if len(session.Interactions) > 0 {
		prompt = session.Interactions[len(session.Interactions)-1].PromptMessage
	}

	return &types.SessionSummary{
		SessionID:              session.ID,
		Name:                   session.Name,
		Type:                   session.Type,
		ModelName:              session.ModelName,
		Owner:                  session.Owner,
		Created:                session.Created,
		Updated:                session.Updated,
		Summary:                prompt,
		Priority:               session.Metadata.Priority,
		AppID:                  session.ParentApp,
		OrganizationID:         session.OrganizationID,
		QuestionSetID:          session.QuestionSetID,
		QuestionSetExecutionID: session.QuestionSetExecutionID,
		Metadata:               session.Metadata,
	}, nil
}

func OwnerContextFromRequestContext(user *types.User) types.OwnerContext {
	return types.OwnerContext{
		Owner:     user.ID,
		OwnerType: user.Type,
	}
}

func OwnerContext(user string) types.OwnerContext {
	return types.OwnerContext{
		Owner:     user,
		OwnerType: types.OwnerTypeUser,
	}
}

func IsInteger(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func GetAssistant(app *types.App, assistantID string) *types.AssistantConfig {
	if assistantID == "" {
		assistantID = "0"
	}

	var assistant *types.AssistantConfig
	for _, a := range app.Config.Helix.Assistants {
		if a.ID == assistantID {
			assistant = &a
			break
		}
	}

	if IsInteger(assistantID) {
		assistantIndex, _ := strconv.Atoi(assistantID)
		if len(app.Config.Helix.Assistants) > assistantIndex {
			assistant = &app.Config.Helix.Assistants[assistantIndex]
		}
	}

	return assistant
}

func ContentHash(b []byte) string {
	hash := sha256.Sum256(b)
	hashString := hex.EncodeToString(hash[:])

	return hashString[:10]
}

// SlugifyName converts a name to a URL-friendly slug without spaces
// Similar to GitHub repository names: lowercase, alphanumeric, hyphens/underscores only
// Example: "Modern Todo App" -> "modern-todo-app"
func SlugifyName(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")

	// Replace other non-alphanumeric characters (except hyphens and underscores) with hyphens
	reg := regexp.MustCompile(`[^a-z0-9_-]+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Collapse multiple consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Ensure slug is not empty
	if slug == "" {
		slug = "unnamed"
	}

	return slug
}
