package data

import (
	"fmt"
	"path"
	"time"

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

// get the most recent user interaction
func GetUserInteraction(session *types.Session) (*types.Interaction, error) {
	for i := len(session.Interactions) - 1; i >= 0; i-- {
		interaction := session.Interactions[i]
		if interaction.Creator == types.CreatorTypeUser {
			return &interaction, nil
		}
	}
	return nil, fmt.Errorf("no user interaction found")
}

func GetSystemInteraction(session *types.Session) (*types.Interaction, error) {
	for i := len(session.Interactions) - 1; i >= 0; i-- {
		interaction := session.Interactions[i]
		if interaction.Creator == types.CreatorTypeSystem {
			return &interaction, nil
		}
	}
	return nil, fmt.Errorf("no system interaction found")
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

// update the most recent system interaction

type InteractionUpdater func(*types.Interaction) (*types.Interaction, error)

func UpdateSystemInteraction(session *types.Session, updater InteractionUpdater) (*types.Session, error) {
	targetInteraction, err := GetSystemInteraction(session)
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
