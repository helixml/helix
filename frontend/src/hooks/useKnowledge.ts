import { useState, useEffect, useCallback, useRef } from 'react'
import {
  IKnowledgeSource,
} from '../types'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'

/**
 * Hook to manage single app state and operations
 * Consolidates app management logic from App.tsx
 */
export const useKnowledge = (
  appId: string,
  onUpdateKnowledge: (knowledge: IKnowledgeSource[]) => void,
  opts: {
    showErrors?: boolean
  } = {
    showErrors: true, 
  },
) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()

  const {
    showErrors,
  } = opts
  
  /**
   * 
   * 
   * hook state
   * 
   * 
   */
  const [knowledge, setKnowledge] = useState<IKnowledgeSource[]>([])
  const [pollingActive, setPollingActive] = useState(true)
  const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null)
  
  /**
   * 
   * 
   * knowledge handlers
   * 
   * 
   */

  /**
   * Merges server-controlled knowledge fields with current knowledge state
   * Only updates fields that the server controls during background processing
   */
  const mergeKnowledgeUpdates = useCallback((currentKnowledge: IKnowledgeSource[], serverKnowledge: IKnowledgeSource[]) => {
    // If we don't have any current knowledge, just use server knowledge
    if (!currentKnowledge.length) return serverKnowledge;
    
    return currentKnowledge.map(clientItem => {
      // Find matching server item by ID
      const serverItem = serverKnowledge.find(serverItem => serverItem.id === clientItem.id);
      
      // If no matching server item found, return client item unchanged
      if (!serverItem) return clientItem;
      
      // Only update server-controlled fields
      return {
        ...clientItem,        
        state: serverItem.state,
        message: serverItem.message,
        progress: serverItem.progress,
        crawled_sources: serverItem.crawled_sources,
        version: serverItem.version
      };
    });
  }, []);

  /**
   * Loads knowledge for the app
   */
  const loadKnowledge = useCallback(async () => {
    if(!appId) return
    const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${appId}`, undefined, {
      snackbar: showErrors,
    })
    setKnowledge(knowledge || [])
  }, [api, appId, showErrors])

  const handleRefreshKnowledge = useCallback((id: string) => {
    api.post(`/api/v1/knowledge/${id}/refresh`, null, {}, {
      snackbar: true,
    }).then(() => {
      // Call fetchKnowledge immediately after the refresh is initiated
      loadKnowledge();
    }).catch((error) => {
      console.error('Error refreshing knowledge:', error);
      snackbar.error('Failed to refresh knowledge');
    });
  }, [api, loadKnowledge]);

  const handleCompleteKnowledgePreparation = useCallback((id: string) => {
    api.post(`/api/v1/knowledge/${id}/complete`, null, {}, {
      snackbar: true,
    }).then(() => {
      // Call fetchKnowledge immediately after completing preparation
      loadKnowledge();
      snackbar.success('Knowledge preparation completed. Indexing started.');
    }).catch((error) => {
      console.error('Error completing knowledge preparation:', error);
      snackbar.error('Failed to complete knowledge preparation');
    });
  }, [api, loadKnowledge]);

  
  const handleKnowledgeUpdate = useCallback((updatedKnowledge: IKnowledgeSource[]) => {
    console.log('[App] handleKnowledgeUpdate - Received updated knowledge sources:', updatedKnowledge)
    onUpdateKnowledge(updatedKnowledge)
    setKnowledge(updatedKnowledge)
  }, [onUpdateKnowledge])
  
  /**
   * Polling effect for knowledge updates
   * Regularly checks for changes to server-controlled fields
   */
  useEffect(() => {
    if (!appId || !account.user) return;

    // Function to poll for knowledge updates
    const pollKnowledge = async () => {
      try {
        // Fetch latest knowledge from server
        const serverKnowledge = await api.get<IKnowledgeSource[]>(
          `/api/v1/knowledge?app_id=${appId}`, 
          undefined, 
          { snackbar: false } // Silent - don't show errors for polling
        );
        
        if (!serverKnowledge) return;
        
        // Merge with current knowledge, preserving user edits
        const updatedKnowledge = mergeKnowledgeUpdates(knowledge, serverKnowledge);
        
        // Only update if something changed
        if (JSON.stringify(updatedKnowledge) !== JSON.stringify(knowledge)) {
          console.log('[useApp] Polling detected knowledge changes');
          setKnowledge(updatedKnowledge);
          
          // We won't try to update app state directly, since the knowledge state
          // will be used by the app components anyway
        }
        
        // Check if we should stop polling
        const allComplete = updatedKnowledge.every(k => 
          k.state === 'complete' || k.state === 'error'
        );
        
        if (allComplete) {
          console.log('[useApp] All knowledge processing complete, stopping polling');
          setPollingActive(false);
        }
        
      } catch (error) {
        console.error('Error polling knowledge:', error);
        // Don't stop polling on errors - retry next interval
      }
    };
    
    // Start polling if active
    if (pollingActive) {
      // Initial poll
      pollKnowledge();
      
      // Set up interval
      pollingIntervalRef.current = setInterval(pollKnowledge, 2000);
    }
    
    // Cleanup function
    return () => {
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current);
        pollingIntervalRef.current = null;
      }
    };
  }, [appId, account.user, pollingActive, api, knowledge, mergeKnowledgeUpdates]);
  
  /**
   * Effect to restart polling when new knowledge is added
   */
  useEffect(() => {
    // If knowledge length increases, restart polling
    if (knowledge.length > 0 && !pollingActive) {
      const hasProcessingItems = knowledge.some(k => 
        k.state !== 'complete' && k.state !== 'error'
      );
      
      if (hasProcessingItems) {
        console.log('[useApp] Detected processing knowledge items, restarting polling');
        setPollingActive(true);
      }
    }
  }, [knowledge, pollingActive]);
  
  /**
   * The main loading that will trigger when the page loads
   */ 
  useEffect(() => {
    if (!appId) return
    loadKnowledge()
  }, [
    appId,
    account.user,
  ])

  return {
    // Knowledge methods
    knowledge,
    handleRefreshKnowledge,
    handleCompleteKnowledgePreparation,
    handleKnowledgeUpdate,
  }
}

export default useKnowledge 