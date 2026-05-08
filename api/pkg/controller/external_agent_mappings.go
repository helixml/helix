package controller

import "sync"

type ExternalAgentRequestContextMappings struct {
	ContextMappingsMutex        *sync.RWMutex
	RequestToSessionMapping     *map[string]string
	RequestToInteractionMapping *map[string]string
}

// SetRequestInteractionMapping maps requestID → interactionID so that
// handleMessageAdded/handleMessageCompleted can route responses to the
// correct interaction.
func (m *ExternalAgentRequestContextMappings) SetRequestInteractionMapping(requestID, interactionID string) {
	m.ContextMappingsMutex.Lock()
	if *m.RequestToInteractionMapping == nil {
		*m.RequestToInteractionMapping = make(map[string]string)
	}
	(*m.RequestToInteractionMapping)[requestID] = interactionID
	m.ContextMappingsMutex.Unlock()
}

func (m *ExternalAgentRequestContextMappings) SetRequestSessionMapping(requestID, sessionID string) {
	m.ContextMappingsMutex.Lock()
	if *m.RequestToSessionMapping == nil {
		*m.RequestToSessionMapping = make(map[string]string)
	}
	(*m.RequestToSessionMapping)[requestID] = sessionID
	m.ContextMappingsMutex.Unlock()
}
