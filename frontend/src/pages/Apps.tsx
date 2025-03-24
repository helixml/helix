import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import LockIcon from '@mui/icons-material/Lock'

import Page from '../components/system/Page'
import CreateAppWindow from '../components/apps/CreateAppWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'

import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useApi from '../hooks/useApi'

import {
  IApp,
  SESSION_TYPE_TEXT,
} from '../types'

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const api = useApi()
  const {
    params,
    setParams,
    removeParams,
    navigate,
  } = useRouter()

  const [ deletingApp, setDeletingApp ] = useState<IApp>()

  useEffect(() => {
    const handleOAuthAppCreation = async () => {
      if (params.create === 'true' && params.template && params.provider_name && params.oauth === 'true') {
        const timeoutId = setTimeout(() => {
          snackbar.info('App creation is taking longer than expected...');
        }, 3000);
        
        try {
          await createOAuthApp(params.template, params.provider_name);
          // Clear the timeout if createOAuthApp completes successfully
          clearTimeout(timeoutId);
        } catch (err) {
          clearTimeout(timeoutId);
          console.error('Error during OAuth app creation:', err);
          snackbar.error('Failed to create app with OAuth integration');
        }
        
        // Clean up URL parameters regardless of outcome
        removeParams(['create', 'template', 'provider_name', 'oauth']);
      }
    };
    
    handleOAuthAppCreation();
  }, [params.create, params.template, params.provider_name, params.oauth]);

  const createOAuthApp = async (templateId: string, providerName: string) => {
    try {
      console.log('Creating OAuth app with template:', templateId, 'provider name:', providerName);
      
      // Add loading state indication
      snackbar.success('Initializing app creation...');
      
      // Fetch provider details with error checking - look up by name
      const providersResponse = await api.get('/api/v1/oauth/providers');
      if (!Array.isArray(providersResponse)) {
        console.error('Failed to load OAuth providers');
        snackbar.error('Could not load OAuth providers');
        return;
      }
      
      const provider = providersResponse.find((p: any) => p.name === providerName);
      if (!provider) {
        console.error('Failed to find OAuth provider with name:', providerName);
        snackbar.error('Could not find OAuth provider');
        return;
      }
      
      console.log('Loaded provider details:', provider);
      
      // Get the template app configuration
      let templateConfig;
      try {
        // Extract the template type from the templateId
        const templateType = templateId.includes('github') ? 'github' :
                             templateId.includes('jira') ? 'jira' :
                             templateId.includes('slack') ? 'slack' :
                             templateId.includes('google') ? 'google' : '';
        
        if (templateType) {
          templateConfig = await api.get(`/api/v1/template_apps/${templateType}`);
          console.log('Loaded template configuration:', templateConfig);
        }
      } catch (error) {
        console.warn('Could not load template from API, using fallback configuration', error);
        // We'll continue with fallback configuration
      }
      
      // Ensure we have a default model
      const defaultModel = account.models && account.models.length > 0 
        ? account.models[0].id 
        : '';
      
      if (!defaultModel) {
        console.warn('No default model available');
      }
      
      // Set app name and description based on template
      const getUniqueAppName = (baseName: string) => {
        // Add a timestamp to ensure uniqueness
        const timestamp = new Date().toISOString().slice(11, 19).replace(/:/g, '');
        return `${baseName} ${timestamp}`;
      };
      
      let baseAppName, appDescription;
      
      if (templateConfig) {
        // Use the template configuration if available
        baseAppName = templateConfig.name;
        appDescription = templateConfig.description;
      } else {
        // Fallback to hardcoded values
        baseAppName = templateId.includes('github') ? 'GitHub Repository Analyzer' : 
                      templateId.includes('jira') ? 'Jira Project Manager' :
                      templateId.includes('slack') ? 'Slack Channel Assistant' :
                      templateId.includes('google') ? 'Google Drive Navigator' :
                      `${provider.name} Assistant`;
                      
        appDescription = templateId.includes('github') ? 'Analyze GitHub repositories, issues, and PRs' : 
                        templateId.includes('jira') ? 'Manage and analyze Jira projects and issues' :
                        templateId.includes('slack') ? 'Answer questions and perform tasks in Slack channels' :
                        templateId.includes('google') ? 'Search and summarize documents in Google Drive' :
                        `AI assistant that connects to your ${provider.name} account`;
      }
      
      const appName = getUniqueAppName(baseAppName);
      
      // Create API tool configuration
      const toolName = `${provider.name} API`;
      
      // Determine API URL based on provider type
      const getProviderApiUrl = (provider: any): string => {
        // Use provider.api_url if available
        if (provider.api_url) {
          return provider.api_url;
        }
        
        // Default API URLs for known provider types
        switch (provider.type) {
          case 'github':
            return 'https://api.github.com';
          case 'slack':
            return 'https://slack.com/api';
          case 'google':
            return 'https://www.googleapis.com';
          case 'jira':
          case 'atlassian':
            return 'https://api.atlassian.com';
          case 'microsoft':
            return 'https://graph.microsoft.com/v1.0';
          default:
            console.warn(`No default API URL for provider type: ${provider.type}`);
            return '';
        }
      };
      
      // Get the API URL
      const apiUrl = getProviderApiUrl(provider);
      
      // Log the API URL for debugging
      console.log('Provider API URL:', apiUrl);
      
      // Validation check - API URL is required
      if (!apiUrl) {
        console.error('API URL is required but not available for provider:', provider);
        snackbar.error('API URL is required for API tools');
        return;
      }
      
      // Get an appropriate API schema for this provider
      const getProviderSchema = (provider: any): string => {
        // Try to use schema from provider if available
        if (provider.api_schema) {
          return provider.api_schema;
        }
        
        // Generate default schema for known providers
        switch (provider.type) {
          case 'github':
            return JSON.stringify({
              openapi: "3.0.0",
              info: {
                title: "GitHub API",
                version: "1.0.0",
                description: "API for GitHub"
              },
              paths: {
                "/users/{username}": {
                  get: {
                    operationId: "getUser",
                    summary: "Get a user",
                    parameters: [
                      {
                        name: "username",
                        in: "path",
                        required: true,
                        schema: { type: "string" }
                      }
                    ]
                  }
                },
                "/repos/{owner}/{repo}": {
                  get: {
                    operationId: "getRepo",
                    summary: "Get a repository",
                    parameters: [
                      {
                        name: "owner",
                        in: "path",
                        required: true,
                        schema: { type: "string" }
                      },
                      {
                        name: "repo",
                        in: "path",
                        required: true,
                        schema: { type: "string" }
                      }
                    ]
                  }
                }
              }
            });
          case 'slack':
            return JSON.stringify({
              openapi: "3.0.0",
              info: {
                title: "Slack API",
                version: "1.0.0",
                description: "API for Slack"
              },
              paths: {
                "/chat.postMessage": {
                  post: {
                    operationId: "postMessage",
                    summary: "Post a message to a channel"
                  }
                }
              }
            });
          case 'google':
            return JSON.stringify({
              openapi: "3.0.0",
              info: {
                title: "Google Drive API",
                version: "1.0.0",
                description: "API for Google Drive"
              },
              paths: {
                "/files": {
                  get: {
                    operationId: "listFiles",
                    summary: "List files"
                  }
                }
              }
            });
          default:
            // Return a minimal schema for unknown provider types
            return JSON.stringify({
              openapi: "3.0.0",
              info: {
                title: `${provider.name} API`,
                version: "1.0.0",
                description: `API for ${provider.name}`
              },
              paths: {}
            });
        }
      };
      
      // Get the schema for this provider
      const schema = getProviderSchema(provider);
      console.log(`Using schema for provider type ${provider.type}:`, schema.length > 100 ? schema.substring(0, 100) + '...' : schema);
      
      // Prepare app configuration
      const appConfig = {
        helix: {
          external_url: '',
          name: appName,
          description: appDescription,
          avatar: '',
          image: '',
          assistants: [{
            name: 'Default Assistant',
            description: appDescription,
            avatar: '',
            image: '',
            model: defaultModel,
            type: SESSION_TYPE_TEXT,
            system_prompt: `You are an AI assistant that connects to ${provider.name}. You can help users access their data and perform actions.`,
            apis: [{
              name: toolName,
              description: `Access ${provider.name} data and functionality`,
              url: apiUrl,
              schema: schema, // <-- Using the schema we generated
              oauth_provider: provider.name, // Use provider name instead of type
              oauth_scopes: provider.default_scopes || []
            }],
            gptscripts: [],
            tools: [],
            rag_source_id: '',
            lora_id: '',
            is_actionable_template: '',
          }],
        },
        secrets: {},
        allowed_domains: [],
      };
      
      console.log('Creating app with config:', JSON.stringify(appConfig, null, 2));
      
      // Create the app
      const newApp = await apps.createApp('helix', appConfig);
      
      if (!newApp) {
        console.error('App creation failed - no app returned from API');
        snackbar.error('Failed to create app');
        return;
      }
      
      console.log('Successfully created app:', newApp);
      
      // Clean up URL params and navigate to the new app
      removeParams(['create', 'template', 'provider_name', 'oauth']);
      navigate('app', { app_id: newApp.id });
      snackbar.success(`Created new ${provider.name} app`);
    } catch (err) {
      console.error('Error creating OAuth app:', err);
      
      // Extract and display more specific error information
      let errorMessage = 'Failed to create app with OAuth integration';
      
      // Check for axios error response
      if (err && (err as any).response && (err as any).response.data) {
        const responseData = (err as any).response.data;
        if (responseData.error) {
          errorMessage += `: ${responseData.error}`;
        } else if (typeof responseData === 'string') {
          errorMessage += `: ${responseData}`;
        }
      } else if (err instanceof Error) {
        errorMessage += `: ${err.message}`;
      }
      
      snackbar.error(errorMessage);
      
      // Clean up URL params even on error
      removeParams(['create', 'template', 'provider_name', 'oauth']);
    }
  };

  const onConnectRepo = useCallback(async (repo: string) => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    const newApp = await apps.createGithubApp(repo)
    if(!newApp) return false
    removeParams(['add_app'])
    snackbar.success('app created')
    apps.loadApps()
    navigate('app', {
      app_id: newApp.id,
    })
    return true
  }, [
    apps.createApp,
  ])

  const onEditApp = (app: IApp) => {
    account.orgNavigate('app', {
      app_id: app.id,
    })
  }

  const onDeleteApp = useCallback(async () => {
    if(!deletingApp) return
    const result = await apps.deleteApp(deletingApp.id)
    if(!result) return
    setDeletingApp(undefined)
    apps.loadApps()
    snackbar.success('app deleted')
  }, [
    deletingApp,
    apps.deleteApp,
  ])

  useEffect(() => {
    if(!account.user) return
    if(!params.add_app) return
    apps.loadGithubStatus(`${window.location.href}?add_app=true`)
  }, [
    account.user,
    params.add_app,
  ])

  useEffect(() => {
    if(!apps.githubStatus) return
    apps.loadGithubRepos()
  }, [
    apps.githubStatus,
  ])

  useEffect(() => {
    if(!params.snackbar_message) return
    snackbar.success(params.snackbar_message)
  }, [
    params.snackbar_message,
  ])

  useEffect(() => {
    apps.loadApps()
  }, [
    apps.loadApps,
  ])

  return (
    <Page
      breadcrumbTitle="Apps"
      orgBreadcrumbs={ true }
      topbarContent={(
        <div>
          <Button
            id="secrets-button"
            variant="contained"
            color="secondary"
            endIcon={<LockIcon />}
            onClick={() => navigate('secrets')}
            sx={{ mr: 2 }}
          >
            Secrets
          </Button>

          <Button
            id="new-app-button"
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            onClick={apps.createOrgApp}
            sx={{ mr: 2 }}
          >
            New App
          </Button>
          <Button
            id="connect-repo-button"
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            onClick={ () => {
              if(!account.user) {
                account.setShowLoginWindow(true)
                return false
              }
              setParams({add_app: 'true'})
            }}
          >
            Connect Repo
          </Button>
        </div>
      )}
    >
      <Container
        maxWidth="xl"
        sx={{
          mb: 4,
        }}
      >
        <AppsTable
          data={ apps.apps }
          onEdit={ onEditApp }
          onDelete={ setDeletingApp }
        />
      </Container>
      {
        params.add_app && apps.githubStatus && (
          <CreateAppWindow
            githubStatus={ apps.githubStatus }
            githubRepos={ apps.githubRepos}
            githubReposLoading={ apps.githubReposLoading }
            onConnectRepo={ onConnectRepo }
            onCancel={ () => removeParams(['add_app']) }
            onLoadRepos={ apps.loadGithubRepos }
            connectLoading= { apps.connectLoading }
            connectError= { apps.connectError }
          />
        )
      }
      {
        deletingApp && (
          <DeleteConfirmWindow
            title="this app"
            onCancel={ () => setDeletingApp(undefined) }
            onSubmit={ onDeleteApp }
          />
        )
      }
    </Page>
  )
}

export default Apps
