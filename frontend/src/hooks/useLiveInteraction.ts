import { useState, useEffect, useMemo } from 'react';
import { INTERACTION_STATE_COMPLETE } from '../types';
import { useStreaming } from '../contexts/streaming';
import { TypesInteraction, TypesInteractionState } from '../api/api';

interface LiveInteractionResult {
  message: string;
  status: string;
  isComplete: boolean;
  isStale: boolean;
  stepInfos: any[]; // Add this line
}

const useLiveInteraction = (sessionId: string, initialInteraction: TypesInteraction | null, staleThreshold = 10000): LiveInteractionResult => {
  const [interaction, setInteraction] = useState<TypesInteraction | null>(initialInteraction);
  const { currentResponses, stepInfos } = useStreaming();
  const [recentTimestamp, setRecentTimestamp] = useState(Date.now());
  const [isStale, setIsStale] = useState(false);

  const isAppTryHelixDomain = useMemo(() => {
    return window.location.hostname === 'app.helix.ml';
  }, []);

  useEffect(() => {
    if (sessionId) {
      const currentResponse = currentResponses.get(sessionId);
      if (currentResponse) {
        setInteraction((prevInteraction: TypesInteraction | null): TypesInteraction => {
          if (prevInteraction === null) {
            return currentResponse as TypesInteraction;
          }
          return {
            ...prevInteraction,
            ...currentResponse,
          };
        });
        setRecentTimestamp(Date.now());
        // Reset stale state when we get an update
        if (isStale) {
          setIsStale(false);
        }
      }
    }
  }, [sessionId, currentResponses, isStale]);

  // Check for stale state, but only update when it changes from non-stale to stale
  useEffect(() => {
    // Only run stale check if we're on the tryhelix domain
    if (!isAppTryHelixDomain) return;
    
    const checkStale = () => {
      const shouldBeStale = (Date.now() - recentTimestamp) > staleThreshold;
      // Only update state if it's different (prevents unnecessary re-renders)
      if (shouldBeStale !== isStale) {
        setIsStale(shouldBeStale);
      }
    };
    
    // Check immediately and then set up interval
    checkStale();
    const intervalID = setInterval(checkStale, 1000);
    
    return () => clearInterval(intervalID);
  }, [recentTimestamp, staleThreshold, isStale, isAppTryHelixDomain]);

  return {
    message: interaction?.response_message || '',
    status: interaction?.state || '',    
    isComplete: interaction?.state === TypesInteractionState.InteractionStateComplete,
    isStale,
    stepInfos: stepInfos.get(sessionId) || [],
  };
};

export default useLiveInteraction;