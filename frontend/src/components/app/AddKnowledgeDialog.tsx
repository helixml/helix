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
} from '@mui/material';
import { IKnowledgeSource } from '../../types';
import { useListOAuthProviders, useListOAuthConnections } from '../../services/oauthProvidersService';
import { TypesOAuthProviderType } from '../../api/api';

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
  const [siteId, setSiteId] = useState('');
  const [driveId, setDriveId] = useState('');
  const [folderPath, setFolderPath] = useState('');
  const [oauthProviderId, setOauthProviderId] = useState('');
  const [filterExtensions, setFilterExtensions] = useState('');
  const [recursive, setRecursive] = useState(true);

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
        setError('SharePoint Site ID is required');
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
    setSiteId('');
    setDriveId('');
    setFolderPath('');
    setOauthProviderId('');
    setFilterExtensions('');
    setRecursive(true);
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

                <TextField
                  fullWidth
                  label="SharePoint Site ID"
                  value={siteId}
                  onChange={(e) => {
                    setSiteId(e.target.value);
                    setError('');
                  }}
                  placeholder="contoso.sharepoint.com,guid,guid"
                  helperText="The SharePoint site ID (format: hostname,site-guid,web-guid)"
                  sx={{ mb: 2 }}
                />

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