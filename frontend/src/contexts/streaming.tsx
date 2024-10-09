import React, { createContext, useContext, ReactNode, useState, useCallback, useEffect } from 'react';
import ReconnectingWebSocket from 'reconnecting-websocket';
import { ISession, IWebsocketEvent, WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE, WORKER_TASK_RESPONSE_TYPE_PROGRESS, IInteraction, ISessionChatRequest, SESSION_TYPE_TEXT } from '../types';
import useApi from '../hooks/useApi';
import useAccount from '../hooks/useAccount';
import { createParser, type ParsedEvent, type ReconnectInterval } from 'eventsource-parser';

interface NewInferenceParams {
  message: string;
  appId?: string;
  assistantId?: string;
  ragSourceId?: string;
  modelName?: string;
  loraDir?: string;
  sessionId?: string;
}

interface StreamingContextType {
  NewInference: (params: NewInferenceParams) => Promise<ISession>;
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

        if (workerResponse.type === WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
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

  const NewInference = async ({
    message,
    appId = '',
    assistantId = '',
    ragSourceId = '',
    modelName = '',
    loraDir = '',
    sessionId = ''
  }: NewInferenceParams): Promise<ISession> => {
    console.log('NewInference', appId);
    const sessionChatRequest: ISessionChatRequest = {
      type: SESSION_TYPE_TEXT,
      stream: true,
      app_id: appId,
      assistant_id: assistantId,
      rag_source_id: ragSourceId,
      model: modelName,
      lora_dir: loraDir,
      session_id: sessionId,
      messages: [{
        role: 'user',
        content: {
          content_type: 'text',
          parts: [message]
        },
      }]
    };

    try {
      const response = await fetch('/api/v1/sessions/chat', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${account.token}`,
        },
        body: JSON.stringify(sessionChatRequest),
      });

      if (!response.ok) {
        throw new Error('Failed to create or update session');
      }

      if (!response.body) {
        throw new Error('Response body is null');
      }

      const reader = response.body.getReader();
      let sessionData: ISession | null = null;
      let promiseResolved = false;

      const processStream = new Promise<void>((resolveStream, rejectStream) => {
        const parser = createParser((event: ParsedEvent | ReconnectInterval) => {
          if (event.type === 'event') {
            if (!event.data || event.data === '') {
              return;
            }
            if (event.data === "[DONE]") {
              // Safely remove the completed stream from currentResponses
              if (sessionData && sessionData.id) {
                setCurrentResponses(prev => {
                  const newMap = new Map(prev);
                  newMap.delete(sessionData!.id);
                  return newMap;
                });
              }
              resolveStream();
              return;
            }
            console.log('event.data', event.data);
            try {
              const parsedData = JSON.parse(event.data);
              if (!sessionData) {
                // This is the first chunk of data
                sessionData = parsedData;
                if (sessionData && sessionData.id) {
                  setCurrentSessionId(sessionData.id);
                  const messageSegment = parsedData.choices[0]?.delta?.content;
                  setCurrentResponses(prev => new Map(prev).set(sessionData!.id, { message: messageSegment || '', status: '', progress: 0 }));
                  
                  if (!promiseResolved) {
                    promiseResolved = true;
                  }
                } else {
                  console.error('Invalid session data received:', sessionData);
                  rejectStream(new Error('Invalid session data'));
                }
              } else if (Array.isArray(parsedData?.choices) && parsedData.choices.length > 0) {
                const messageSegment = parsedData.choices[0]?.delta?.content;
                if (messageSegment) {
                  setCurrentResponses(prev => {
                    const current = prev.get(sessionData!.id) || {};
                    return new Map(prev).set(sessionData!.id, {
                      ...current,
                      message: (current.message || '') + messageSegment
                    });
                  });
                }
              }
            } catch (error) {
              console.error('Error parsing SSE data:', error);
              rejectStream(error);
            }
          }
        });

        const pump = async () => {
          try {
            while (true) {
              const { done, value } = await reader.read();
              if (done) {
                // Safely remove the completed stream from currentResponses
                if (sessionData && sessionData.id) {
                  setCurrentResponses(prev => {
                    const newMap = new Map(prev);
                    newMap.delete(sessionData!.id);
                    return newMap;
                  });
                }
                resolveStream();
                break;
              }
              parser.feed(new TextDecoder().decode(value));
            }
          } catch (error) {
            rejectStream(error);
          }
        };

        pump();
      });

      // Wait for the first chunk to resolve the outer promise
      await new Promise<void>((resolve) => {
        const checkResolved = () => {
          if (promiseResolved) {
            resolve();
          } else {
            setTimeout(checkResolved, 10);
          }
        };
        checkResolved();
      });

      // Continue processing the stream in the background
      processStream.catch((error) => {
        console.error('Error processing stream:', error);
      });

      if (!sessionData) {
        throw new Error('Failed to receive session data');
      }

      return sessionData;

    } catch (error) {
      console.error('Error in NewInference:', error);
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
    NewInference,
    currentResponses,
    updateCurrentResponse,
  };

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  );
};