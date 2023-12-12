package data

import (
	"fmt"
	"path"
	"time"

	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
)

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
			return &interaction, nil
		}
	}
	return nil, fmt.Errorf("interaction not found: %s", id)
}

func GetLastUserInteraction(interactions []types.Interaction) (*types.Interaction, error) {
	for i := len(interactions) - 1; i >= 0; i-- {
		interaction := interactions[i]
		if interaction.Creator == types.CreatorTypeUser {
			return &interaction, nil
		}
	}
	return nil, fmt.Errorf("no user interaction found")
}

// get the most recent user interaction
func GetUserInteraction(session *types.Session) (*types.Interaction, error) {
	return GetLastUserInteraction(session.Interactions)
}

func GetLastSystemInteraction(interactions []types.Interaction) (*types.Interaction, error) {
	for i := len(interactions) - 1; i >= 0; i-- {
		interaction := interactions[i]
		if interaction.Creator == types.CreatorTypeSystem {
			return &interaction, nil
		}
	}
	return nil, fmt.Errorf("no system interaction found")
}

func GetSystemInteraction(session *types.Session) (*types.Interaction, error) {
	return GetLastSystemInteraction(session.Interactions)
}

func FilterUserInteractions(interactions []types.Interaction) []types.Interaction {
	filtered := []types.Interaction{}
	for _, interaction := range interactions {
		if interaction.Creator == types.CreatorTypeUser {
			filtered = append(filtered, interaction)
		}
	}
	return filtered
}

func FilterSystemInteractions(interactions []types.Interaction) []types.Interaction {
	filtered := []types.Interaction{}
	for _, interaction := range interactions {
		if interaction.Creator == types.CreatorTypeSystem {
			filtered = append(filtered, interaction)
		}
	}
	return filtered
}

func FilterFinetuneInteractions(interactions []types.Interaction) []types.Interaction {
	filtered := []types.Interaction{}
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

func CopyInteractionsUntil(interactions []types.Interaction, id string) []types.Interaction {
	copied := []types.Interaction{}
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

	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == targetInteraction.ID {
			newInteractions = append(newInteractions, *updatedInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	session.Interactions = newInteractions
	session.Updated = time.Now()

	return session, nil
}

func UpdateUserInteraction(session *types.Session, updater InteractionUpdater) (*types.Session, error) {
	targetInteraction, err := GetUserInteraction(session)
	if err != nil {
		return nil, err
	}
	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s", session.ID)
	}

	return UpdateInteraction(session, targetInteraction.ID, updater)
}

func UpdateSystemInteraction(session *types.Session, updater InteractionUpdater) (*types.Session, error) {
	targetInteraction, err := GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}
	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s", session.ID)
	}

	return UpdateInteraction(session, targetInteraction.ID, updater)
}

func GetSessionSummary(session *types.Session) (*types.SessionSummary, error) {
	systemInteraction, err := GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}
	userInteraction, err := GetUserInteraction(session)
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
	}, nil
}

func CreateSession(req types.CreateSessionRequest) (types.Session, error) {
	systemInteraction := types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Creator:        types.CreatorTypeSystem,
		Mode:           req.SessionMode,
		Message:        "",
		Files:          []string{},
		State:          types.InteractionStateWaiting,
		Finished:       false,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
	}

	session := types.Session{
		ID:            req.SessionID,
		Name:          system.GenerateAmusingName(),
		ModelName:     req.ModelName,
		Type:          req.SessionType,
		Mode:          req.SessionMode,
		ParentSession: req.ParentSession,
		Owner:         req.Owner,
		OwnerType:     req.OwnerType,
		Created:       time.Now(),
		Updated:       time.Now(),
		Interactions: []types.Interaction{
			req.UserInteraction,
			systemInteraction,
		},
		Config: types.SessionConfig{
			OriginalMode: req.SessionMode,
			Origin: types.SessionOrigin{
				Type: types.SessionOriginTypeUserCreated,
			},
		},
	}

	return session, nil
}

func CloneSession(oldSession types.Session, interactionID string) (*types.Session, error) {
	session := types.Session{
		ID:            system.GenerateUUID(),
		Name:          system.GenerateAmusingName(),
		ModelName:     oldSession.ModelName,
		Type:          oldSession.Type,
		Mode:          oldSession.Mode,
		ParentSession: oldSession.ParentSession,
		Owner:         oldSession.Owner,
		OwnerType:     oldSession.OwnerType,
		Created:       time.Now(),
		Updated:       time.Now(),
		Config:        oldSession.Config,
	}

	session.Interactions = CopyInteractionsUntil(oldSession.Interactions, interactionID)
	session.Config.Origin.Type = types.SessionOriginTypeCloned
	session.Config.Origin.ClonedSessionID = oldSession.ID
	session.Config.Origin.ClonedInteractionID = interactionID

	return &session, nil
}
