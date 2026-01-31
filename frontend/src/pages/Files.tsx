import React, { FC, useState, useCallback, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'


import Page from '../components/system/Page'
import MonacoEditor from '../components/widgets/MonacoEditor'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useLightTheme from '../hooks/useLightTheme'
import { useSaveFilestoreFile, useFilestoreConfig } from '../services/filestoreService'
import { FilestoreItem } from '../api/api'

const Files: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  
  const {
    params,
  } = useRouter()

  const {
    file_path,
  } = params
  
  const [ selectedFile, setSelectedFile ] = useState<FilestoreItem | null>(null)
  const [ fileContent, setFileContent ] = useState('')
  const [ originalContent, setOriginalContent ] = useState('')
  const [ isLoadingContent, setIsLoadingContent ] = useState(false)
  const [ hasUnsavedChanges, setHasUnsavedChanges ] = useState(false)
  
  // Refs for debouncing
  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null)
  const lastSavedContentRef = useRef<string>('')

  const {
    data: configData,
    isLoading: isLoadingConfig,
    error: configError
  } = useFilestoreConfig()  

  // Note: Files listing is now handled by FilesSidebar component
  // This page only handles file content display and editing

  // Save file mutation
  const saveFileMutation = useSaveFilestoreFile()

  // Monitor URL query parameter for file_path changes
  useEffect(() => {
    if (file_path) {
      // Only show preview for files (not directories)
      // Check if the path has a file extension to determine if it's a file
      const fileName = file_path.split('/').pop() || file_path
      const hasExtension = fileName.includes('.') && !fileName.endsWith('/')
      
      if (hasExtension) {
        // Create a file object from the path for content loading
        const file: FilestoreItem = {
          name: fileName,
          path: file_path,
          directory: false,
          url: `/api/v1/filestore/viewer/${file_path}` // Construct the viewer URL
        }
        setSelectedFile(file)
        loadFileContent(file)
      } else {
        // It's a directory, don't show preview
        setSelectedFile(null)
        setFileContent('')
      }
    } else {
      setSelectedFile(null)
      setFileContent('')
    }
  }, [file_path])

  const loadFileContent = useCallback(async (file: FilestoreItem) => {
    if (!file.url || file.directory) return
    
    setIsLoadingContent(true)
    try {
      const response = await fetch(file.url)
      if (response.ok) {
        const content = await response.text()
        setFileContent(content)
        setOriginalContent(content)
        lastSavedContentRef.current = content
        setHasUnsavedChanges(false)
      } else {
        snackbar.error('Failed to load file content')
        setFileContent('')
        setOriginalContent('')
        lastSavedContentRef.current = ''
        setHasUnsavedChanges(false)
      }
    } catch (error) {
      console.error('Error loading file content:', error)
      snackbar.error('Failed to load file content')
      setFileContent('')
      setOriginalContent('')
      lastSavedContentRef.current = ''
      setHasUnsavedChanges(false)
    } finally {
      setIsLoadingContent(false)
    }
  }, [snackbar])

  const getFileLanguage = useCallback((fileName: string) => {
    const extension = fileName.split('.').pop()?.toLowerCase()
    const languageMap: { [key: string]: string } = {
      'js': 'javascript',
      'ts': 'typescript',
      'jsx': 'javascript',
      'tsx': 'typescript',
      'py': 'python',
      'java': 'java',
      'cpp': 'cpp',
      'c': 'c',
      'h': 'c',
      'go': 'go',
      'rs': 'rust',
      'php': 'php',
      'rb': 'ruby',
      'swift': 'swift',
      'kt': 'kotlin',
      'scala': 'scala',
      'sh': 'shell',
      'bash': 'shell',
      'ps1': 'powershell',
      'bat': 'batch',
      'cmd': 'batch',
      'yaml': 'yaml',
      'yml': 'yaml',
      'json': 'json',
      'xml': 'xml',
      'html': 'html',
      'css': 'css',
      'scss': 'scss',
      'sass': 'sass',
      'less': 'less',
      'sql': 'sql',
      'md': 'markdown',
      'txt': 'plaintext',
      'log': 'plaintext',
    }
    return languageMap[extension || ''] || 'plaintext'
  }, [])

  const isImageFile = useCallback((fileName: string) => {
    const extension = fileName.split('.').pop()?.toLowerCase()
    return ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'svg', 'webp'].includes(extension || '')
  }, [])

  const isTextFile = useCallback((fileName: string) => {
    const extension = fileName.split('.').pop()?.toLowerCase()
    const textExtensions = [
      'js', 'ts', 'jsx', 'tsx', 'py', 'java', 'cpp', 'c', 'h', 'go', 'rs', 'php', 'rb', 'swift', 'kt', 'scala',
      'sh', 'bash', 'ps1', 'bat', 'cmd', 'yaml', 'yml', 'json', 'xml', 'html', 'css', 'scss', 'sass', 'less',
      'sql', 'md', 'txt', 'log', 'csv', 'ini', 'cfg', 'conf', 'env', 'dockerfile', 'makefile'
    ]
    return textExtensions.includes(extension || '')
  }, [])

  // File editing functions

  const handleContentChange = useCallback((newContent: string) => {
    setFileContent(newContent)
    setHasUnsavedChanges(newContent !== originalContent)
    
    // Clear existing timeout
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current)
    }
    
    // Set new timeout for debounced save
    saveTimeoutRef.current = setTimeout(() => {
      if (newContent !== lastSavedContentRef.current && selectedFile) {
        saveFileMutation.mutate(
          { path: selectedFile.path || selectedFile.name || '', content: newContent, config: configData },
          {
            onSuccess: () => {
              lastSavedContentRef.current = newContent
              setHasUnsavedChanges(false)
              snackbar.success('File saved automatically')
            },
            onError: (error) => {
              console.error('Auto-save failed:', error)
              snackbar.error('Failed to save file automatically')
            }
          }
        )
      }
    }, 2000) // 2 second debounce
  }, [originalContent, selectedFile, saveFileMutation, snackbar])


  const handleEditorBlur = useCallback(() => {
    // Save immediately on blur if there are unsaved changes
    if (hasUnsavedChanges && selectedFile) {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current)
        saveTimeoutRef.current = null
      }
      
      saveFileMutation.mutate(
        { path: selectedFile.path || selectedFile.name || '', content: fileContent, config: configData },
        {
          onSuccess: () => {
            setOriginalContent(fileContent)
            lastSavedContentRef.current = fileContent
            setHasUnsavedChanges(false)
            snackbar.success('File saved')
          },
          onError: (error) => {
            console.error('Save failed:', error)
            snackbar.error('Failed to save file')
          }
        }
      )
    }
  }, [hasUnsavedChanges, selectedFile, fileContent, saveFileMutation, snackbar])

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current)
      }
    }
  }, [])



  if(!account.user) return null
  return (
    <Page
      breadcrumbTitle="Files"
    >
      <Container
        maxWidth="xl"
        sx={{
          display: 'block',
        }}
      >
        <Box sx={{ width: '100%', pl: 2, pr: 2, mt: 2 }}>
          <Box sx={{
            backgroundColor: themeConfig.darkPanel,
            p: 0,              
            borderRadius: 2,
            boxShadow: '0 4px 24px 0 rgba(0,0,0,0.12)',
            width: '100%'
          }}>
            <Box sx={{ width: '100%', p: 0, pl: 4 }}>
              <Box sx={{
                overflow: 'auto',
                pb: 8,
                minHeight: 'calc(100vh - 120px)',
                ...lightTheme.scrollbar
              }}>
                <Box sx={{ mt: "-1px", p: 0 }}>
                  {/* File Content Display */}
                  {selectedFile ? (
                    <Box sx={{ p: 2 }}>
                      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                        <Typography variant="h6">
                          {selectedFile.name}
                          {hasUnsavedChanges && (
                            <Typography component="span" variant="body2" color="warning.main" sx={{ ml: 1 }}>
                              (unsaved changes)
                            </Typography>
                          )}
                        </Typography>
                        
                      </Box>
                      
                      {isLoadingContent ? (
                        <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
                          <Typography>Loading content...</Typography>
                        </Box>
                      ) : isImageFile(selectedFile.name || '') ? (
                        <Box sx={{ textAlign: 'center' }}>
                          <img
                            src={selectedFile.url}
                            alt={selectedFile.name || ''}
                            style={{
                              maxWidth: '100%',
                              maxHeight: '400px',
                              objectFit: 'contain',
                            }}
                          />
                        </Box>
                      ) : isTextFile(selectedFile.name || '') ? (
                        <MonacoEditor
                          value={fileContent}
                          onChange={handleContentChange}
                          onBlur={handleEditorBlur}
                          language={getFileLanguage(selectedFile.name || '')}
                          readOnly={false}
                          autoHeight={true}
                          minHeight={200}
                          maxHeight={400}
                          theme="helix-dark"
                          options={{
                            fontSize: 14,
                            lineNumbers: 'on',
                            folding: true,
                            lineDecorationsWidth: 0,
                            lineNumbersMinChars: 3,
                            scrollBeyondLastLine: false,
                            minimap: { enabled: false },
                            wordWrap: 'on',
                            wrappingIndent: 'indent',
                          }}
                        />
                      ) : (
                        <Typography variant="body2" color="text.secondary">
                          File type not supported for preview
                        </Typography>
                      )}
                    </Box>
                  ) : (
                    <Box sx={{ p: 4, textAlign: 'center' }}>
                      <Typography variant="body1" color="text.secondary">
                        Select a file from the sidebar to preview it here
                      </Typography>
                    </Box>
                  )}
                </Box>
              </Box>
            </Box>
          </Box>
        </Box>
      </Container>
    </Page>
  )
}

export default Files