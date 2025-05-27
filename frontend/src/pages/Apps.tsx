import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import LockIcon from '@mui/icons-material/Lock'

import Page from '../components/system/Page'
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
  IAppConfig,
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
      if (params.create === 'true' && params.template_type && params.provider_name && params.oauth === 'true') {
        const timeoutId = setTimeout(() => {
          snackbar.info('App creation is taking longer than expected...');
        }, 3000);
        
        try {
          await createOAuthApp(params.template_type, params.provider_name);
          // Clear the timeout if createOAuthApp completes successfully
          clearTimeout(timeoutId);
        } catch (err) {
          clearTimeout(timeoutId);
          console.error('Error during OAuth app creation:', err);
          snackbar.error('Failed to create app with OAuth integration');
        }
        
        // Clean up URL parameters regardless of outcome
        removeParams(['create', 'template_type', 'provider_name', 'oauth']);
      }
    };
    
    handleOAuthAppCreation();
  }, [params.create, params.template_type, params.provider_name, params.oauth]);

  const createOAuthApp = async (templateType: string, providerName: string) => {
    try {
      console.log('Creating OAuth app with template type:', templateType, 'provider name:', providerName);
      
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
        // Use the template type directly
        if (templateType) {
          const response = await api.getApiClient().v1TemplateAppsDetail(templateType);
          templateConfig = response.data;
          console.log('Loaded template configuration:', templateConfig);
        }
      } catch (error) {
        console.error('Could not load template from API', error);
        snackbar.error('Failed to load template configuration');
        return;
      }
      
      // Template configuration is required
      if (!templateConfig) {
        console.error('No template configuration available');
        snackbar.error('No template configuration available');
        return;
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
      
      // Use the template configuration
      const appName = getUniqueAppName(templateConfig.name || '');
      const appDescription = templateConfig.description || '';
      
      // Create API tool configuration
      const toolName = `${provider.name} API`;
      
      // Get the API URL from the template
      const apiUrl = templateConfig.api_url || '';
      
      // Log the API URL for debugging
      console.log('Provider API URL from template:', apiUrl);
      
      // Validation check - API URL is required
      if (!apiUrl) {
        console.error('API URL is required but not available in template config');
        snackbar.error('API URL is required for API tools');
        return;
      }
      
      // Get schema from template
      let schema = '';
      
      // If we have template assistants and APIs, use that schema
      if (templateConfig.assistants && 
          templateConfig.assistants.length > 0 && 
          templateConfig.assistants[0].apis && 
          templateConfig.assistants[0].apis.length > 0 &&
          templateConfig.assistants[0].apis[0].schema) {
        schema = templateConfig.assistants[0].apis[0].schema || '';
        console.log('Using schema from template config');
      } else {
        console.error('No schema available in template configuration');
        snackbar.error('No API schema available');
        return;
      }
      
      console.log(`Schema from template:`, schema.length > 100 ? schema.substring(0, 100) + '...' : schema);
      
      // Prepare app configuration
      const appConfig: IAppConfig = {
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
              schema: schema,
              oauth_provider: provider.name,
              oauth_scopes: provider.default_scopes || []
            }],
            gptscripts: [],
            tools: [],
            rag_source_id: '',
            lora_id: '',
            is_actionable_template: ''
          }]
        },
        secrets: {},
        allowed_domains: []
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
      removeParams(['create', 'template_type', 'provider_name', 'oauth']);
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
      removeParams(['create', 'template_type', 'provider_name', 'oauth']);
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
