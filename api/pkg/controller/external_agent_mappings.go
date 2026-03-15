package controller

import "sync"

type ExternalAgentRequestContextMappings struct {
	ContextMappingsMutex        *sync.RWMutex
	SessionToWaitingInteraction *map[string][]string
	RequestToSessionMapping     *map[string]string
}

// SetWaitingInteraction enqueues interactionID for the session.
// Using a FIFO queue (not overwrite) prevents the off-by-one bug where a concurrent
// sendMessageToSpecTaskAgent overwrites the mapping while another interaction is streaming,
// causing streaming updates to land in the wrong interaction.
func (m *ExternalAgentRequestContextMappings) SetWaitingInteraction(sessionID, interactionID string) {
	m.ContextMappingsMutex.Lock()
	if *m.SessionToWaitingInteraction == nil {
		*m.SessionToWaitingInteraction = make(map[string][]string)
	}
	(*m.SessionToWaitingInteraction)[sessionID] = append((*m.SessionToWaitingInteraction)[sessionID], interactionID)
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
