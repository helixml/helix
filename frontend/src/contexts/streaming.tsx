import React, { createContext, useContext, ReactNode, useState, useCallback, useEffect, useRef } from 'react';
import ReconnectingWebSocket from 'reconnecting-websocket';
import { ISession, IWebsocketEvent, WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE, WORKER_TASK_RESPONSE_TYPE_PROGRESS, IInteraction, ISessionChatRequest, SESSION_TYPE_TEXT, ISessionType } from '../types';
import useAccount from '../hooks/useAccount';
import useSessions from '../hooks/useSessions';

interface NewInferenceParams {
  type: ISessionType;
  message: string;
  appId?: string;
  assistantId?: string;
  ragSourceId?: string;
  modelName?: string;
  loraDir?: string;
  sessionId?: string;
  orgId?: string;
}

interface StreamingContextType {
  NewInference: (params: NewInferenceParams) => Promise<ISession>;
  setCurrentSessionId: (sessionId: string) => void;
  currentResponses: Map<string, Partial<IInteraction>>;
  stepInfos: Map<string, any[]>;
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
  const sessions = useSessions()
  const [currentResponses, setCurrentResponses] = useState<Map<string, Partial<IInteraction>>>(new Map());
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);
  const [stepInfos, setStepInfos] = useState<Map<string, any[]>>(new Map());

  // Add refs for managing streaming state
  const messageBufferRef = useRef<Map<string, string[]>>(new Map());
  const pendingUpdateRef = useRef<boolean>(false);
  const messageHistoryRef = useRef<Map<string, string>>(new Map());

  // Clear stepInfos when setting a new session
  const clearSessionData = useCallback((sessionId: string | null) => {
    // Don't clear anything if setting to the same session ID
    if (sessionId === currentSessionId) return;
    
    if (sessionId) {
      // Clear stepInfos for the new session
      setStepInfos(prev => {
        const newMap = new Map(prev);
        newMap.delete(sessionId);
        return newMap;
      });
    }
    
    setCurrentSessionId(sessionId);
  }, [currentSessionId]);

  // Function to flush message buffer to state
  const flushMessageBuffer = useCallback((sessionId: string) => {
    const chunks = messageBufferRef.current.get(sessionId);
    if (!chunks || chunks.length === 0) return;

    setCurrentResponses(prev => {
      const current = prev.get(sessionId) || {};
      const existingMessage = messageHistoryRef.current.get(sessionId) || '';
      const newChunks = chunks.join('');
      const newMessage = existingMessage + newChunks;
      
      // Update our history ref with the new complete message
      messageHistoryRef.current.set(sessionId, newMessage);
      
      // Clear just the buffer, keeping the history
      messageBufferRef.current.set(sessionId, []);
      
      return new Map(prev).set(sessionId, {
        ...current,
        message: newMessage
      });
    });
  }, []);

  // Schedule a flush if one isn't already pending
  const scheduleFlush = useCallback((sessionId: string) => {
    if (pendingUpdateRef.current) return;
    pendingUpdateRef.current = true;

    requestAnimationFrame(() => {
      flushMessageBuffer(sessionId);
      pendingUpdateRef.current = false;
    });
  }, [flushMessageBuffer]);

  // Function to add a message chunk to the buffer
  const addMessageChunk = useCallback((sessionId: string, chunk: string) => {
    const chunks = messageBufferRef.current.get(sessionId) || [];
    chunks.push(chunk);
    messageBufferRef.current.set(sessionId, chunks);
    scheduleFlush(sessionId);
  }, [scheduleFlush]);

  const handleWebsocketEvent = useCallback((parsedData: IWebsocketEvent) => {
    if (!currentSessionId) return;

    if (parsedData.type as string === "step_info") {
        const stepInfo = parsedData.step_info;
        setStepInfos(prev => {
            const currentSteps = prev.get(currentSessionId) || [];
            const updatedSteps = [...currentSteps, stepInfo];
            return new Map(prev).set(currentSessionId, updatedSteps);
        });
    }

    if (parsedData.type === WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE && parsedData.worker_task_response) {
      const workerResponse = parsedData.worker_task_response;

      // Use requestAnimationFrame to batch updates and prevent UI blocking
      requestAnimationFrame(() => {
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

          // Store the latest state in the ref
          const newMap = new Map(prev).set(currentSessionId, updatedInteraction);
          return newMap;
        });
      });
    }
    
    // If there's a session update with state changes
    if (parsedData.type === "session_update" && parsedData.session) {
      const lastInteraction = parsedData.session.interactions[parsedData.session.interactions.length - 1];
      
      // Update currentResponses with the latest interaction state
      // This ensures useLiveInteraction will receive the updated state
      if (lastInteraction.id) {
        requestAnimationFrame(() => {
          setCurrentResponses(prev => {
            const current = prev.get(currentSessionId) || {};
            const updatedInteraction: Partial<IInteraction> = {
              ...current,
              id: lastInteraction.id,
              state: lastInteraction.state,
              finished: lastInteraction.finished,
              // Copy any other important fields from the interaction
              message: lastInteraction.message || current.message
            };
            
            const newMap = new Map(prev).set(currentSessionId, updatedInteraction);
            return newMap;
          });
        });
      }
    }
  }, [currentSessionId]);

  useEffect(() => {
    if (!account.token || !currentSessionId) return;

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsHost = window.location.host;
    const url = `${wsProtocol}//${wsHost}/api/v1/ws/user?session_id=${currentSessionId}`;
    const rws = new ReconnectingWebSocket(url);

    const messageHandler = (event: MessageEvent<any>) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent;
      if (parsedData.session_id !== currentSessionId) return;
      
      // Reload all sessions to refresh the name in the sidebar
      sessions.loadSessions()
      handleWebsocketEvent(parsedData);
    };

    rws.addEventListener('message', messageHandler);

    return () => {
      rws.removeEventListener('message', messageHandler);
      rws.close();
    };
  }, [account.token, currentSessionId, handleWebsocketEvent]);

  const NewInference = async ({
    type,
    message,
    appId = '',
    assistantId = '',
    ragSourceId = '',
    modelName = '',
    loraDir = '',
    sessionId = '',
    orgId = '',
  }: NewInferenceParams): Promise<ISession> => {
    // Clear both buffer and history for new sessions
    messageBufferRef.current.delete(sessionId);
    messageHistoryRef.current.delete(sessionId);
    
    // Clear stepInfos for the session to reset the Glowing Orb list between interactions
    setStepInfos(prev => {
      const newMap = new Map(prev);
      if (sessionId) {
        newMap.delete(sessionId);
      }
      return newMap;
    });

    const sessionChatRequest: ISessionChatRequest = {
      type,
      stream: true,
      app_id: appId,
      organization_id: orgId,
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
      let decoder = new TextDecoder();
      let buffer = '';

      const processStream = new Promise<void>((resolveStream, rejectStream) => {
        const processChunk = (chunk: string) => {
          const lines = chunk.split('\n');
          
          if (buffer) {
            lines[0] = buffer + lines[0];
            buffer = '';
          }
          
          if (!chunk.endsWith('\n')) {
            buffer = lines.pop() || '';
          }

          for (const line of lines) {
            if (!line.trim()) continue;
            
            if (line.startsWith('data: ')) {
              const data = line.slice(5);
              
              if (data === '[DONE]') {
                if (sessionData?.id) {
                  // Final flush of any remaining content
                  flushMessageBuffer(sessionData.id);
                  setCurrentResponses(prev => {
                    const newMap = new Map(prev);
                    newMap.delete(sessionData!.id);
                    return newMap;
                  });
                }
                resolveStream();
                return;
              }

              try {
                const parsedData = JSON.parse(data);
                if (!sessionData) {
                  sessionData = parsedData;
                  if (sessionData?.id) {
                    // Set the current session ID first
                    clearSessionData(sessionData.id);

                    // Explicitly clear any existing data for this session
                    messageBufferRef.current.set(sessionData.id, []);
                    messageHistoryRef.current.set(sessionId, '');

                    // Initialize with empty response until we get content
                    setCurrentResponses(prev => {
                      return new Map(prev).set(sessionData!.id, { message: '' });
                    });
                    
                    if (parsedData.choices?.[0]?.delta?.content) {
                      addMessageChunk(sessionData.id, parsedData.choices[0].delta.content);
                    }

                    if (!promiseResolved) {
                      promiseResolved = true;
                    }
                  } else {
                    console.error('Invalid session data received:', sessionData);
                    rejectStream(new Error('Invalid session data'));
                  }
                } else if (parsedData.choices?.[0]?.delta?.content) {
                  addMessageChunk(sessionData.id, parsedData.choices[0].delta.content);
                }
              } catch (error) {
                console.error('Error parsing SSE data:', error);
                console.warn('Continuing despite parse error');
              }
            }
          }
        };

        const pump = async () => {
          try {
            while (true) {
              const { done, value } = await reader.read();
              
              if (done) {
                if (buffer) {
                  processChunk(buffer);
                }
                if (sessionData?.id) {
                  flushMessageBuffer(sessionData.id);
                }
                resolveStream();
                break;
              }

              const chunk = decoder.decode(value, { stream: true });
              processChunk(chunk);
            }
          } catch (error) {
            rejectStream(error);
          }
        };

        pump().catch(error => {
          console.error('Pump error:', error);
          rejectStream(error);
        });
      });

      await Promise.race([
        new Promise<void>((resolve) => {
          const checkResolved = () => {
            if (promiseResolved) {
              resolve();
            } else {
              setTimeout(checkResolved, 10);
            }
          };
          checkResolved();
        }),
        new Promise<void>((_, reject) => 
          setTimeout(() => reject(new Error('Timeout waiting for first chunk')), 5000)
        )
      ]);

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
    setCurrentSessionId: clearSessionData,
    currentResponses,
    updateCurrentResponse,
    stepInfos,
  };

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  );
};