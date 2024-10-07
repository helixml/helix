import React, { createContext, useContext, ReactNode } from 'react';

interface StreamingContextType {
  // We'll leave this empty for now
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
  // For now, we're not doing anything in this provider
  const value = {};

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  );
};