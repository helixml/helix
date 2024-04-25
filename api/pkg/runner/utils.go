package runner

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

func getLastInteractionID(session *types.Session) (string, error) {
	if len(session.Interactions) == 0 {
		return "", fmt.Errorf("session has no messages")
	}
	interaction := session.Interactions[len(session.Interactions)-1]
	if interaction.Creator != types.CreatorTypeSystem {
		return "", fmt.Errorf("session does not have a system interaction as last message")
	}
	return interaction.ID, nil
}

func modelInstanceMatchesSession(modelInstance ModelInstance, session *types.Session) bool {
	return modelInstance.Filter().Mode == session.Mode &&
		modelInstance.Filter().Type == session.Type &&
		(modelInstance.Filter().LoraDir == session.LoraDir ||
			(modelInstance.Filter().LoraDir == types.LORA_DIR_NONE && session.LoraDir == ""))
}
