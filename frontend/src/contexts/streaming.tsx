import React, { createContext, useContext, ReactNode, useState, useCallback, useEffect } from 'react';
import ReconnectingWebSocket from 'reconnecting-websocket';
import { ISession, SESSION_MODE_INFERENCE, SESSION_TYPE_TEXT, IWebsocketEvent, WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE, WORKER_TASK_RESPONSE_TYPE_PROGRESS, WORKER_TASK_RESPONSE_TYPE_STREAM, IInteraction } from '../types';
import useApi from '../hooks/useApi';
import useAccount from '../hooks/useAccount';

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
  const account = useAccount();
  const [currentResponses, setCurrentResponses] = useState<Map<string, Partial<IInteraction>>>(new Map());
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);

  const handleWebsocketEvent = useCallback((parsedData: IWebsocketEvent) => {
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

  useEffect(() => {
    if (!account.token || !currentSessionId) return;

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsHost = window.location.host;
    const url = `${wsProtocol}//${wsHost}/api/v1/ws/user?access_token=${account.tokenUrlEscaped}&session_id=${currentSessionId}`;
    const rws = new ReconnectingWebSocket(url);

    const messageHandler = (event: MessageEvent<any>) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent;
      if (parsedData.session_id !== currentSessionId) return;
      handleWebsocketEvent(parsedData);
    };

    rws.addEventListener('message', messageHandler);

    return () => {
      rws.removeEventListener('message', messageHandler);
      rws.close();
    };
  }, [account.token, currentSessionId, handleWebsocketEvent]);

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