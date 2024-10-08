import React, { createContext, useContext, ReactNode, useState, useCallback } from 'react';
import { ISession, ISessionChatRequest, SESSION_MODE_INFERENCE, SESSION_TYPE_TEXT, IWebsocketEvent, WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE, WORKER_TASK_RESPONSE_TYPE_PROGRESS, WORKER_TASK_RESPONSE_TYPE_STREAM, IInteraction } from '../types';
import useApi from '../hooks/useApi';
import useWebsocket from '../hooks/useWebsocket';

interface StreamingContextType {
  NewSession: (message: string, appId: string) => Promise<ISession>;
  currentResponses: Map<string, Partial<IInteraction>>;
  updateCurrentResponse: (sessionId: string, interaction: Partial<IInteraction>) => void;
}

const StreamingContext = createContext<StreamingContextType | undefined>(undefined);

export const useStreaming = (): StreamingContextType => {
  const context = useContext(StreamingContext);
  if (context === undefined) {
    throw new Error('useStreaming must be used within a StreamingProvider');
  }
  return context;
};

export const StreamingContextProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const api = useApi();
  const [currentResponses, setCurrentResponses] = useState<Map<string, Partial<IInteraction>>>(new Map());
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);

  const handleWebsocketEvent = useCallback((parsedData: IWebsocketEvent) => {
    console.log('WebSocket message received:', parsedData);
    if (!currentSessionId) return;

    if (parsedData.type === WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE && parsedData.worker_task_response) {
      const workerResponse = parsedData.worker_task_response;
      setCurrentResponses(prev => {
        const current = prev.get(currentSessionId) || {};
        let updatedInteraction: Partial<IInteraction> = { ...current };

        if (workerResponse.type === WORKER_TASK_RESPONSE_TYPE_STREAM && workerResponse.message) {
          updatedInteraction.message = (current.message || '') + workerResponse.message;
        } else if (workerResponse.type === WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
          if (workerResponse.message) {
            updatedInteraction.message = workerResponse.message;
          }
          if (workerResponse.progress !== undefined) {
            updatedInteraction.progress = workerResponse.progress;
          }
          if (workerResponse.status) {
            updatedInteraction.status = workerResponse.status;
          }
        }

        return new Map(prev).set(currentSessionId, updatedInteraction);
      });
    }
  }, [currentSessionId]);

  const NewSession = async (message: string, appId: string): Promise<ISession> => {
    console.log('NewSession', appId)
    const sessionChatRequest = {
      mode: SESSION_MODE_INFERENCE,
      type: SESSION_TYPE_TEXT,
      stream: true,
      legacy: true,
      app_id: appId,
      messages: [{
        role: 'user',
        content: {
          content_type: 'text',
          parts: [message]
        },
      }]
    };

    try {
      const newSessionData = await api.post('/api/v1/sessions/chat', sessionChatRequest);
      if (!newSessionData) {
        throw new Error('Failed to create new session');
      }
      setCurrentResponses(prev => new Map(prev).set(newSessionData.id, { message: '', status: '', progress: 0 }));
      setCurrentSessionId(newSessionData.id);

      // TODO: when we have multiple concurrent streaming sessions, rather than
      // multiple websocket connections we can just have one and filter for a
      // set of session_ids
      useWebsocket(newSessionData.id, handleWebsocketEvent);
      return newSessionData;
    } catch (error) {
      console.error('Error creating new session:', error);
      throw error;
    }
  };

  const updateCurrentResponse = (sessionId: string, interaction: Partial<IInteraction>) => {
    setCurrentResponses(prev => {
      const current = prev.get(sessionId) || {};
      return new Map(prev).set(sessionId, { ...current, ...interaction });
    });
  };

  const value = {
    NewSession,
    currentResponses,
    updateCurrentResponse,
  };

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  );
};