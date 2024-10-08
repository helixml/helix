import React, { createContext, useContext, ReactNode, useState, useEffect, useCallback, useRef } from 'react';
import { ISession, ISessionChatRequest, SESSION_MODE_INFERENCE, SESSION_TYPE_TEXT, IWebsocketEvent, WEBSOCKET_EVENT_TYPE_SESSION_UPDATE, IInteraction } from '../types';
import useApi from '../hooks/useApi';

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
  const [socket, setSocket] = useState<WebSocket | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  const connectWebSocket = useCallback((sessionId: string) => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/v1/ws?session_id=${sessionId}`;
    console.log('Attempting to connect WebSocket:', wsUrl);

    const newSocket = new WebSocket(wsUrl);
    socketRef.current = newSocket;

    // Start logging WebSocket state immediately
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }
    intervalRef.current = setInterval(() => {
      console.log('Current WebSocket state:', newSocket.readyState);
    }, 1000);

    newSocket.onopen = () => {
      console.log('WebSocket connected successfully');
    };

    newSocket.onmessage = (event) => {
      console.log('WebSocket message received:', event.data);
      try {
        const parsedData: IWebsocketEvent = JSON.parse(event.data);
        if (parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
          const lastInteraction = parsedData.session.interactions[parsedData.session.interactions.length - 1];
          if (lastInteraction && lastInteraction.creator === 'assistant') {
            setCurrentResponses(prev => {
              const current = prev.get(sessionId) || {};
              return new Map(prev).set(sessionId, {
                ...current,
                message: ((current.message || '') + (lastInteraction.message || '')),
                status: lastInteraction.status,
                progress: lastInteraction.progress,
              });
            });
          }
        }
      } catch (error) {
        console.error('Error parsing WebSocket message:', error);
      }
    };

    newSocket.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    newSocket.onclose = (event) => {
      console.log('WebSocket closed:', event);
      // Clear the interval when the WebSocket closes
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };

    setSocket(newSocket);
    return newSocket;
  }, []);

  // close websocket connection when component unmounts
  useEffect(() => {
    return () => {
      if (socketRef.current) {
        console.log('Closing WebSocket connection');
        socketRef.current.close();
      }
      // Clear the interval when the component unmounts
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, []);

  const NewSession = async (message: string, appId: string): Promise<ISession> => {
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
      connectWebSocket(newSessionData.id);
      
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