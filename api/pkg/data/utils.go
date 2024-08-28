package data

import (
	"fmt"
	"path"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/helixml/helix/api/pkg/system"
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

func GetInteractionFinetuneFile(session *types.Session, interactionID string) (string, error) {
	interaction, err := GetInteraction(session, interactionID)
	if err != nil {
		return "", err
	}
	if len(interaction.Files) == 0 {
		return "", fmt.Errorf("no files found")
	}
	foundFile := ""
	for _, filepath := range interaction.Files {
		if path.Base(filepath) == types.TEXT_DATA_PREP_QUESTIONS_FILE {
			foundFile = filepath
			break
		}
	}

	if foundFile == "" {
		return "", fmt.Errorf("file is not a jsonl file")
	}

	return foundFile, nil
}

func GetInteraction(session *types.Session, id string) (*types.Interaction, error) {
	if id == "" {
		return nil, fmt.Errorf("interaction id is required")
	}
	for _, interaction := range session.Interactions {
		if interaction.ID == id {
			return interaction, nil
		}
	}
	return nil, fmt.Errorf("interaction not found: %s", id)
}

func GetLastInteractions(session *types.Session, limit int) []*types.Interaction {
	interactions := session.Interactions
	if len(interactions) == 0 {
		return interactions
	}
	if len(interactions) < limit {
		limit = len(interactions)
	}
	return interactions[len(interactions)-limit:]
}

func GetLastUserInteraction(interactions []*types.Interaction) (*types.Interaction, error) {
	for i := len(interactions) - 1; i >= 0; i-- {
		interaction := interactions[i]
		if interaction.Creator == types.CreatorTypeUser {
			return interaction, nil
		}
	}
	return nil, fmt.Errorf("no user interaction found")
}

func GetFirstUserInteraction(interactions []*types.Interaction) (*types.Interaction, error) {
	for _, interaction := range interactions {
		if interaction.Creator == types.CreatorTypeUser {
			return interaction, nil
		}
	}
	return nil, fmt.Errorf("no user interaction found")
}

// get the most recent user interaction
func GetUserInteraction(interactions []*types.Interaction) (*types.Interaction, error) {
	return GetLastUserInteraction(interactions)
}

func GetLastAssistantInteraction(interactions []*types.Interaction) (*types.Interaction, error) {
	for i := len(interactions) - 1; i >= 0; i-- {
		interaction := interactions[i]
		if interaction.Creator == types.CreatorTypeAssistant {
			return interaction, nil
		}
	}
	return nil, fmt.Errorf("no system interaction found")
}

func GetAssistantInteraction(session *types.Session) (*types.Interaction, error) {
	return GetLastAssistantInteraction(session.Interactions)
}

func FilterUserInteractions(interactions []*types.Interaction) []*types.Interaction {
	filtered := []*types.Interaction{}
	for _, interaction := range interactions {
		if interaction.Creator == types.CreatorTypeUser {
			filtered = append(filtered, interaction)
		}
	}
	return filtered
}

func FilterFinetuneInteractions(interactions []*types.Interaction) []*types.Interaction {
	filtered := []*types.Interaction{}
	for _, interaction := range interactions {
		if interaction.Mode == types.SessionModeFinetune {
			filtered = append(filtered, interaction)
		}
	}
	return filtered
}

func FilterInferenceInteractions(interactions []types.Interaction) []types.Interaction {
	filtered := []types.Interaction{}
	for _, interaction := range interactions {
		if interaction.Mode == types.SessionModeInference {
			filtered = append(filtered, interaction)
		}
	}
	return filtered
}

func CopyInteractionsUntil(interactions []*types.Interaction, id string) []*types.Interaction {
	copied := []*types.Interaction{}
	for _, interaction := range interactions {
		copied = append(copied, interaction)
		if interaction.ID == id {
			break
		}
	}
	return copied
}

// update the most recent system interaction

type InteractionUpdater func(*types.Interaction) (*types.Interaction, error)

func UpdateInteraction(session *types.Session, id string, updater InteractionUpdater) (*types.Session, error) {
	targetInteraction, err := GetInteraction(session, id)
	if err != nil {
		return nil, err
	}
	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s", session.ID)
	}

	targetInteraction.Updated = time.Now()
	updatedInteraction, err := updater(targetInteraction)
	if err != nil {
		return nil, err
	}

	newInteractions := []*types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == targetInteraction.ID {
			newInteractions = append(newInteractions, updatedInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	session.Interactions = newInteractions
	session.Updated = time.Now()

	return session, nil
}

func UpdateUserInteraction(session *types.Session, updater InteractionUpdater) (*types.Session, error) {
	targetInteraction, err := GetUserInteraction(session.Interactions)
	if err != nil {
		return nil, err
	}
	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s", session.ID)
	}

	return UpdateInteraction(session, targetInteraction.ID, updater)
}

func UpdateSystemInteraction(session *types.Session, updater InteractionUpdater) (*types.Session, error) {
	targetInteraction, err := GetAssistantInteraction(session)
	if err != nil {
		return nil, err
	}
	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s", session.ID)
	}

	return UpdateInteraction(session, targetInteraction.ID, updater)
}

func GetSessionSummary(session *types.Session) (*types.SessionSummary, error) {
	systemInteraction, err := GetAssistantInteraction(session)
	if err != nil {
		return nil, err
	}
	userInteraction, err := GetUserInteraction(session.Interactions)
	if err != nil {
		return nil, err
	}
	summary := ""
	if session.Mode == types.SessionModeInference {
		summary = userInteraction.Message
	} else if session.Mode == types.SessionModeFinetune {
		summary = fmt.Sprintf("fine tuning on %d files", len(userInteraction.Files))
	} else {
		return nil, fmt.Errorf("invalid session mode")
	}
	return &types.SessionSummary{
		SessionID:     session.ID,
		Name:          session.Name,
		InteractionID: systemInteraction.ID,
		Mode:          session.Mode,
		Type:          session.Type,
		ModelName:     session.ModelName,
		Owner:         session.Owner,
		LoraDir:       session.LoraDir,
		Created:       systemInteraction.Created,
		Updated:       systemInteraction.Updated,
		Scheduled:     systemInteraction.Scheduled,
		Completed:     systemInteraction.Completed,
		Summary:       summary,
		Priority:      session.Metadata.Priority,
	}, nil
}

func CloneSession(
	oldSession types.Session,
	interactionID string,
	ctx types.OwnerContext,
) (*types.Session, error) {
	session := types.Session{
		ID:            system.GenerateUUID(),
		Name:          system.GenerateAmusingName(),
		ModelName:     oldSession.ModelName,
		Type:          oldSession.Type,
		Mode:          oldSession.Mode,
		ParentSession: oldSession.ParentSession,
		Owner:         ctx.Owner,
		OwnerType:     ctx.OwnerType,
		Created:       time.Now(),
		Updated:       time.Now(),
		Metadata:      oldSession.Metadata,
	}

	session.Interactions = CopyInteractionsUntil(oldSession.Interactions, interactionID)
	session.Metadata.Origin.Type = types.SessionOriginTypeCloned
	session.Metadata.Origin.ClonedSessionID = oldSession.ID
	session.Metadata.Origin.ClonedInteractionID = interactionID

	return &session, nil
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
