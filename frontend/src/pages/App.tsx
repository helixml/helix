import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import React, { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { v4 as uuidv4 } from 'uuid'
import { stringify as stringifyYaml } from 'yaml'
import ApiIntegrations from '../components/app/ApiIntegrations'
import APIKeysSection from '../components/app/APIKeysSection'
import AppSettings from '../components/app/AppSettings'
import CodeExamples from '../components/app/CodeExamples'
import DevelopersSection from '../components/app/DevelopersSection'
import GPTScriptsSection from '../components/app/GPTScriptsSection'
import KnowledgeEditor from '../components/app/KnowledgeEditor'
import PreviewPanel from '../components/app/PreviewPanel'
import ZapierIntegrations from '../components/app/ZapierIntegrations'
import Page from '../components/system/Page'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import Window from '../components/widgets/Window'
import useAccount from '../hooks/useAccount'
import useApp from '../hooks/useApp'
import useRouter from '../hooks/useRouter'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useWebsocket from '../hooks/useWebsocket'
import useFilestore from '../hooks/useFilestore';
import AppLogsTable from '../components/app/AppLogsTable'

import {
  APP_SOURCE_GITHUB,
  IAssistantApi,
  IAssistantGPTScript,
  IAssistantZapier,
  IFileStoreItem,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
} from '../types'

const App: FC = () => {
  const account = useAccount()
  const session = useSession()
  const filestore = useFilestore();
  const {
    params,
  } = useRouter()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  
  // Load the app using the useApp hook
  const {
    // App state
    app,
    isNewApp,
    isLoading,
    hasLoaded,
    knowledgeList,
    knowledgeSources,
    isReadOnly,
    knowledgeErrors,
    showErrors,
    
    // State setters
    setShowErrors,
    
    // Validation methods
    validateApiSchemas,
    validateKnowledge,
    
    // App operations
    handleKnowledgeUpdate,
    
    // Inference methods
    loading,
    inputValue,
    model,
    setInputValue,
    onInference,
    onSave,
    
    // Search
    searchResults,
    onSearch,
    
    // Navigation
    tabValue,
    handleTabChange,
    handleLaunch,
    
    // API keys
    onAddAPIKey,
  } = useApp(params.app_id)

  // UI-specific state that doesn't belong in the hook
  const [isSearchMode, setIsSearchMode] = useState(() => {
    const params = new URLSearchParams(window.location.search);
    return params.get('isSearchMode') === 'true'
  })
  const [showBigSchema, setShowBigSchema] = useState(false)
  const [deletingAPIKey, setDeletingAPIKey] = useState('')
  const [schema, setSchema] = useState('')
  const [editingGptScript, setEditingGptScript] = useState<{
    tool: IAssistantGPTScript;
    index: number;
  } | null>(null)
  const [hasKnowledgeSources, setHasKnowledgeSources] = useState(true)

  // Functions for handling file operations
  const handleLoadFiles = useCallback(async (path: string): Promise<IFileStoreItem[]> =>  {
    try {
      const filesResult = await fetch(`/api/v1/filestore/list?path=${encodeURIComponent(path)}`, {
        headers: {
          'Authorization': `Bearer ${account.token}`
        }
      }).then(r => r.json())
      
      if(filesResult) {
        return filesResult
      }
    } catch(e) {}
    return []
  }, [account.token]);

  // Upload the files to the filestore
  const handleFileUpload = useCallback(async (path: string, files: File[]) => {
    const formData = new FormData()
    files.forEach((file) => {
      formData.append("files", file)
    })
    
    await fetch(`/api/v1/filestore/upload?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${account.token}`
      },
      body: formData
    })
  }, [account.token]);

  const handleRefreshKnowledge = useCallback((id: string) => {
    fetch(`/api/v1/knowledge/${id}/refresh`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${account.token}`
      }
    })
    .then(() => {
      snackbar.success('Knowledge refresh initiated')
    })
    .catch((error) => {
      console.error('Error refreshing knowledge:', error)
      snackbar.error('Failed to refresh knowledge')
    })
  }, [account.token, snackbar]);

  // Updating the schema when app changes
  const getUpdatedSchema = useCallback(() => {
    if (!app) return '';
    
    // Remove empty values and format as YAML
    const removeEmptyValues = (obj: any): any => {
      if (Array.isArray(obj)) {
        const filtered = obj.map(removeEmptyValues).filter(v => v !== undefined && v !== null);
        return filtered.length ? filtered : undefined;
      } else if (typeof obj === 'object' && obj !== null) {
        const filtered = Object.fromEntries(
          Object.entries(obj)
            .map(([k, v]) => [k, removeEmptyValues(v)])
            .filter(([_, v]) => v !== undefined && v !== null && v !== '')
        );
        return Object.keys(filtered).length ? filtered : undefined;
      }
      return obj === '' ? undefined : obj;
    };
    
    // Create a temporary app state with current form values
    const currentConfig = {
      ...app.config.helix,
    };

    // Remove empty values and format as YAML
    let cleanedConfig = removeEmptyValues(currentConfig);
    const configName = cleanedConfig.name;
    delete cleanedConfig.name;
    cleanedConfig = {
      "apiVersion": "app.aispec.org/v1alpha1",
      "kind": "AIApp",
      "metadata": {
        "name": configName
      },
      "spec": cleanedConfig
    };
    return stringifyYaml(cleanedConfig, { indent: 2 });
  }, [app]);

  // Update schema when app changes
  useEffect(() => {
    if (!app) return;
    setSchema(getUpdatedSchema());
  }, [app, getUpdatedSchema]);

  // Check if app has knowledge sources when knowledge list changes
  useEffect(() => {
    if (knowledgeList) {
      setHasKnowledgeSources(knowledgeList.length > 0);
    }
  }, [knowledgeList]);

  // Functions for managing GPT scripts
  const [onSaveGptScript, onDeleteGptScript] = useMemo(() => {
    // Save GPT script function
    const saveScript = (script: IAssistantGPTScript, index?: number) => {
      if (!app) return;
      
      const updatedApp = { ...app };
      const assistants = updatedApp.config.helix.assistants || [];
      
      if (assistants.length > 0) {
        const gptscripts = [...(assistants[0].gptscripts || [])];
        const targetIndex = typeof index === 'number' ? index : gptscripts.length;
        gptscripts[targetIndex] = script;
        
        assistants[0].gptscripts = gptscripts;
        updatedApp.config.helix.assistants = assistants;
      }
      
      setEditingGptScript(null);
    };
    
    // Delete GPT script function
    const deleteScript = (scriptId: string) => {
      if (!app) return;
      
      const updatedApp = { ...app };
      const assistants = updatedApp.config.helix.assistants || [];
      
      if (assistants.length > 0) {
        assistants[0].gptscripts = (assistants[0].gptscripts || [])
          .filter((script) => script.file !== scriptId);
        
        updatedApp.config.helix.assistants = assistants;
      }
    };
    
    return [saveScript, deleteScript];
  }, [app]);

  // Functions for managing API tools
  const [onSaveApiTool, onDeleteApiTool] = useMemo(() => {
    // Save API tool function
    const saveTool = (tool: IAssistantApi, index?: number) => {
      if (!app) return;
      
      const updatedApp = { ...app };
      const assistants = updatedApp.config.helix.assistants || [];
      
      if (assistants.length > 0) {
        const apis = [...(assistants[0].apis || [])];
        const targetIndex = typeof index === 'number' ? index : apis.length;
        apis[targetIndex] = tool;
        
        assistants[0].apis = apis;
        updatedApp.config.helix.assistants = assistants;
      }
    };
    
    // Delete API tool function
    const deleteTool = (toolId: string) => {
      if (!app) return;
      
      const updatedApp = { ...app };
      const assistants = updatedApp.config.helix.assistants || [];
      
      if (assistants.length > 0) {
        assistants[0].apis = (assistants[0].apis || [])
          .filter((api) => api.name !== toolId);
        
        updatedApp.config.helix.assistants = assistants;
      }
    };
    
    return [saveTool, deleteTool];
  }, [app]);

  // Functions for managing Zapier tools
  const [onSaveZapierTool, onDeleteZapierTool] = useMemo(() => {
    // Save Zapier tool function
    const saveTool = (tool: IAssistantZapier, index?: number) => {
      if (!app) return;
      
      const updatedApp = { ...app };
      const assistants = updatedApp.config.helix.assistants || [];
      
      if (assistants.length > 0) {
        const zapier = [...(assistants[0].zapier || [])];
        const targetIndex = typeof index === 'number' ? index : zapier.length;
        zapier[targetIndex] = tool;
        
        assistants[0].zapier = zapier;
        updatedApp.config.helix.assistants = assistants;
      }
    };
    
    // Delete Zapier tool function
    const deleteTool = (toolId: string) => {
      if (!app) return;
      
      const updatedApp = { ...app };
      const assistants = updatedApp.config.helix.assistants || [];
      
      if (assistants.length > 0) {
        assistants[0].zapier = (assistants[0].zapier || [])
          .filter((z) => z.name !== toolId);
        
        updatedApp.config.helix.assistants = assistants;
      }
    };
    
    return [saveTool, deleteTool];
  }, [app]);

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])

  const readOnly = useMemo(() => {
    if(!app) return true
    return false
  }, [
    app,
  ])

  const isGithubApp = useMemo(() => app?.app_source === APP_SOURCE_GITHUB, [app]); 

  const handleCopyEmbedCode = useCallback(() => {
    if (account.apiKeys.length > 0) {
      const embedCode = `<script src="https://cdn.jsdelivr.net/npm/@helixml/chat-embed"></script>
<script>
  ChatWidget({
    url: '${window.location.origin}/v1/chat/completions',
    model: 'llama3:instruct',
    bearerToken: '${account.apiKeys[0].key}',
  })
</script>`
      navigator.clipboard.writeText(embedCode).then(() => {
        snackbar.success('Embed code copied to clipboard');
      }, (err) => {
        console.error('Could not copy text: ', err);
        snackbar.error('Failed to copy embed code');
      });
    } else {
      snackbar.error('No API key available');
    }
  }, [account.apiKeys, snackbar]);

  // Use websocket for session updates
  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      session.setData(parsedData.session)
    }
  })

  // Get data for rendered components
  const assistants = app?.config.helix.assistants || []
  const apiAssistants = assistants.length > 0 ? assistants[0].apis || [] : []
  const zapierAssistants = assistants.length > 0 ? assistants[0].zapier || [] : []
  const gptscriptsAssistants = assistants.length > 0 ? assistants[0].gptscripts || [] : []

  // Don't render until we have necessary data
  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded && params.app_id !== "new") return null
  
  return (
    <Page
      breadcrumbs={[
        {
          title: 'Apps',
          routeName: 'apps'
        },
        {
          title: app.config.helix.name || 'App',
        }
      ]}
      topbarContent={(
        <Box sx={{ textAlign: 'right' }}>
          <Button
            sx={{ mr: 2 }}
            type="button"
            color="primary"
            variant="outlined"
            onClick={handleCopyEmbedCode}
            startIcon={<ContentCopyIcon />}
            disabled={account.apiKeys.length === 0 || isReadOnly}
          >
            Embed
          </Button>
          <Button
            type="button"
            color="secondary"
            variant="contained"
            onClick={handleLaunch}
            disabled={app.id === 'new'}
          >
            Launch
          </Button>
        </Box>
      )}
    >
      <Container maxWidth="xl" sx={{ height: 'calc(100% - 100px)' }}>
        <Box sx={{ height: 'calc(100vh - 100px)', width: '100%', flexGrow: 1, p: 2 }}>
          <Box>
            <Tabs value={tabValue} onChange={handleTabChange}>
              <Tab label="Settings" value="settings" />
              <Tab label="Knowledge" value="knowledge" />
              <Tab label="Integrations" value="integrations" />
              <Tab label="GPTScripts" value="gptscripts" />
              <Tab label="API Keys" value="apikeys" />
              <Tab label="Developers" value="developers" />
              <Tab label="Logs" value="logs" />
            </Tabs>
          </Box>
          <Box sx={{ height: 'calc(100% - 48px)', overflow: 'hidden' }}>
            <Grid container spacing={2} sx={{ height: '100%' }}>
              <Grid item sm={12} md={6} sx={{ 
                borderRight: '1px solid #303047',
                height: '100%',
                overflow: 'auto',
                pb: 8 // Add padding at bottom to prevent content being hidden behind fixed bar
              }}>
                <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                  {tabValue === 'settings' && (
                    <AppSettings
                      name={app.config.helix.name || ''}
                      setName={value => app.config.helix.name = value}
                      description={app.config.helix.description || ''}
                      setDescription={value => app.config.helix.description = value}
                      systemPrompt={assistants[0]?.system_prompt || ''}
                      setSystemPrompt={value => {
                        if (assistants[0]) assistants[0].system_prompt = value
                      }}
                      avatar={app.config.helix.avatar || ''}
                      setAvatar={value => app.config.helix.avatar = value}
                      image={app.config.helix.image || ''}
                      setImage={value => app.config.helix.image = value}
                      shared={app.shared}
                      setShared={value => app.shared = value}
                      global={app.global}
                      setGlobal={value => app.global = value}
                      model={assistants[0]?.model || ''}
                      setModel={value => {
                        if (assistants[0]) assistants[0].model = value
                      }}
                      providerEndpoint={assistants[0]?.provider || ''}
                      setProviderEndpoint={value => {
                        if (assistants[0]) assistants[0].provider = value
                      }}
                      providerEndpoints={account.providerEndpoints}
                      readOnly={readOnly}
                      isReadOnly={isReadOnly}
                      showErrors={showErrors}
                      isAdmin={account.admin}
                    />
                  )}

                  {tabValue === 'knowledge' && (
                    <Box sx={{ mt: 2 }}>
                      <Typography variant="h6" sx={{ mb: 2 }}>
                        Knowledge Sources
                      </Typography>
                      <KnowledgeEditor
                        knowledgeSources={knowledgeSources}
                        onUpdate={handleKnowledgeUpdate}
                        onRefresh={handleRefreshKnowledge}
                        onUpload={handleFileUpload}
                        loadFiles={handleLoadFiles}
                        uploadProgress={filestore.uploadProgress}
                        disabled={isReadOnly}
                        knowledgeList={knowledgeList}
                        appId={app.id}
                      />
                      {knowledgeErrors && showErrors && (
                        <Alert severity="error" sx={{ mt: 2 }}>
                          Please specify at least one URL for each knowledge source.
                        </Alert>
                      )}
                    </Box>
                  )}

                  {tabValue === 'integrations' && (
                    <>
                      <ApiIntegrations
                        apis={apiAssistants}
                        onSaveApiTool={onSaveApiTool}
                        onDeleteApiTool={onDeleteApiTool}
                        isReadOnly={isReadOnly}
                      />

                      <ZapierIntegrations
                        zapier={zapierAssistants}
                        onSaveZapierTool={onSaveZapierTool}
                        onDeleteZapierTool={onDeleteZapierTool}
                        isReadOnly={isReadOnly}
                      />
                    </>
                  )}

                  {tabValue === 'gptscripts' && (
                    <GPTScriptsSection
                      app={app}
                      onAddGptScript={() => {
                        const newScript: IAssistantGPTScript = {
                          name: '',
                          description: '',
                          content: '',
                        };
                        setEditingGptScript({
                          tool: newScript,
                          index: gptscriptsAssistants.length
                        });
                      }}
                      onEdit={(tool, index) => setEditingGptScript({tool, index})}
                      onDeleteGptScript={onDeleteGptScript}
                      isReadOnly={isReadOnly}
                      isGithubApp={isGithubApp}
                    />
                  )}

                  {tabValue === 'apikeys' && (
                    <APIKeysSection
                      apiKeys={account.apiKeys}
                      onAddAPIKey={onAddAPIKey}
                      onDeleteKey={(key) => setDeletingAPIKey(key)}
                      allowedDomains={app.config.allowed_domains || []}
                      setAllowedDomains={domains => app.config.allowed_domains = domains}
                      isReadOnly={isReadOnly}
                      readOnly={readOnly}
                    />
                  )}

                  {tabValue === 'developers' && (
                    <DevelopersSection
                      schema={schema}
                      setSchema={setSchema}
                      showErrors={showErrors}
                      appId={app.id}
                      navigate={useRouter().navigate}
                    />
                  )}

                  {tabValue === 'logs' && (
                    <Box sx={{ mt: 2 }}>
                      <AppLogsTable appId={app.id} />
                    </Box>
                  )}
                </Box>
              </Grid>
              {/* For API keys section show  */}
              {tabValue === 'apikeys' ? (
                <CodeExamples apiKey={account.apiKeys[0]?.key || ''} />
              ) : (
                <PreviewPanel
                loading={loading}
                name={app.config.helix.name || ''}
                avatar={app.config.helix.avatar || ''}
                image={app.config.helix.image || ''}
                isSearchMode={isSearchMode}
                setIsSearchMode={setIsSearchMode}
                inputValue={inputValue}
                setInputValue={setInputValue}
                onInference={onInference}
                onSearch={onSearch}
                hasKnowledgeSources={hasKnowledgeSources}
                searchResults={searchResults}
                session={session.data}
                serverConfig={account.serverConfig}
                themeConfig={themeConfig}
                snackbar={snackbar}
                />
              )}
            </Grid>
          </Box>
        </Box>
      </Container>

      {/* Fixed bottom bar with save button */}
      {tabValue !== 'developers' && tabValue !== 'apikeys' && tabValue !== 'logs' && (
        <Box sx={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          borderTop: '1px solid #303047',
          bgcolor: 'background.paper',
          zIndex: 1000,
        }}>
          <Container maxWidth="xl">
            <Box sx={{ p: 2 }}>
              <Button
                type="button"
                color="secondary"
                variant="contained"
                onClick={() => onSave(false)}
                disabled={isReadOnly && !isGithubApp}
              >
                Save
              </Button>
            </Box>
          </Container>
        </Box>
      )}

      {
        showBigSchema && (
          <Window
            title="Schema"
            fullHeight
            size="lg"
            open
            withCancel
            cancelTitle="Close"
            onCancel={() => setShowBigSchema(false)}
          >
            <Box
              sx={{
                p: 2,
                height: '100%',
              }}
            >
              <TextField
                error={showErrors && !schema}
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                fullWidth
                multiline
                disabled
                label="App Configuration"
                helperText={showErrors && !schema ? "Please enter a schema" : ""}
                sx={{ height: '100%' }} // Set the height to '100%'
              />
            </Box>
          </Window>
        )
      }
      {
        deletingAPIKey && (
          <DeleteConfirmWindow
            title="this API key"
            onSubmit={async () => {
              try {
                await fetch(`/api/v1/api_keys?key=${deletingAPIKey}`, {
                  method: 'DELETE',
                  headers: {
                    'Authorization': `Bearer ${account.token}`
                  }
                })
                
                snackbar.success('API Key deleted')
                account.loadApiKeys({
                  types: 'app',
                  app_id: params.app_id,
                })
                setDeletingAPIKey('')
              } catch (error) {
                console.error('Error deleting API key:', error)
                snackbar.error('Failed to delete API key')
              }
            }}
            onCancel={() => {
              setDeletingAPIKey('')
            }}
          />
        )
      }

      {editingGptScript && (
        <Window
          title={`${editingGptScript.tool.name ? 'Edit' : 'Add'} GPTScript`}
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingGptScript(null)}
          onSubmit={() => {
            if (editingGptScript.tool) {
              onSaveGptScript(editingGptScript.tool, editingGptScript.index);
            }
          }}
        >
          <Box sx={{ p: 2 }}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              GPTScript
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.name}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, name: e.target.value }
                  } : null)}
                  label="Name"
                  fullWidth
                  error={showErrors && !editingGptScript.tool.name}
                  helperText={showErrors && !editingGptScript.tool.name ? 'Please enter a name' : ''}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.description}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, description: e.target.value }
                  } : null)}
                  label="Description"
                  fullWidth
                  error={showErrors && !editingGptScript.tool.description}
                  helperText={showErrors && !editingGptScript.tool.description ? "Description is required" : ""}
                  disabled={isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.content}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, content: e.target.value }
                  } : null)}
                  label="Script Content"
                  fullWidth
                  multiline
                  rows={10}
                  error={showErrors && !editingGptScript.tool.content}
                  helperText={showErrors && !editingGptScript.tool.content ? "Script content is required" : ""}
                  disabled={isReadOnly}
                />
              </Grid>
            </Grid>
          </Box>
        </Window>
      )}
    </Page>
  )
}

export default App
