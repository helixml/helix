import { useState, useEffect, useCallback, useRef } from 'react'
import {
  IFileStoreItem,
  IKnowledgeSource,
} from '../types'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'
import useFilestore from './useFilestore'

export const default_max_depth = 1
export const default_max_pages = 5

const formatTimeRemaining = (seconds: number): string => {
  if (seconds < 60) {
    return `${Math.round(seconds)}s`
  } else if (seconds < 3600) {
    return `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`
  } else {
    return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  }
}

/**
 * Hook to manage single app state and operations
 * Consolidates app management logic from App.tsx
 */
export const useKnowledge = ({
  appId,
  saveKnowledgeToApp,
  onSaveApp,
}: {
  appId: string,
  // we save the list of knowledge by passing it up to the app
  // this function is hooked up to the useApp hook
  // so the knowledge state is updated via updating the app
  // once we have done this - we can call loadKnowledge internally
  // to get the latest state
  saveKnowledgeToApp: (knowledge: IKnowledgeSource[]) => Promise<void>,

  // used to trigger a re-index of the app
  onSaveApp: () => Promise<any>,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const filestore = useFilestore()
  const account = useAccount()
  
  const [expanded, setExpanded] = useState<string | false>(false);
  const [errors, setErrors] = useState<Record<string, string[]>>({});

  // the client side knowledge state that is mutated by the user
  const [knowledge, setKnowledge] = useState<IKnowledgeSource[]>([])

  // the server side knowledge state that is polled from the server
  // this keeps the "preparing" state in sync with the server
  // we pluck only the fields that we need to update in the client
  //
  // * state
  // * message
  // * progress
  // * crawled_sources
  // * version
  const [serverKnowledge, setServerKnowledge] = useState<IKnowledgeSource[]>([])

  const [directoryFiles, setDirectoryFiles] = useState<Record<string, IFileStoreItem[]>>({})
  const [deletingFiles, setDeletingFiles] = useState<Record<string, boolean>>({});  

  const [localUploadProgress, setLocalUploadProgress] = useState<any>(null)
  const uploadStartTimeRef = useRef<number | null>(null)
  const [uploadEta, setUploadEta] = useState<string | null>(null)
  const cancelTokenRef = useRef<AbortController | null>(null)
  const uploadCancelledRef = useRef<boolean>(false)
  const uploadSpeedRef = useRef<number[]>([])
  const [currentSpeed, setCurrentSpeed] = useState<number | null>(null)
  const [uploadingFileCount, setUploadingFileCount] = useState<number>(0)
  
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
    if (!currentKnowledge.length) return serverKnowledge
    
    return currentKnowledge.map(clientItem => {
      // Find matching server item by ID
      const serverItem = serverKnowledge.find(serverItem => serverItem.id === clientItem.id)
      
      // If no matching server item found, return client item unchanged
      if (!serverItem) return clientItem
      
      // Only update server-controlled fields
      return {
        ...clientItem,        
        state: serverItem.state,
        message: serverItem.message,
        progress: serverItem.progress,
        crawled_sources: serverItem.crawled_sources,
        version: serverItem.version
      }
    })
  }, [])

  /**
   * Loads knowledge for the app
   */
  const loadKnowledge = async (): Promise<IKnowledgeSource[]> => {
    if(!appId) return []
    const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${appId}`, undefined, {
      snackbar: false,
    })
    setKnowledge(knowledge || [])
    return knowledge || []
  }

  const loadServerKnowledge = async () => {
    if(!appId) return
    const knowledge = await api.get<IKnowledgeSource[]>(`/api/v1/knowledge?app_id=${appId}`, undefined, {
      snackbar: false,
    })
    setServerKnowledge(knowledge || [])
  }

  const handleRefreshKnowledge = async (id: string) => {
    try {
      await api.post(`/api/v1/knowledge/${id}/refresh`, null, {}, {
        snackbar: true,
      })
      await loadKnowledge()
    } catch (error) {
      console.error('Error refreshing knowledge:', error)
      snackbar.error('Failed to refresh knowledge')
    }
  }

  const handleCompleteKnowledgePreparation = async (id: string) => {
    try {
      await api.post(`/api/v1/knowledge/${id}/complete`, null, {}, {
        snackbar: true,
      })
      await loadKnowledge()
      snackbar.success('Knowledge preparation completed. Indexing started.')
    } catch (error) {
      console.error('Error completing knowledge preparation:', error)
      snackbar.error('Failed to complete knowledge preparation')
    }
  }

  const updateAllKnowledge = async (updatedKnowledge: IKnowledgeSource[]): Promise<IKnowledgeSource[]> => {
    console.log('[App] handleKnowledgeUpdate - Received updated knowledge sources:', updatedKnowledge)
    await saveKnowledgeToApp(updatedKnowledge)
    const newKnowledge = await loadKnowledge()
    // this can run in the background
    loadServerKnowledge()
    return newKnowledge
  }

  const handleAddSource = async (newSource: IKnowledgeSource) => {
    console.log('[KnowledgeEditor] handleAddSource - Adding new source:', newSource);

    const existingKnowledge = [...knowledge]
    const newKnowledge = await updateAllKnowledge([...knowledge, newSource])

    // Find the newly added knowledge source by comparing IDs between old and new arrays
    const newlyAddedSource = newKnowledge.find(newItem => 
      !existingKnowledge.some(existingItem => existingItem.id === newItem.id)
    )

    if(!newlyAddedSource) return

    setExpanded(`panel${newlyAddedSource.id}`)
    
    if (newlyAddedSource.source.filestore) {
      snackbar.info(`Knowledge source "${newlyAddedSource.name}" created. You can now upload files.`)

      // Explicitly load directory contents when a new filestore source is added
      if (newlyAddedSource.source.filestore.path) {
        // Initialize an empty array for the new knowledge source to prevent "undefined" checks
        setDirectoryFiles(prev => ({
          ...prev,
          [newlyAddedSource.id]: [] // Initialize with empty array
        }))
        // Then load the actual directory contents
        loadDirectoryContents(newlyAddedSource.source.filestore.path, newlyAddedSource.id)
      }
    }
  }

  const handleDeleteSource = async (id: string) => {
    console.log('[KnowledgeEditor] deleteSource - Deleting source:', id)
    const newSources = knowledge.filter(k => k.id != id)
    await updateAllKnowledge(newSources)
    snackbar.success(`Knowledge source deleted.`)
  }

  const updateSingleKnowledge = async (id: string, updatedSource: Partial<IKnowledgeSource>) => {
    let loadDirectoryContentsPath = ''

    const newKnowledgeList = knowledge.map(existingKnowledge => {
      if (existingKnowledge.id != id) return existingKnowledge

      loadDirectoryContentsPath = updatedSource.source?.filestore?.path && 
        updatedSource.source.filestore.path != existingKnowledge.source.filestore?.path ? updatedSource.source?.filestore?.path : ''

      const updatedKnowledge = { ...existingKnowledge, ...updatedSource }
      if (updatedKnowledge.refresh_schedule === 'custom') {
        updatedKnowledge.refresh_schedule = '0 0 * * *'
      } else if (updatedKnowledge.refresh_schedule === 'One off') {
        updatedKnowledge.refresh_schedule = ''
      }
  
      if (updatedKnowledge.source.web && updatedKnowledge.source.web.crawler) {
        updatedKnowledge.source.web.crawler.enabled = true
      }
  
      if (updatedKnowledge.source.web?.crawler) {
        updatedKnowledge.source.web.crawler.max_depth = updatedKnowledge.source.web.crawler.max_depth || default_max_depth
        updatedKnowledge.source.web.crawler.max_pages = updatedKnowledge.source.web.crawler.max_pages || default_max_pages
      }

      return updatedKnowledge
    })

    await updateAllKnowledge(newKnowledgeList)    

    if (loadDirectoryContentsPath) {
      await loadDirectoryContents(loadDirectoryContentsPath, id)
    }
  }

  const validateSources = () => {
    const newErrors: Record<string, string[]> = {};
    knowledge.forEach((source, index) => {      
      if ((!source.source.web?.urls || source.source.web.urls.length === 0) && !source.source.filestore?.path) {
        newErrors[`${index}`] = ["At least one URL or a filestore path must be specified."];
      }
    });
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const calculateEta = (loaded: number, total: number, startTime: number) => {
    const elapsedSeconds = (Date.now() - startTime) / 1000;
    
    if (elapsedSeconds < 0.1) {
      const percentComplete = loaded / total;
      if (percentComplete > 0) {
        return formatTimeRemaining(Math.ceil((total / loaded) * elapsedSeconds));
      }
      return "Calculating...";
    }
    
    const currentSpeedValue = loaded / elapsedSeconds;
    
    uploadSpeedRef.current.push(currentSpeedValue)
    if (uploadSpeedRef.current.length > 5) {
      uploadSpeedRef.current.shift()
    }
    
    const smoothedSpeed = uploadSpeedRef.current.reduce((sum, speed) => sum + speed, 0) / 
                         uploadSpeedRef.current.length
    
    setCurrentSpeed(smoothedSpeed)
    
    if (smoothedSpeed > 0) {
      const remainingBytes = total - loaded
      const remainingSeconds = remainingBytes / smoothedSpeed
      
      if (remainingSeconds < 1 && remainingSeconds > 0) {
        return "< 1s"
      }
      
      return formatTimeRemaining(remainingSeconds)
    }
    
    return "Calculating..."
  }
  
  const loadDirectoryContents = async (path: string, id: string) => {
    if (!path) return
    if (!id) {
      console.error('[useKnowledge] loadDirectoryContents called with empty ID')
      return
    }
    
    try {
      // Ensure consistent path handling
      let loadPath = path
      if (!loadPath.startsWith(`apps/${appId}/`)) {
        loadPath = `apps/${appId}/${loadPath}`
      }
      
      console.log(`[useKnowledge] Loading directory contents for ID: ${id}, path: ${loadPath}`)
      
      // Fetch files from filestore
      const files = await api.get<IFileStoreItem[]>('/api/v1/filestore/list', {
        params: {
          path: loadPath,
        }
      }) || []

      // Store files with knowledge ID as key
      setDirectoryFiles(prev => {
        const updated = {
          ...prev,
          [id]: files
        }
        console.log(`[useKnowledge] Updated directoryFiles:`, updated)
        return updated
      })
    } catch (error) {
      console.error(`[useKnowledge] Error loading directory contents for ID: ${id}:`, error)
    }
  }

  const handleFileUpload = async (id: string, files: File[]) => {
    console.log(`[useKnowledge] Starting file upload for knowledge ID: ${id}`)
    const source = knowledge.find(k => k.id === id)
    if(!source) {
      console.error(`[useKnowledge] Knowledge source not found for ID: ${id}`)
      return
    }
    if (!source.source.filestore?.path) {
      snackbar.error('No filestore path specified')
      return
    }

    // Ensure consistent path handling
    let uploadPath = source.source.filestore.path
    if (!uploadPath.startsWith(`apps/${appId}/`)) {
      uploadPath = `apps/${appId}/${uploadPath}`
    }
    console.log(`[useKnowledge] Upload path: ${uploadPath}`)

    uploadCancelledRef.current = false
    
    cancelTokenRef.current = new AbortController()
    
    try {
      uploadSpeedRef.current = []
      setCurrentSpeed(null)
      setUploadingFileCount(files.length)
      
      uploadStartTimeRef.current = Date.now()
      setLocalUploadProgress({
        percent: 0,
        uploadedBytes: 0,
        totalBytes: files.reduce((total, file) => total + file.size, 0)
      })
      setUploadEta("Calculating...") 

      const formData = new FormData()
      files.forEach((file) => {
        formData.append("files", file)
      })

      try {
        await api.post('/api/v1/filestore/upload', formData, {
          params: {
            path: uploadPath,
          },
          signal: cancelTokenRef.current.signal,
          onUploadProgress: (progressEvent) => {
            if (uploadCancelledRef.current) return
            
            const percent = progressEvent.total && progressEvent.total > 0 ?
              Math.round((progressEvent.loaded * 100) / progressEvent.total) : 0
            
            setLocalUploadProgress({
              percent,
              uploadedBytes: progressEvent.loaded || 0,
              totalBytes: progressEvent.total || 0,
            })
            
            if (uploadStartTimeRef.current && progressEvent.total && progressEvent.loaded > 0) {
              const eta = calculateEta(progressEvent.loaded, progressEvent.total, uploadStartTimeRef.current)
              setUploadEta(eta)
            }
          }
        })

        if (!uploadCancelledRef.current) {
          snackbar.success(`Successfully uploaded ${files.length} file${files.length !== 1 ? 's' : ''}`)

          // Get updated file list after successful upload
          const updatedFiles = await filestore.getFiles(uploadPath)
          console.log(`[useKnowledge] Upload complete, found ${updatedFiles.length} files for ID: ${id}`, updatedFiles)
          
          // Update directory files state with knowledge ID as key
          setDirectoryFiles(prev => ({
            ...prev,
            [id]: updatedFiles
          }))
         
          await onSaveApp()
          await handleRefreshKnowledge(id)

          snackbar.info('Re-indexing started for newly uploaded files. This may take a few minutes.')
        }
      } catch (uploadError: unknown) {
        if (
          typeof uploadError === 'object' && 
          uploadError !== null && 
          ('name' in uploadError) && 
          (uploadError.name === 'AbortError' || uploadError.name === 'CanceledError')
        ) {
          console.log('Upload was cancelled by user')
          return
        }
        
        if (!uploadCancelledRef.current) {
          console.error('Direct upload failed, falling back to onUpload method:', uploadError)
          
          try {
            await filestore.upload(uploadPath, files)
            
            if (!uploadCancelledRef.current) {
              snackbar.success(`Successfully uploaded ${files.length} file${files.length !== 1 ? 's' : ''}`)
              
              // Get updated file list after fallback upload
              const fallbackFiles = await filestore.getFiles(uploadPath)
              console.log(`[useKnowledge] Fallback upload complete, found ${fallbackFiles.length} files for ID: ${id}`, fallbackFiles)
              
              // Update directory files state with knowledge ID as key
              setDirectoryFiles(prev => ({
                ...prev,
                [id]: fallbackFiles
              }))

              await onSaveApp()
              await handleRefreshKnowledge(id)

              snackbar.info('Re-indexing started for newly uploaded files. This may take a few minutes.')
            }
          } catch (fallbackError) {
            if (!uploadCancelledRef.current) {
              console.error('Error in fallback upload:', fallbackError)
              snackbar.error('Failed to upload files. Please try again.')
            }
          }
        }
      }
    } catch (error: unknown) {
      if (!uploadCancelledRef.current) {
        console.error('Error uploading files:', error)
        snackbar.error('Failed to upload files. Please try again.')
      }
    } finally {
      if (uploadCancelledRef.current) {
        setLocalUploadProgress(null)
        uploadStartTimeRef.current = null
        setUploadEta(null)
        setUploadingFileCount(0)
        cancelTokenRef.current = null
      } else {
        setTimeout(() => {
          setLocalUploadProgress(null)
          uploadStartTimeRef.current = null
          setUploadEta(null)
          setUploadingFileCount(0)
          cancelTokenRef.current = null
        }, 1000)
      }
      
      uploadCancelledRef.current = false
    }

    const uploadedKnowledge = knowledge.find(k => k.id === id)

    if (uploadedKnowledge) {
      if (uploadedKnowledge.source.filestore?.path) {
        loadDirectoryContents(uploadedKnowledge.source.filestore.path, uploadedKnowledge.id)
      }
    }
  }

  const handleCancelUpload = () => {
    if (cancelTokenRef.current) {
      uploadCancelledRef.current = true;
      
      snackbar.info('Upload cancelled');
      
      cancelTokenRef.current.abort();
      
      setLocalUploadProgress(null);
      uploadStartTimeRef.current = null;
      setUploadEta(null);
      setUploadingFileCount(0);
      cancelTokenRef.current = null;
    }
  };
  
  const handleDeleteFile = async (id: string, fileName: string) => {
    const source = knowledge.find(k => k.id === id)
    if (!source) {
      snackbar.error('Knowledge source not found')
      return
    }
    if (!source.source.filestore?.path) {
      snackbar.error('No filestore path specified');
      return
    }
    try {
      const fileId = `${id}-${fileName}`;
      setDeletingFiles(prev => ({
        ...prev,
        [fileId]: true
      }));
      
      let basePath = source.source.filestore.path;
      if (!basePath.startsWith(`apps/${appId}/`)) {
        basePath = `apps/${appId}/${basePath}`;
      }
      
      const filePath = `${basePath}/${fileName}`;
      
      const response = await api.delete('/api/v1/filestore/delete', {
        params: {
          path: filePath,
        }
      });
      
      if (response) {
        snackbar.success(`File "${fileName}" deleted successfully`);
        
        const files = await filestore.getFiles(basePath);
        console.log(`[useKnowledge] After file deletion, found ${files.length} files for ID: ${id}`, files);
        
        setDirectoryFiles(prev => ({
          ...prev,
          [id]: files
        }));
      } else {
        snackbar.error(`Failed to delete file "${fileName}"`);
      }
    } catch (error) {
      console.error('Error deleting file:', error);
      snackbar.error('An error occurred while deleting the file');
    } finally {
      const fileId = `${id}-${fileName}`;
      setDeletingFiles(prev => ({
        ...prev,
        [fileId]: false
      }));
    }
  };

  useEffect(() => {
    validateSources()
  }, [knowledge])
  
  useEffect(() => {
    if(!account.user) return
    if(!appId) return
    let intervalID: NodeJS.Timeout | null = null
    const runAsync = async () => {
      intervalID = setInterval(() => {
        loadServerKnowledge()
      }, 2000)
      loadServerKnowledge()
      const knowledge = await loadKnowledge()

      knowledge.forEach((source) => {
        if (source.source.filestore?.path) {
          loadDirectoryContents(source.source.filestore.path, source.id)
        }
      })
    }

    runAsync()

    return () => {
      if (intervalID) {
        clearInterval(intervalID)
        intervalID = null
      }
    }
  }, [
    account.user,
    appId,
  ])

  return {
    // Knowledge CRUD methods
    knowledge,
    serverKnowledge,

    updateSingleKnowledge,
    handleRefreshKnowledge,
    handleCompleteKnowledgePreparation,
    handleAddSource,
    handleDeleteSource,

    // UI state
    expanded,
    setExpanded,
    errors,

    // file upload handlers
    handleFileUpload,
    handleCancelUpload,
    handleDeleteFile,
    loadDirectoryContents,

    // file upload state
    directoryFiles,
    deletingFiles,
    localUploadProgress,
    uploadEta,
    currentSpeed,
    uploadingFileCount,
  }
}

export default useKnowledge 