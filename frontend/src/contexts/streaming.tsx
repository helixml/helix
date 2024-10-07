import React, { createContext, useContext, ReactNode } from 'react';
import { ISession, ISessionChatRequest, SESSION_MODE_INFERENCE, SESSION_TYPE_TEXT } from '../types';
import useApi from '../hooks/useApi';

interface StreamingContextType {
  NewSession: (message: string, appId: string) => Promise<ISession>;
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
      return newSessionData;
    } catch (error) {
      console.error('Error creating new session:', error);
      throw error;
    }
  };

  const value = {
    NewSession,
  };

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  );
};