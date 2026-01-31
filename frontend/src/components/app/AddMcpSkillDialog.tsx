import React, { useState, useEffect } from 'react';
import { AxiosError } from 'axios';
import {
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  IconButton,
  List,
  ListItem,
  Grid,
  Alert,
  Tooltip,
  CircularProgress,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Chip,
  Tabs,
  Tab,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Avatar,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import SettingsIcon from '@mui/icons-material/Settings';
import { IAppFlatState, ITool } from '../../types';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';
import useApi from '../../hooks/useApi';
import { TypesToolMCPClientConfig, McpTool, TypesOAuthProvider } from '../../api/api';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';
import { useListOAuthProviders } from '../../services/oauthProvidersService';

interface AddMcpSkillDialogProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  skill?: {
    name: string;
    url: string;
    headers?: Record<string, string>;
    oauth_provider?: string;
    oauth_scopes?: string[];
  };
  app: IAppFlatState;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

const NameTypography = styled(Typography)(({ theme }) => ({
  fontSize: '2rem',
  fontWeight: 700,
  color: '#F8FAFC',
  marginBottom: theme.spacing(1),
}));



const SectionCard = styled(Box)(({ theme }) => ({
  background: '#23262F',
  borderRadius: 12,
  padding: theme.spacing(3),
  marginBottom: theme.spacing(3),
  boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
}));

const DarkTextField = styled(TextField)(({ theme }) => ({
  '& .MuiInputBase-root': {
    background: '#23262F',
    color: '#F1F1F1',
    borderRadius: 8,
  },
  '& .MuiInputLabel-root': {
    color: '#A0AEC0',
  },
  '& .MuiOutlinedInput-notchedOutline': {
    borderColor: '#353945',
  },
  '&:hover .MuiOutlinedInput-notchedOutline': {
    borderColor: '#6366F1',
  },
}));

const DarkButton = styled(Button)(({ theme }) => ({
  background: '#353945',
  color: '#F1F1F1',
  borderRadius: 8,
  '&:hover': {
    background: '#6366F1',
    color: '#fff',
  },
}));

const AddMcpSkillDialog: React.FC<AddMcpSkillDialogProps> = ({
  open,
  onClose,
  onClosed,
  skill: initialSkill,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const api = useApi();

  const [skill, setSkill] = useState({
    name: '',
    url: '',
    headers: {} as Record<string, string>,
    oauth_provider: '',
    oauth_scopes: [] as string[],
  });

  const [existingSkill, setExistingSkill] = useState<typeof skill | null>(null);
  const [existingSkillIndex, setExistingSkillIndex] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [urlError, setUrlError] = useState<string | null>(null);
  const [validating, setValidating] = useState(false);
  const [validationResult, setValidationResult] = useState<TypesToolMCPClientConfig | null>(null);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState(0);
  const [parsedMcpTools, setParsedMcpTools] = useState<McpTool[]>([]);
  const [existingTool, setExistingTool] = useState<ITool | null>(null);

  // OAuth state
  const [oauthProvider, setOAuthProvider] = useState<string>('');
  const [oauthScopes, setOAuthScopes] = useState<string[]>([]);

  // Fetch oauth providers
  const { data: allOAuthProviders = [], isLoading: isLoadingOAuthProviders, error: oauthProvidersError } = useListOAuthProviders();

  // Set OAuth provider when providers finish loading and skill has an oauth_provider
  useEffect(() => {
    if (!isLoadingOAuthProviders && skill.oauth_provider && skill.oauth_provider !== oauthProvider) {
      setOAuthProvider(skill.oauth_provider);
    }
  }, [isLoadingOAuthProviders, skill.oauth_provider, oauthProvider]);

  useEffect(() => {
    if (initialSkill) {
      setSkill({
        name: initialSkill.name,
        url: initialSkill.url,
        headers: initialSkill.headers ?? {},
        oauth_provider: initialSkill.oauth_provider ?? '',
        oauth_scopes: initialSkill.oauth_scopes ?? [],
      });

      // Set OAuth state
      setOAuthProvider(initialSkill.oauth_provider ?? '');
      setOAuthScopes(initialSkill.oauth_scopes ?? []);

      // Find existing skill in app.mcpTools
      const existingIndex = app.mcpTools?.findIndex(mcp => mcp.name === initialSkill.name) ?? -1;
      if (existingIndex !== -1) {
        setExistingSkill({
          name: initialSkill.name,
          url: initialSkill.url,
          headers: initialSkill.headers ?? {},
          oauth_provider: initialSkill.oauth_provider ?? '',
          oauth_scopes: initialSkill.oauth_scopes ?? [],
        });
        setExistingSkillIndex(existingIndex);
      }

      const existingTool = app.tools?.find(tool => tool.name === initialSkill.name);
      if (existingTool) {
        setExistingTool(existingTool);
      }
    } else {
      // Reset form when opening for new skill
      setSkill({
        name: '',
        url: '',
        headers: {},
        oauth_provider: '',
        oauth_scopes: [],
      });
      setExistingSkill(null);
      setExistingSkillIndex(null);
      setExistingTool(null);
      setOAuthProvider('');
      setOAuthScopes([]);
    }
    // Reset validation state when dialog opens
    setValidationResult(null);
    setValidationError(null);
  }, [initialSkill, open, app.mcpTools]);

  useEffect(() => {
    if (existingTool) {
      if (existingTool.tool_type === 'mcp') {
        setParsedMcpTools(existingTool.config.mcp?.tools || []);
      }
    }
  }, [existingTool, existingTool?.config.mcp?.tools]);

  const handleChange = (field: string, value: string) => {
    setSkill((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleUrlChange = (value: string) => {
    if (value && !value.toLowerCase().startsWith('http')) {
      setUrlError('URL must start with http:// or https://');
    } else {
      setUrlError(null);
    }

    setSkill((prev) => ({
      ...prev,
      url: value,
    }));
  };

  const handleHeaderChange = (key: string, value: string) => {
    const newHeaders = { ...skill.headers };
    if (value === '') {
      delete newHeaders[key];
    } else {
      newHeaders[key] = value;
    }
    setSkill((prev) => ({
      ...prev,
      headers: newHeaders,
    }));
  };

  const addHeader = () => {
    const newHeaders = { ...skill.headers, '': '' };
    setSkill((prev) => ({
      ...prev,
      headers: newHeaders,
    }));
  };

  const removeHeader = (key: string) => {
    const newHeaders = { ...skill.headers };
    delete newHeaders[key];
    setSkill((prev) => ({
      ...prev,
      headers: newHeaders,
    }));
  };

  const handleOAuthProviderChange = (provider: string) => {
    setOAuthProvider(provider);
    setSkill((prev) => ({
      ...prev,
      oauth_provider: provider,
    }));
  };

  const addScope = () => {
    const newScopes = [...oauthScopes, ''];
    setOAuthScopes(newScopes);
    setSkill((prev) => ({
      ...prev,
      oauth_scopes: newScopes,
    }));
  };

  const removeScope = (index: number) => {
    const newScopes = [...oauthScopes];
    newScopes.splice(index, 1);
    setOAuthScopes(newScopes);
    setSkill((prev) => ({
      ...prev,
      oauth_scopes: newScopes,
    }));
  };

  const handleScopeChange = (index: number, value: string) => {
    const newScopes = [...oauthScopes];
    newScopes[index] = value;
    setOAuthScopes(newScopes);
    setSkill((prev) => ({
      ...prev,
      oauth_scopes: newScopes,
    }));
  };

  const handleValidate = async () => {
    try {
      setValidating(true);
      setValidationError(null);
      setValidationResult(null);

      // Validate URL before making the API call
      if (!skill.url.toLowerCase().startsWith('http')) {
        setUrlError('URL must start with http:// or https://');
        setValidating(false);
        return;
      }

      // Call the validation API
      const result = await api.getApiClient().v1SkillsValidateCreate({
        name: skill.name,
        url: skill.url,
        headers: skill.headers,
        oauth_provider: skill.oauth_provider,
      });

      setValidationResult(result.data);
      setValidationError(null);
    } catch (err) {
      console.error('Validation error:', err);
      const axiosError = err as AxiosError;
      const errMessage = axiosError.response?.data ?
        typeof axiosError.response.data === 'string' ?
          axiosError.response.data :
          JSON.stringify(axiosError.response.data) :
        axiosError.message || 'Failed to validate MCP skill';

      setValidationError(errMessage);
      setValidationResult(null);
    } finally {
      setValidating(false);
    }
  };

  const handleSave = async () => {
    try {
      setError(null);

      // Validate URL before saving
      if (!skill.url.toLowerCase().startsWith('http')) {
        setUrlError('URL must start with http:// or https://');
        return;
      }

      // Construct the MCP skill object
      const mcpSkill = {
        name: skill.name,
        url: skill.url,
        headers: skill.headers,
        oauth_provider: skill.oauth_provider || undefined,
        oauth_scopes: skill.oauth_scopes.filter(s => s.trim() !== ''),
      };

      // Copy app object, has to be deep copy as we have arrays inside
      const appCopy = JSON.parse(JSON.stringify(app));

      // Initialize mcpTools array if it doesn't exist
      if (!appCopy.mcpTools) {
        appCopy.mcpTools = [];
      }

      // Based on index update the app mcpTools array (if set, otherwise add)
      if (existingSkillIndex !== null) {
        appCopy.mcpTools[existingSkillIndex] = mcpSkill;
      } else {
        appCopy.mcpTools.push(mcpSkill);
      }

      // Update the application
      await onUpdate(appCopy);

      onClose();
    } catch (err) {
      console.log(err);
      // Convert to axios error
      const axiosError = err as AxiosError;
      // If we have response, then show err.response.data, otherwise show err.message      
      const errMessage = axiosError.response?.data ? JSON.stringify(axiosError.response.data) : axiosError.message || 'Failed to save skill';

      setError(errMessage);
    }
  };

  const handleDisable = async () => {
    if (existingSkillIndex !== null) {
      // Remove the skill from mcpTools
      app.mcpTools = app.mcpTools?.filter((_, index) => index !== existingSkillIndex);
      await onUpdate(app);
    }
    onClose();
  };

  const handleClose = () => {
    onClose();
  };

  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setActiveTab(newValue);
  };



  const renderMcpTools = (tools: McpTool[]) => {
    return (
      <Box sx={{ mt: 3 }}>
        <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC', display: 'flex', alignItems: 'center', gap: 1 }}>
          <CheckCircleIcon sx={{ color: '#10B981' }} />
          Available MCP Tools ({tools.length})
        </Typography>
        {tools.map((tool, index) => (
          <Accordion
            key={`tool-${index}`}
            sx={{
              background: '#23262F',
              mb: 1,
              '&:before': { display: 'none' },
              boxShadow: 'none',
              '& .MuiAccordionSummary-root': {
                minHeight: 48,
              }
            }}
          >
            <AccordionSummary
              expandIcon={<ExpandMoreIcon sx={{ color: '#A0AEC0' }} />}
              sx={{
                '& .MuiAccordionSummary-content': {
                  margin: '12px 0',
                }
              }}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, width: '100%' }}>
                <Typography sx={{ color: '#F8FAFC', fontWeight: 500 }}>
                  {tool.name}
                </Typography>
                {tool.annotations?.readOnlyHint && (
                  <Chip label="Read Only" size="small" sx={{ background: '#1E40AF', color: '#93C5FD' }} />
                )}
                {tool.annotations?.destructiveHint && (
                  <Chip label="Destructive" size="small" sx={{ background: '#991B1B', color: '#FCA5A5' }} />
                )}
              </Box>
            </AccordionSummary>
            <AccordionDetails>
              <Typography sx={{ color: '#A0AEC0', mb: 2 }}>
                {tool.description || 'No description available'}
              </Typography>
              {tool.inputSchema && tool.inputSchema.properties && (
                <Box>
                  <Typography variant="subtitle2" sx={{ color: '#F8FAFC', mb: 1 }}>
                    Parameters:
                  </Typography>
                  <Box sx={{ pl: 2 }}>
                    {Object.entries(tool.inputSchema.properties).map(([propName, propSchema]: [string, any]) => (
                      <Box key={propName} sx={{ mb: 1 }}>
                        <Typography variant="body2" sx={{ color: '#F8FAFC' }}>
                          • {propName}
                          {tool.inputSchema?.required?.includes(propName) && (
                            <span style={{ color: '#EF4444' }}> *</span>
                          )}
                          {propSchema.type && (
                            <span style={{ color: '#A0AEC0' }}> ({propSchema.type})</span>
                          )}
                        </Typography>
                        {propSchema.description && (
                          <Typography variant="caption" sx={{ color: '#A0AEC0', ml: 2, display: 'block' }}>
                            {propSchema.description}
                          </Typography>
                        )}
                      </Box>
                    ))}
                  </Box>
                </Box>
              )}
            </AccordionDetails>
          </Accordion>
        ))}
      </Box>
    );
  };

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          height: '90vh',
          maxHeight: '800px',
          minHeight: '600px'
        }
      }}
      TransitionProps={{
        onExited: () => {
          setSkill({
            name: '',
            url: '',
            headers: {},
            oauth_provider: '',
            oauth_scopes: [],
          });
          setExistingSkill(null);
          setExistingSkillIndex(null);
          setUrlError(null);
          setValidationResult(null);
          setValidationError(null);
          setOAuthProvider('');
          setOAuthScopes([]);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={{ ...lightTheme.scrollbar, height: '100%', display: 'flex', flexDirection: 'column' }}>
        <Box sx={{
          mt: 2,
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden'
        }}>

          <Box sx={{ flex: 1, overflow: 'auto' }}>
            {/* Tabs */}
            <Box sx={{ borderBottom: 1, borderColor: '#353945', mb: 3 }}>
              <Tabs
                value={activeTab}
                onChange={handleTabChange}
                sx={{
                  '& .MuiTab-root': {
                    color: '#A0AEC0',
                    textTransform: 'none',
                    fontSize: '1rem',
                    fontWeight: 500,
                    '&.Mui-selected': {
                      color: '#F8FAFC',
                    },
                  },
                  '& .MuiTabs-indicator': {
                    backgroundColor: '#6366F1',
                  },
                }}
              >
                <Tab label="General" />
                <Tab label="Details" />
              </Tabs>
            </Box>

            {/* Tab Content */}
            {activeTab === 0 && (
              <Box>
                <NameTypography>
                  {skill.name || 'New MCP Skill'}
                </NameTypography>

                <SectionCard>
                  <DarkTextField
                    fullWidth
                    label="Name"
                    value={skill.name}
                    helperText="The name of the MCP skill, make it informative and unique for the AI"
                    onChange={(e) => handleChange('name', e.target.value)}
                    margin="normal"
                    required
                  />

                  <DarkTextField
                    fullWidth
                    label="MCP Server URL"
                    helperText={urlError || "The URL of the MCP server to connect to. URLs ending with /sse will be treated as SSE endpoints."}
                    value={skill.url}
                    onChange={(e) => handleUrlChange(e.target.value)}
                    margin="normal"
                    required
                    error={!!urlError}
                    sx={{
                      '& .MuiFormHelperText-root': {
                        color: urlError ? '#EF4444' : '#A0AEC0',
                        fontSize: '0.875rem',
                        marginTop: '4px',
                      },
                      '& .MuiOutlinedInput-root': {
                        '&.Mui-error': {
                          '& fieldset': {
                            borderColor: '#EF4444',
                          },
                        },
                      },
                    }}
                  />

                  <Box sx={{ mt: 3 }}>
                    <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
                      Headers Configuration
                    </Typography>
                    <List>
                      {Object.entries(skill.headers).map(([key, value], index) => (
                        <ListItem key={`header-${index}`} sx={{ px: 0 }}>
                          <Grid container spacing={1}>
                            <Grid item xs={5}>
                              <DarkTextField
                                size="small"
                                placeholder="Header Name"
                                value={key}
                                onChange={(e) => {
                                  const newHeaders = { ...skill.headers };
                                  delete newHeaders[key];
                                  newHeaders[e.target.value] = value;
                                  setSkill((prev) => ({
                                    ...prev,
                                    headers: newHeaders,
                                  }));
                                }}
                                fullWidth
                              />
                            </Grid>
                            <Grid item xs={5}>
                              <DarkTextField
                                size="small"
                                placeholder="Header Value"
                                value={value}
                                onChange={(e) => handleHeaderChange(key, e.target.value)}
                                fullWidth
                              />
                            </Grid>
                            <Grid item xs={2}>
                              <IconButton
                                size="small"
                                onClick={() => removeHeader(key)}
                                sx={{ color: '#F87171' }}
                              >
                                <DeleteIcon />
                              </IconButton>
                            </Grid>
                          </Grid>
                        </ListItem>
                      ))}
                    </List>
                    <Button
                      startIcon={<AddIcon />}
                      onClick={addHeader}
                      size="small"
                      sx={{ mt: 1 }}
                    >
                      Add Header
                    </Button>
                  </Box>

                  {validationError && (
                    <Alert
                      variant="outlined"
                      severity="error"
                      sx={{ mb: 3 }}
                      onClose={() => setValidationError(null)}
                    >
                      {validationError}
                    </Alert>
                  )}

                  {validationResult && validationResult.tools && validationResult.tools.length > 0 && (
                    renderMcpTools(validationResult.tools)
                  )}


                  {/* OAuth Configuration Section */}
                  <Box sx={{ mt: 3 }}>
                    <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
                      OAuth Configuration
                    </Typography>
                    <FormControl fullWidth sx={{ mb: 2 }}>
                      <InputLabel id="oauth-provider-label" sx={{ color: '#A0AEC0' }}>OAuth Provider</InputLabel>
                      <Select
                        labelId="oauth-provider-label"
                        id="oauth-provider"
                        value={oauthProvider}
                        label="OAuth Provider"
                        onChange={(e) => handleOAuthProviderChange(e.target.value)}
                        sx={{
                          '& .MuiInputBase-root': {
                            color: '#F1F1F1',
                          },
                          '& .MuiOutlinedInput-notchedOutline': {
                            borderColor: '#353945',
                          },
                          '&:hover .MuiOutlinedInput-notchedOutline': {
                            borderColor: '#6366F1',
                          },
                          '& .MuiSvgIcon-root': {
                            color: '#A0AEC0',
                          },
                        }}
                      >
                        <MenuItem value="">None</MenuItem>
                        {allOAuthProviders.map((provider) => (
                          <MenuItem key={provider.id} value={provider.name}>
                            <Box sx={{ display: 'flex', alignItems: 'center' }}>
                              <Avatar
                                sx={{
                                  bgcolor: PROVIDER_COLORS[provider.type || 'custom'] || PROVIDER_COLORS.custom,
                                  color: 'white',
                                  mr: 1,
                                  width: 24,
                                  height: 24
                                }}
                              >
                                {PROVIDER_ICONS[provider.type || 'custom'] || PROVIDER_ICONS.custom}
                              </Avatar>
                              <span>{provider.name}</span>
                            </Box>
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>

                    {oauthProvidersError && (
                      <Alert
                        variant="outlined"
                        severity="error"
                        sx={{ mt: 1 }}
                      >
                        Failed to load OAuth providers: {oauthProvidersError.message}
                      </Alert>
                    )}

                    {oauthProvider && (
                      <Box sx={{ mb: 2 }}>
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                          <Typography variant="subtitle1" sx={{ color: '#F8FAFC' }}>Required Scopes</Typography>
                          <Button
                            startIcon={<AddIcon />}
                            onClick={addScope}
                            variant="outlined"
                            size="small"
                            sx={{
                              borderColor: '#6366F1',
                              color: '#6366F1',
                              '&:hover': {
                                borderColor: '#818CF8',
                                background: 'rgba(99, 102, 241, 0.1)',
                              },
                            }}
                          >
                            Add Scope
                          </Button>
                        </Box>

                        {oauthScopes.length === 0 ? (
                          <Typography variant="body2" sx={{ color: '#A0AEC0' }}>
                            No scopes defined. Add scopes to request specific permissions.
                          </Typography>
                        ) : (
                          oauthScopes.map((scope, index) => (
                            <Box key={index} sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                              <DarkTextField
                                value={scope}
                                onChange={(e) => handleScopeChange(index, e.target.value)}
                                fullWidth
                                placeholder="Enter scope"
                                size="small"
                              />
                              <IconButton
                                onClick={() => removeScope(index)}
                                color="error"
                                sx={{ ml: 1 }}
                              >
                                <DeleteIcon />
                              </IconButton>
                            </Box>
                          ))
                        )}
                      </Box>
                    )}
                  </Box>
                </SectionCard>

              </Box>
            )}

            {activeTab === 1 && (
              <Box>
                <Typography variant="h6" sx={{ mb: 3, color: '#F8FAFC' }}>
                  MCP Tools
                </Typography>

                {/* URL display */}
                {skill.url && (
                  <Box sx={{ mb: 3 }}>
                    <Typography variant="subtitle2" sx={{ color: '#A0AEC0', mb: 1 }}>
                      MCP Server URL:
                    </Typography>
                    <Typography variant="body1" sx={{ color: '#F1F1F1', fontFamily: 'monospace', bgcolor: '#23262F', p: 1, borderRadius: 1 }}>
                      {skill.url}
                    </Typography>
                  </Box>
                )}

                {/* Get MCP tools from the existing skill configuration */}
                {(() => {
                  // const existingMcpSkill = app.mcpTools?.find(mcp => mcp.name === skill.name);
                  // const tools = existingMcpSkill ? [] : []; // For now, we don't have access to the actual tools array from the MCP server

                  if (parsedMcpTools.length === 0) {
                    return (
                      <Box sx={{
                        border: '1px solid #757575',
                        borderRadius: 2,
                        p: 3,
                        textAlign: 'center',
                        color: '#A0AEC0'
                      }}>
                        <Typography variant="body1" sx={{ mb: 1 }}>
                          No MCP tools found
                        </Typography>
                        <Typography variant="body2">
                          {!parsedMcpTools ?
                            'This MCP skill has not been configured yet.' :
                            'No tools are available from this MCP server.'
                          }
                        </Typography>
                      </Box>
                    );
                  }

                  return (
                    <Box sx={{
                      border: '1px solid #353945',
                      borderRadius: 2,
                      overflow: 'hidden'
                    }}>
                      <Box sx={{
                        bgcolor: '#23262F',
                        p: 2,
                        borderBottom: '1px solid #353945',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1
                      }}>
                        <Tooltip title="MCP Tool">
                          <SettingsIcon fontSize="small" sx={{ color: lightTheme.textColorFaded }} />
                        </Tooltip>
                        <Typography variant="subtitle2" sx={{ color: lightTheme.textColorFaded, fontWeight: 500 }}>
                          Available MCP Tools ({parsedMcpTools.length})
                        </Typography>
                      </Box>

                      <Box sx={{ maxHeight: '400px', overflow: 'auto' }}>
                        {parsedMcpTools.map((tool: McpTool, index: number) => (
                          <Accordion
                            key={`tool-${index}`}
                            sx={{
                              background: '#23262F',
                              mb: 1,
                              '&:before': { display: 'none' },
                              boxShadow: 'none',
                              '& .MuiAccordionSummary-root': {
                                minHeight: 48,
                              }
                            }}
                          >
                            <AccordionSummary
                              expandIcon={<ExpandMoreIcon sx={{ color: '#A0AEC0' }} />}
                              sx={{
                                '& .MuiAccordionSummary-content': {
                                  margin: '12px 0',
                                }
                              }}
                            >
                              <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, width: '100%' }}>
                                <Typography sx={{ color: '#F8FAFC', fontWeight: 500 }}>
                                  {tool.name}
                                </Typography>
                                {tool.annotations?.readOnlyHint && (
                                  <Chip label="Read Only" size="small" sx={{ background: '#1E40AF', color: '#93C5FD' }} />
                                )}
                                {tool.annotations?.destructiveHint && (
                                  <Chip label="Destructive" size="small" sx={{ background: '#991B1B', color: '#FCA5A5' }} />
                                )}
                              </Box>
                            </AccordionSummary>
                            <AccordionDetails>
                              <Typography sx={{ color: '#A0AEC0', mb: 2 }}>
                                {tool.description || 'No description available'}
                              </Typography>
                              {tool.inputSchema && tool.inputSchema.properties && (
                                <Box>
                                  <Typography variant="subtitle2" sx={{ color: '#F8FAFC', mb: 1 }}>
                                    Parameters:
                                  </Typography>
                                  <Box sx={{ pl: 2 }}>
                                    {Object.entries(tool.inputSchema.properties).map(([propName, propSchema]: [string, any]) => (
                                      <Box key={propName} sx={{ mb: 1 }}>
                                        <Typography variant="body2" sx={{ color: '#F8FAFC' }}>
                                          • {propName}
                                          {tool.inputSchema?.required?.includes(propName) && (
                                            <span style={{ color: '#EF4444' }}> *</span>
                                          )}
                                          {propSchema.type && (
                                            <span style={{ color: '#A0AEC0' }}> ({propSchema.type})</span>
                                          )}
                                        </Typography>
                                        {propSchema.description && (
                                          <Typography variant="caption" sx={{ color: '#A0AEC0', ml: 2, display: 'block' }}>
                                            {propSchema.description}
                                          </Typography>
                                        )}
                                      </Box>
                                    ))}
                                  </Box>
                                </Box>
                              )}
                            </AccordionDetails>
                          </Accordion>
                        ))}
                      </Box>
                    </Box>
                  );
                })()}
              </Box>
            )}
          </Box>
        </Box>
      </DialogContent>
      <DialogActions sx={{ background: '#181A20', borderTop: '1px solid #23262F', flexDirection: 'column', alignItems: 'stretch' }}>
        {error && (
          <Box sx={{ width: '100%', pl: 2, pr: 2, mb: 3 }}>
            <Alert variant="outlined" severity="error" sx={{ width: '100%' }}>
              {error}
            </Alert>
          </Box>
        )}
        <Box sx={{ display: 'flex', width: '100%' }}>
          <Button
            onClick={handleClose}
            size="small"
            variant="outlined"
            color="primary"
          >
            Close
          </Button>
          {/* Add spacer here */}
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 1, mr: 2 }}>
            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button
                variant="outlined"
                onClick={handleValidate}
                disabled={!skill.url.trim() || validating || !!urlError}
                startIcon={validating ? <CircularProgress size={16} /> : null}
                size="small"
                sx={{
                  borderColor: '#6366F1',
                  color: '#6366F1',
                  '&:hover': {
                    borderColor: '#818CF8',
                    background: 'rgba(99, 102, 241, 0.1)',
                  },
                  '&:disabled': {
                    borderColor: '#353945',
                    color: '#6B7280',
                  }
                }}
              >
                {validating ? 'Validating...' : 'Validate'}
              </Button>
              {existingSkill && (
                <Button
                  onClick={handleDisable}
                  size="small"
                  variant="outlined"
                  color="error"
                  sx={{ borderColor: '#EF4444', color: '#EF4444', '&:hover': { borderColor: '#DC2626', color: '#DC2626' } }}
                >
                  Disable
                </Button>
              )}
              <Button
                onClick={handleSave}
                size="small"
                variant="outlined"
                color="secondary"
                disabled={!skill.name.trim() || !skill.url.trim()}
              >
                {existingSkill ? 'Save' : 'Enable'}
              </Button>
            </Box>
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default AddMcpSkillDialog;
