import { useState, useCallback } from 'react';

interface RDPConnectionInfo {
  host: string;
  rdp_port: number;
  username: string;
  rdp_password: string;
  proxy_url?: string;
  session_id?: string;
  status?: string;
}

interface UseRDPConnectionResult {
  connectionInfo: RDPConnectionInfo | null;
  isLoading: boolean;
  error: string | null;
  fetchConnectionInfo: (sessionId: string, isRunner?: boolean) => Promise<RDPConnectionInfo | null>;
  clearError: () => void;
}

export const useRDPConnection = (): UseRDPConnectionResult => {
  const [connectionInfo, setConnectionInfo] = useState<RDPConnectionInfo | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchConnectionInfo = useCallback(async (sessionId: string, isRunner = false): Promise<RDPConnectionInfo | null> => {
    // Note: When isRunner=true, sessionId parameter actually contains the runnerID
    if (!sessionId) {
      setError('Session ID is required');
      return null;
    }

    setIsLoading(true);
    setError(null);

    try {
      let response: Response;
      
      if (isRunner) {
        // For runners (fleet page), use runner-specific endpoint
        const runnerEndpoint = `/api/v1/external-agents/runners/${sessionId}/rdp`;
        console.log('Fetching runner RDP info from:', runnerEndpoint);
        response = await fetch(runnerEndpoint);
      } else {
        // For sessions, try external agent RDP endpoint first
        const sessionEndpoint = `/api/v1/external-agents/${sessionId}/rdp`;
        console.log('Fetching session RDP info from:', sessionEndpoint);
        response = await fetch(sessionEndpoint);
        
        if (!response.ok) {
          // Fallback to session RDP endpoint if it exists
          const fallbackEndpoint = `/api/v1/sessions/${sessionId}/rdp-connection`;
          console.log('Trying fallback endpoint:', fallbackEndpoint);
          response = await fetch(fallbackEndpoint);
        }
      }

      if (!response.ok) {
        console.error('RDP endpoint failed:', {
          status: response.status,
          statusText: response.statusText,
          url: response.url,
          isRunner
        });
        throw new Error(`Failed to fetch RDP connection info: ${response.status} ${response.statusText}`);
      }

      const data = await response.json();
      console.log('RDP connection info received:', data);

      // Normalize the response format
      const normalizedInfo: RDPConnectionInfo = {
        host: data.host || 'localhost',
        rdp_port: data.rdp_port || 3389,
        username: data.username || 'zed',
        rdp_password: data.rdp_password || '',
        proxy_url: isRunner 
          ? `/api/v1/external-agents/runners/${sessionId}/guac/proxy`  // sessionId is actually runnerID when isRunner=true
          : `/api/v1/sessions/${sessionId}/guac/proxy`,               // sessionId is actual sessionID when isRunner=false
        session_id: data.session_id || sessionId,
        status: data.status || 'unknown'
      };

      // Validate that we have essential connection info
      if (!normalizedInfo.rdp_password) {
        console.warn('RDP password not provided by server - this is a security issue');
        // Don't fail completely, but log the security issue
      }

      console.log('Normalized connection info:', normalizedInfo);
      setConnectionInfo(normalizedInfo);
      return normalizedInfo;

    } catch (err: any) {
      const errorMessage = err.message || 'Failed to fetch RDP connection info';
      console.error('RDP connection fetch error:', errorMessage);
      setError(errorMessage);
      setConnectionInfo(null);
      return null;
    } finally {
      setIsLoading(false);
    }
  }, []);

  const clearError = useCallback(() => {
    setError(null);
  }, []);

  return {
    connectionInfo,
    isLoading,
    error,
    fetchConnectionInfo,
    clearError
  };
};