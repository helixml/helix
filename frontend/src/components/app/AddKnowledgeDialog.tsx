import React, { useState } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  RadioGroup,
  FormControlLabel,
  Radio,
  TextField,
  FormControl,
  CircularProgress,
  InputLabel,
  Select,
  MenuItem,
  FormHelperText,
  Checkbox,
  Box,
  Alert,
  Typography,
} from '@mui/material';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import { IKnowledgeSource } from '../../types';
import { useListOAuthProviders, useListOAuthConnections } from '../../services/oauthProvidersService';
import { TypesOAuthProviderType } from '../../api/api';
import useApi from '../../hooks/useApi';

interface AddKnowledgeDialogProps {
  open: boolean;
  onClose: () => void;
  onAdd: (source: IKnowledgeSource) => void;
  appId: string;
}

const AddKnowledgeDialog: React.FC<AddKnowledgeDialogProps> = ({
  open,
  onClose,
  onAdd,
  appId,
}) => {
  const [sourceType, setSourceType] = useState<'web' | 'filestore' | 'text' | 'sharepoint'>('web');
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [plainText, setPlainText] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  // SharePoint specific state
  const [siteUrl, setSiteUrl] = useState('');
  const [siteId, setSiteId] = useState('');
  const [siteName, setSiteName] = useState('');
  const [driveId, setDriveId] = useState('');
  const [folderPath, setFolderPath] = useState('');
  const [oauthProviderId, setOauthProviderId] = useState('');
  const [filterExtensions, setFilterExtensions] = useState('');
  const [recursive, setRecursive] = useState(true);
  const [isLookingUp, setIsLookingUp] = useState(false);
  const [lookupError, setLookupError] = useState('');

  // API client for SharePoint lookup
  const api = useApi();

  // Fetch OAuth providers and connections for SharePoint
  const { data: oauthProviders } = useListOAuthProviders();
  const { data: oauthConnections } = useListOAuthConnections();

  // Filter to only Microsoft providers (by type or by URL pattern)
  const microsoftProviders = oauthProviders?.filter(
    p => p.type === TypesOAuthProviderType.OAuthProviderTypeMicrosoft ||
         p.auth_url?.includes('login.microsoftonline.com') ||
         p.token_url?.includes('login.microsoftonline.com')
  ) || [];

  // Check if user has a connection for the selected provider
  const hasConnectionForProvider = (providerId: string) => {
    return oauthConnections?.some(c => c.provider?.id === providerId);
  };

  // Validate SharePoint URL format
  const isValidSharePointUrl = (url: string) => {
    return url.includes('sharepoint.com');
  };

  // Lookup SharePoint site ID from URL
  const handleLookupSite = async () => {
    if (!siteUrl.trim()) {
      setLookupError('Please enter a SharePoint site URL');
      return;
    }

    if (!isValidSharePointUrl(siteUrl)) {
      setLookupError('Invalid URL. Must be a SharePoint URL (e.g., https://contoso.sharepoint.com/sites/MySite)');
      return;
    }

    if (!oauthProviderId) {
      setLookupError('Please select a Microsoft OAuth provider first');
      return;
    }

    if (!hasConnectionForProvider(oauthProviderId)) {
      setLookupError('You need to connect to this OAuth provider first');
      return;
    }

    setIsLookingUp(true);
    setLookupError('');
    setSiteId('');
    setSiteName('');

    try {
      const response = await api.getApiClient().v1OauthSharepointResolveSiteCreate({
        site_url: siteUrl,
        provider_id: oauthProviderId,
      });

      if (response.data.site_id) {
        setSiteId(response.data.site_id);
        setSiteName(response.data.display_name || '');
        setError('');
      } else {
        setLookupError('Could not resolve site ID from the provided URL');
      }
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to lookup SharePoint site';
      setLookupError(errorMessage);
    } finally {
      setIsLookingUp(false);
    }
  };

  const handleSubmit = () => {
    if (!name.trim()) {
      setError('Name is required');
      return;
    }

    if (sourceType === 'web' && !url.trim()) {
      setError('URL is required for web sources');
      return;
    }

    if (sourceType === 'text' && !plainText.trim()) {
      setError('Text content is required');
      return;
    }

    if (sourceType === 'sharepoint') {
      if (!siteId.trim()) {
        setError('Please lookup the SharePoint site first using the "Lookup Site" button');
        return;
      }
      if (!oauthProviderId) {
        setError('Please select a Microsoft OAuth provider');
        return;
      }
      if (!hasConnectionForProvider(oauthProviderId)) {
        setError('You need to connect to this OAuth provider first');
        return;
      }
    }

    setIsLoading(true);

    const knowledgePath = sourceType === 'filestore' ? name : name;

    let source: IKnowledgeSource['source'];
    if (sourceType === 'filestore') {
      source = { filestore: { path: knowledgePath } };
    } else if (sourceType === 'text') {
      source = { text: plainText };
    } else if (sourceType === 'sharepoint') {
      source = {
        sharepoint: {
          site_id: siteId,
          drive_id: driveId || undefined,
          folder_path: folderPath || undefined,
          oauth_provider_id: oauthProviderId,
          filter_extensions: filterExtensions ? filterExtensions.split(',').map(ext => ext.trim()) : undefined,
          recursive: recursive,
        },
      };
    } else {
      source = {
        web: {
          urls: [url],
          crawler: {
            enabled: true,
            max_depth: 1,
            max_pages: 5,
            readability: true
          }
        }
      };
    }

    const newSource: IKnowledgeSource = {
      id: '',
      name: name,
      source,
      refresh_schedule: '',
      version: '',
      state: '',
      rag_settings: {
        results_count: 0,
        chunk_size: 0,
        chunk_overflow: 0,
        enable_vision: false,
      },
    };

    onAdd(newSource);

    // Adding a small delay to show the loading indicator
    // The parent component should handle closing this dialog after processing is complete
    setTimeout(() => {
      setIsLoading(false);
      handleClose();
    }, 500);
  };

  const handleClose = () => {
    setName('');
    setUrl('');
    setPlainText('');
    setError('');
    setSourceType('web');
    setIsLoading(false);
    // Reset SharePoint fields
    setSiteUrl('');
    setSiteId('');
    setSiteName('');
    setDriveId('');
    setFolderPath('');
    setOauthProviderId('');
    setFilterExtensions('');
    setRecursive(true);
    setLookupError('');
    onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Add Knowledge Source</DialogTitle>
      <DialogContent>
        <FormControl component="fieldset" sx={{ mt: 2, mb: 2 }}>
          <RadioGroup
            row
            value={sourceType}
            onChange={(e) => setSourceType(e.target.value as 'web' | 'filestore' | 'text' | 'sharepoint')}
          >
            <FormControlLabel value="web" control={<Radio />} label="Web" />
            <FormControlLabel value="filestore" control={<Radio />} label="Files" />
            <FormControlLabel value="text" control={<Radio />} label="Plain Text" />
            <FormControlLabel
              value="sharepoint"
              control={<Radio />}
              label="SharePoint"
            />
          </RadioGroup>
        </FormControl>

        <TextField
          fullWidth
          label="Knowledge name"
          value={name}
          onChange={(e) => {
            setName(e.target.value);
            setError('');
          }}
          error={!!error}
          helperText={error || (sourceType === 'filestore' ? `Files will be uploaded to the '${name}' folder in this app` : '')}
          sx={{ mb: 2 }}
        />

        {sourceType === 'web' && (
          <TextField
            fullWidth
            label="URLs (comma-separated)"
            value={url}
            onChange={(e) => {
              setUrl(e.target.value);
              setError('');
            }}
            error={!!error && !url.trim()}
            helperText={error && !url.trim() ? 'URL is required' : ''}
            sx={{ mb: 2 }}
          />
        )}

        {sourceType === 'text' && (
          <TextField
            fullWidth
            multiline
            rows={6}
            label="Your raw text such as markdown, html, etc."
            value={plainText}
            onChange={(e) => {
              setPlainText(e.target.value);
              setError('');
            }}
            error={!!error && !plainText.trim()}
            helperText={error && !plainText.trim() ? 'Text content is required' : ''}
            sx={{ mb: 2 }}
          />
        )}

        {sourceType === 'sharepoint' && (
          <>
            {microsoftProviders.length === 0 ? (
              <FormHelperText error sx={{ mb: 2 }}>
                SharePoint requires a Microsoft OAuth provider. Configure one in Admin Panel → OAuth Providers.
              </FormHelperText>
            ) : (
              <>
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel>Microsoft OAuth Provider</InputLabel>
                  <Select
                    value={oauthProviderId}
                    onChange={(e) => {
                      setOauthProviderId(e.target.value);
                      setError('');
                    }}
                    label="Microsoft OAuth Provider"
                  >
                    {microsoftProviders.map((provider) => (
                      <MenuItem key={provider.id} value={provider.id}>
                        {provider.name}
                        {!hasConnectionForProvider(provider.id || '') && ' (not connected)'}
                      </MenuItem>
                    ))}
                  </Select>
                  {oauthProviderId && !hasConnectionForProvider(oauthProviderId) && (
                    <FormHelperText error>
                      You need to connect your Microsoft account first. Go to Admin Panel → OAuth Connections.
                    </FormHelperText>
                  )}
                </FormControl>

                <Box sx={{ mb: 2 }}>
                  <TextField
                    fullWidth
                    label="SharePoint Site URL"
                    value={siteUrl}
                    onChange={(e) => {
                      setSiteUrl(e.target.value);
                      setSiteId('');
                      setSiteName('');
                      setLookupError('');
                      setError('');
                    }}
                    placeholder="https://contoso.sharepoint.com/sites/MySite"
                    helperText="Enter the full URL of your SharePoint site (e.g., https://yourcompany.sharepoint.com/sites/TeamSite)"
                    sx={{ mb: 1 }}
                  />
                  <Button
                    variant="outlined"
                    onClick={handleLookupSite}
                    disabled={isLookingUp || !siteUrl.trim() || !oauthProviderId || !hasConnectionForProvider(oauthProviderId)}
                    startIcon={isLookingUp ? <CircularProgress size={16} /> : null}
                    size="small"
                  >
                    {isLookingUp ? 'Looking up...' : 'Lookup Site'}
                  </Button>
                </Box>

                {lookupError && (
                  <Alert severity="error" sx={{ mb: 2 }}>
                    {lookupError}
                  </Alert>
                )}

                {siteId && (
                  <Alert
                    severity="success"
                    icon={<CheckCircleIcon />}
                    sx={{ mb: 2 }}
                  >
                    <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>
                      Site found: {siteName || 'SharePoint Site'}
                    </Typography>
                    <Typography variant="caption" sx={{ wordBreak: 'break-all' }}>
                      Site ID: {siteId}
                    </Typography>
                  </Alert>
                )}

                <TextField
                  fullWidth
                  label="Drive ID (optional)"
                  value={driveId}
                  onChange={(e) => setDriveId(e.target.value)}
                  placeholder="Leave empty for default document library"
                  helperText="Specific document library ID. Leave empty to use the site's default drive."
                  sx={{ mb: 2 }}
                />

                <TextField
                  fullWidth
                  label="Folder Path (optional)"
                  value={folderPath}
                  onChange={(e) => setFolderPath(e.target.value)}
                  placeholder="/Documents/Reports"
                  helperText="Path to a specific folder within the drive. Leave empty for root."
                  sx={{ mb: 2 }}
                />

                <TextField
                  fullWidth
                  label="Filter Extensions (optional)"
                  value={filterExtensions}
                  onChange={(e) => setFilterExtensions(e.target.value)}
                  placeholder=".pdf, .docx, .txt"
                  helperText="Comma-separated list of file extensions to include (e.g., .pdf, .docx)"
                  sx={{ mb: 2 }}
                />

                <Box sx={{ mb: 2 }}>
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={recursive}
                        onChange={(e) => setRecursive(e.target.checked)}
                      />
                    }
                    label="Include files in subfolders"
                  />
                </Box>
              </>
            )}
          </>
        )}

      </DialogContent>
      <DialogActions sx={{ display: 'flex', justifyContent: 'space-between' }}>
        <div>
          <Button sx={{ ml:2 }} onClick={handleClose} disabled={isLoading}>Cancel</Button>
        </div>
        <div>
          <Button
            sx={{ mr:2 }}
            onClick={handleSubmit}
            variant="outlined"
            color="secondary"
            disabled={isLoading || (sourceType === 'sharepoint' && microsoftProviders.length === 0)}
            startIcon={isLoading ? <CircularProgress size={20} /> : null}
          >
            {isLoading ? 'Adding...' : 'Add'}
          </Button>
        </div>
      </DialogActions>
    </Dialog>
  );
};

export default AddKnowledgeDialog; 