import { useState, useEffect } from 'react';
import { IInteraction } from '../types';
import { useStreaming } from '../contexts/streaming';

interface LiveInteractionResult {
  message: string;
  status: string;
  progress: number;
  isComplete: boolean;
}

const useLiveInteraction = (sessionId: string, initialInteraction: IInteraction | null): LiveInteractionResult => {
  const [interaction, setInteraction] = useState<IInteraction | null>(initialInteraction);
  const { currentResponses } = useStreaming();

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
      }
    }
  }, [sessionId, currentResponses]);

  return {
    message: interaction?.message || '',
    status: interaction?.status || '',
    progress: interaction?.progress || 0,
    isComplete: interaction?.state === 'complete',
  };
};

export default useLiveInteraction;