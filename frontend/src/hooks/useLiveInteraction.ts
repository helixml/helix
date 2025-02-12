import { useState, useEffect, useMemo } from 'react';
import { IInteraction, INTERACTION_STATE_COMPLETE } from '../types';
import { useStreaming } from '../contexts/streaming';

interface LiveInteractionResult {
  message: string;
  status: string;
  progress: number;
  isComplete: boolean;
  isStale: boolean;
  stepInfos: any[]; // Add this line
}

const useLiveInteraction = (sessionId: string, initialInteraction: IInteraction | null, staleThreshold = 10000): LiveInteractionResult => {
  const [interaction, setInteraction] = useState<IInteraction | null>(initialInteraction);
  const { currentResponses, stepInfos } = useStreaming();
  const [recentTimestamp, setRecentTimestamp] = useState(Date.now());
  const [staleCounter, setStaleCounter] = useState(0);

  const isAppTryHelixDomain = useMemo(() => {
    return window.location.hostname === 'app.tryhelix.ai';
  }, []);

  useEffect(() => {
    if (sessionId) {
      const currentResponse = currentResponses.get(sessionId);
      if (currentResponse) {
        setInteraction((prevInteraction: IInteraction | null): IInteraction => {
          if (prevInteraction === null) {
            return currentResponse as IInteraction;
          }
          return {
            ...prevInteraction,
            ...currentResponse,
          };
        });
        setRecentTimestamp(Date.now());
      }
    }
  }, [sessionId, currentResponses]);

  useEffect(() => {
    const intervalID = setInterval(() => {
      setStaleCounter(c => c + 1);
    }, 1000);
    return () => clearInterval(intervalID);
  }, []);

  const isStale = useMemo(() => {
    if (!isAppTryHelixDomain) {
      return false;
    }
    return (Date.now() - recentTimestamp) > staleThreshold;
  }, [recentTimestamp, staleThreshold, staleCounter, isAppTryHelixDomain]);

  return {
    message: interaction?.message || '',
    status: interaction?.status || '',
    progress: interaction?.progress || 0,
    isComplete: interaction?.state === INTERACTION_STATE_COMPLETE && interaction?.finished,
    isStale,
    stepInfos: stepInfos.get(sessionId) || [], // Add this line
  };
};

export default useLiveInteraction;