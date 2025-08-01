import React, { createContext, useContext, ReactNode, useState, useCallback, useEffect, useRef } from 'react';
import ReconnectingWebSocket from 'reconnecting-websocket';
import { IWebsocketEvent, WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE, WORKER_TASK_RESPONSE_TYPE_PROGRESS, ISessionChatRequest, ISessionType } from '../types';
import useAccount from '../hooks/useAccount';
import useSessions from '../hooks/useSessions';
import { TypesInteraction, TypesMessage, TypesSession } from '../api/api';
import { sessionStepsQueryKey } from '../services/sessionService';
import { useQueryClient } from '@tanstack/react-query';

interface NewInferenceParams {
  regenerate?: boolean;
  type: ISessionType;
  message: string;
  messages?: TypesMessage[];
  image?: string;
  image_filename?: string;
  appId?: string;
  assistantId?: string;
  interactionId?: string;
  provider?: string;
  modelName?: string;
  sessionId?: string;
  orgId?: string;
  attachedImages?: File[];
}

interface StreamingContextType {
  NewInference: (params: NewInferenceParams) => Promise<TypesSession>;
  setCurrentSessionId: (sessionId: string) => void;
  currentResponses: Map<string, Partial<TypesInteraction>>;
  stepInfos: Map<string, any[]>;
  updateCurrentResponse: (sessionId: string, interaction: Partial<TypesInteraction>) => void;
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
  const queryClient = useQueryClient();
  const [currentResponses, setCurrentResponses] = useState<Map<string, Partial<TypesInteraction>>>(new Map());
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
        prompt_message: newMessage
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
          let updatedInteraction: Partial<TypesInteraction> = { ...current };

          if (workerResponse.type === WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
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
      const lastInteraction = parsedData.session.interactions?.[parsedData.session.interactions.length - 1];

      if (!lastInteraction) return;
      
      // Update currentResponses with the latest interaction state
      // This ensures useLiveInteraction will receive the updated state
      if (lastInteraction.id) {
        requestAnimationFrame(() => {
          setCurrentResponses(prev => {
            const current = prev.get(currentSessionId) || {};
            const updatedInteraction: Partial<TypesInteraction> = {
              ...current,
              id: lastInteraction.id,
              state: lastInteraction.state,
              // Copy any other important fields from the interaction
              prompt_message: lastInteraction.prompt_message || current.prompt_message
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

      handleWebsocketEvent(parsedData);

      if (parsedData.step_info && parsedData.step_info.type === "thinking") {
      // Don't reload on thinking info events as we will get a lot of them
        return
      }      

      // Invalidate the stepInfos query
      queryClient.invalidateQueries({ queryKey: sessionStepsQueryKey(currentSessionId) });
      
      // Reload all sessions to refresh the name in the sidebar
      sessions.loadSessions()
    };

    rws.addEventListener('message', messageHandler);

    return () => {
      rws.removeEventListener('message', messageHandler);
      rws.close();
    };
  }, [account.token, currentSessionId, handleWebsocketEvent]);

  const NewInference = async ({
    regenerate = false,
    type,
    message,
    messages,
    appId = '',
    assistantId = '',    
    provider = '',
    modelName = '',    
    sessionId = '',
    interactionId = '',
    orgId = '',
    image = undefined,
    image_filename = undefined,
    attachedImages = [],
  }: NewInferenceParams): Promise<TypesSession> => {
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

    // Construct the content parts first
    const currentContentParts: any[] = [];
    let determinedContentType: string = "text"; // Default for MessageContent.content_type

    // Add text part if message is provided
    if (message) {
      currentContentParts.push({
        type: 'text',
        text: message,
      });
    }

    // Handle attached images
    if (attachedImages && attachedImages.length > 0) {
      for (const file of attachedImages) {
        const reader = new FileReader();
        const imageData = await new Promise<string>((resolve) => {
          reader.onloadend = () => resolve(reader.result as string);
          reader.readAsDataURL(file);
        });
        
        currentContentParts.push({
          type: 'image_url',
          image_url: {
            url: imageData,
          },
        });
      }
      determinedContentType = "multimodal_text";
    } else if (image && image_filename) {
      currentContentParts.push({
        type: 'image_url',
        image_url: {
          url: image,
        },
      });
      determinedContentType = "multimodal_text";
    } else if (!message) {
      console.warn("NewInference called with no message and no image.");
    }

    // This is the payload for Message.Content, matching the Go types.MessageContent struct
    const messagePayloadContent = {
      content_type: determinedContentType,
      parts: currentContentParts,
    };

    // Assign the constructed content to the message
    const sessionChatRequest: ISessionChatRequest = {
      regenerate: regenerate,
      type, // This is ISessionType (e.g. text, image) for the overall session/request
      stream: true,
      app_id: appId,
      organization_id: orgId,
      assistant_id: assistantId,
      interaction_id: interactionId,
      provider: provider,
      model: modelName,      
      session_id: sessionId,
      messages: [
        {
          role: 'user',
          content: messagePayloadContent as any, // Use the correctly structured object, cast to any to bypass TS type mismatch
        },
      ],
    };

    // If messages are supplied in the request, overwrite the default user message
    if (messages && messages.length > 0) {
      sessionChatRequest.messages = messages;
    }

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
      let sessionData: TypesSession | null = null;
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
                    newMap.delete(sessionData?.id || '');
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
                      return new Map(prev).set(sessionData?.id || '', { prompt_message: '' });
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
                  addMessageChunk(sessionData?.id || '', parsedData.choices[0].delta.content);
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

  const updateCurrentResponse = (sessionId: string, interaction: Partial<TypesInteraction>) => {
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