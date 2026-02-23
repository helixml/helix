package controller

import "sync"

type ExternalAgentRequestContextMappings struct {
	ContextMappingsMutex        *sync.RWMutex
	SessionToWaitingInteraction *map[string]string
	RequestToSessionMapping     *map[string]string
}

func (m *ExternalAgentRequestContextMappings) SetWaitingInteraction(sessionID, interactionID string) {
	m.ContextMappingsMutex.Lock()
	if *m.SessionToWaitingInteraction == nil {
		*m.SessionToWaitingInteraction = make(map[string]string)
	}
	(*m.SessionToWaitingInteraction)[sessionID] = interactionID
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
