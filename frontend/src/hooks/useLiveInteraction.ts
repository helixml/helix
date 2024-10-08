import { useState, useEffect } from 'react';
import { IInteraction } from '../types';
import { useStreaming } from '../contexts/streaming';

interface LiveInteractionResult {
  message: string;
  status: string;
  progress: number;
  isComplete: boolean;
  isStale: boolean;
}

const useLiveInteraction = (sessionId: string, initialInteraction: IInteraction | null): LiveInteractionResult => {
  const [interaction, setInteraction] = useState<IInteraction | null>(initialInteraction);
  const { currentResponses } = useStreaming();
  const [isStale, setIsStale] = useState(false);

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
        setIsStale(false);
      } else {
        // XXX this is not what isStale is intended to mean, it's meant to have
        // a timer, did we lose that somewhere?
        setIsStale(true);
      }
    }
  }, [sessionId, currentResponses]);

  return {
    message: interaction?.message || '',
    status: interaction?.status || '',
    progress: interaction?.progress || 0,
    isComplete: interaction?.state === 'complete',
    isStale,
  };
};

export default useLiveInteraction;