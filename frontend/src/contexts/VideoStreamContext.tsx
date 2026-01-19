/**
 * VideoStreamContext - Tracks when video streaming is active
 *
 * When video streaming is active, other components (like screenshot thumbnails)
 * should reduce their polling frequency to avoid main thread contention.
 */

import React, { createContext, useContext, useState, useCallback, useMemo } from 'react';

interface VideoStreamContextValue {
  // Number of active video streams (usually 0 or 1)
  activeStreamCount: number;
  // Whether any video stream is currently active
  isStreaming: boolean;
  // Register a video stream as active
  registerStream: () => () => void;
}

const VideoStreamContext = createContext<VideoStreamContextValue | null>(null);

export const VideoStreamProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [activeStreamCount, setActiveStreamCount] = useState(0);

  const registerStream = useCallback(() => {
    setActiveStreamCount(c => c + 1);
    // Return unregister function
    return () => setActiveStreamCount(c => Math.max(0, c - 1));
  }, []);

  const value = useMemo(() => ({
    activeStreamCount,
    isStreaming: activeStreamCount > 0,
    registerStream,
  }), [activeStreamCount, registerStream]);

  return (
    <VideoStreamContext.Provider value={value}>
      {children}
    </VideoStreamContext.Provider>
  );
};

export const useVideoStream = (): VideoStreamContextValue => {
  const context = useContext(VideoStreamContext);
  if (!context) {
    // Return a default value if not wrapped in provider (graceful degradation)
    return {
      activeStreamCount: 0,
      isStreaming: false,
      registerStream: () => () => {},
    };
  }
  return context;
};
