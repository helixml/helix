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
  ListItemSecondaryAction,
  Link,
  Alert,
  Menu,
  MenuItem,
  Grid,
  Tooltip,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import { IAgentSkill, IRequiredApiParameter, IAppFlatState, IAssistantApi } from '../../types';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme'
import yaml from 'js-yaml';

// Example skills
import { coindeskTool } from './examples/coindeskApi';
import { jobVacanciesTool } from './examples/jobVacanciesApi';
import { productsTool } from './examples/productsApi';
import { climateTool } from './examples/climateApi';
import { exchangeRatesTool } from './examples/exchangeRatesApi';

interface AddApiSkillDialogProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  skill?: IAgentSkill;
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

const DescriptionTypography = styled(Typography)(({ theme }) => ({
  fontSize: '1.1rem',
  color: '#A0AEC0',
  marginBottom: theme.spacing(3),
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

const AddApiSkillDialog: React.FC<AddApiSkillDialogProps> = ({
  open,
  onClose,  
  onClosed,
  skill: initialSkill,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);

  const [skill, setSkill] = useState<IAgentSkill>({
    name: '',
    description: '',
    systemPrompt: '',
    apiSkill: {
      schema: '',
      url: '',
      requiredParameters: [],
      query: {},
      headers: {},
    },
    configurable: true,
  });
  
  const [existingSkill, setExistingSkill] = useState<IAgentSkill | null>(null);
  const [existingSkillIndex, setExistingSkillIndex] = useState<number | null>(null);
  const [parameterValues, setParameterValues] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [schemaError, setSchemaError] = useState<string | null>(null);
  const [urlError, setUrlError] = useState<string | null>(null);

  useEffect(() => {
    if (initialSkill) {
      console.log('initialSkill: ', initialSkill);
      setSkill({
        ...initialSkill,
        apiSkill: {
          ...initialSkill.apiSkill,
        },
      });
      // Find existing skill in app.apiTools
      const existingIndex = app.apiTools?.findIndex(tool => tool.name === initialSkill.name) ?? -1;
      if (existingIndex !== -1) {
        setExistingSkill(initialSkill);
        setExistingSkillIndex(existingIndex);
      }
    } else {
      // Reset form when opening for new skill
      setSkill({
        name: '',
        description: '',
        systemPrompt: '',
        apiSkill: {
          schema: '',
          url: '',
          requiredParameters: [],
          query: {},
          headers: {},
        },
        configurable: true,
      });
      setExistingSkill(null);
      setExistingSkillIndex(null);
    }
  }, [initialSkill, open, initialIsEnabled, app.apiTools]);

  useEffect(() => {
    if (skill.apiSkill.requiredParameters) {
      const initialValues: Record<string, string> = {};
      
      // If we have an existing skill, try to find its configuration in app.apiTools
      if (existingSkill) {
        const existingTool = app.apiTools?.find(tool => tool.name === existingSkill.name);
        if (existingTool) {
          skill.apiSkill.requiredParameters.forEach(param => {
            if (param.name) {
              // Check if parameter is in query or headers based on its type
              if (param.type === 'header' && existingTool.headers) {
                initialValues[param.name] = existingTool.headers[param.name] || '';
              } else if (param.type === 'query' && existingTool.query) {
                initialValues[param.name] = existingTool.query[param.name] || '';
              } else {
                initialValues[param.name] = '';
              }
            }
          });
        }
      } else {
        // For new skills, just initialize empty values
        skill.apiSkill.requiredParameters.forEach(param => {
          if (param.name && !(param.name in parameterValues)) {
            initialValues[param.name] = '';
          }
        });
      }
      
      setParameterValues(prev => ({ ...initialValues, ...prev }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [skill.apiSkill.requiredParameters, existingSkill, app.apiTools]);

  const handleChange = (field: string, value: string) => {
    setSkill((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const validateSchema = (schema: string): boolean => {
    if (!schema.trim()) {
      setSchemaError('Schema is required');
      return false;
    }

    try {
      // Try parsing as JSON first
      JSON.parse(schema);
      setSchemaError(null);
      console.log("valid schema")
      return true;
    } catch (jsonError) {
      try {
        // If JSON parsing fails, try parsing as YAML, 
        // loaded yaml schema should have several properties:
        // - key "paths" should have at least one element
        // - it should have "openapi" with a version number      
        const yamlSchema = yaml.load(schema) as { paths?: any; openapi?: string };
        if (!yamlSchema.paths || !yamlSchema.openapi) {
          setSchemaError('Schema must be valid OpenAPI 3.0.0');
          return false;
        }

        setSchemaError(null);
        return true;
      } catch (yamlError) {
        setSchemaError('Schema must be valid JSON or YAML');
        return false;
      }
    }
  };

  const handleApiSkillChange = (field: string, value: string | Record<string, string>) => {
    if (field === 'schema') {
      validateSchema(value as string);
    } else if (field === 'url') {
      const url = value as string;
      if (url && !url.toLowerCase().startsWith('http')) {
        setUrlError('URL must start with http:// or https://');
      } else {
        setUrlError(null);
      }
    }
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        [field]: value,
      },
    }));
  };

  const addRequiredParameter = () => {
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        requiredParameters: [
          ...prev.apiSkill.requiredParameters,
          {
            name: '',
            description: '',
            type: 'query' as IRequiredApiParameter,
            required: true,
          },
        ],
      },
    }));
  };

  const removeRequiredParameter = (index: number) => {
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        requiredParameters: prev.apiSkill.requiredParameters.filter((_, i) => i !== index),
      },
    }));
  };

  const updateRequiredParameter = (index: number, field: string, value: string | boolean) => {
    setSkill((prev) => ({
      ...prev,
      apiSkill: {
        ...prev.apiSkill,
        requiredParameters: prev.apiSkill.requiredParameters.map((param, i) =>
          i === index ? { ...param, [field]: value } : param
        ),
      },
    }));
  };  

  const handleParameterValueChange = (name: string, value: string) => {
    setParameterValues(prev => ({ ...prev, [name]: value }));
  };

  // Check if all required parameters have values, used to ensure
  // user can't save the skill without filling all required parameters
  const areAllParametersFilled = () => {            
    return skill.apiSkill.requiredParameters.every(param => {
      if (!param.required) return true;
      return parameterValues[param.name]?.trim() !== '';
    });
  };

  const handleSave = async () => {        
    try {
      setError(null);
      
      // Validate schema before saving
      if (!validateSchema(skill.apiSkill.schema)) {
        return;
      }

      // Validate URL before saving
      if (!skill.apiSkill.url.toLowerCase().startsWith('http')) {
        setUrlError('URL must start with http:// or https://');
        return;
      }

      // Construct the IAssistantApi object, which will be used 
      // to update the application
      const assistantApi: IAssistantApi = {
        name: skill.name,
        description: skill.description,
        system_prompt: skill.systemPrompt,
        schema: skill.apiSkill.schema,
        url: skill.apiSkill.url,      
        headers: skill.apiSkill.headers || {},
        query: skill.apiSkill.query || {},
      };

      // Go through required parameters based on parameter type add it to either
      // header or query
      skill.apiSkill.requiredParameters.forEach(param => {
        switch (param.type) {
          case 'header':
            assistantApi.headers![param.name] = parameterValues[param.name];
            break;
          case 'query':
            assistantApi.query![param.name] = parameterValues[param.name];
            break;
          default:
            assistantApi.query![param.name] = parameterValues[param.name];
        }
      });    

      // Copy app object, has to be deep copy as we have arrays inside,
      // so making a copy, adding a new skill into it and updating the app
      const appCopy = JSON.parse(JSON.stringify(app));

      // Based on index update the app api tools array (if set, otherwise add)
      if (existingSkillIndex !== null) {      
        appCopy.apiTools![existingSkillIndex] = assistantApi;
      } else {
        appCopy.apiTools!.push(assistantApi);
      }      

      // Update the application
      await onUpdate(appCopy);

      onClose();
    } catch (err) {
      console.log(err)
      // Convert to axios error
      const axiosError = err as AxiosError;   
      // If we have response, then show err.response.data, otherwise show err.message      
      const errMessage = axiosError.response?.data ? JSON.stringify(axiosError.response.data) : axiosError.message || 'Failed to save skill';
      
      setError(errMessage);
    }
  };

  const handleDisable = async () => {
    if (existingSkillIndex !== null) {
      // Remove the skill from apiTools
      app.apiTools = app.apiTools?.filter((_, index) => index !== existingSkillIndex);
      await onUpdate(app);
    }
    onClose();
  };

  const handleClose = () => {
    onClose();
  };

  const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleExampleSelect = (example: IAssistantApi) => {
    setSkill({
      name: example.name,
      description: example.description,
      systemPrompt: example.system_prompt || '',
      apiSkill: {
        schema: example.schema,
        url: example.url,
        requiredParameters: [],
      },
      configurable: true,
    });
    handleMenuClose();
  };

  const renderDescriptionWithLinks = (text: string) => {
    // URL regex pattern
    const urlRegex = /(https?:\/\/[^\s]+)/g;
    
    // First split by newlines
    const lines = text.split('\n');
    
    return lines.map((line, lineIndex) => {
      // Then split each line by URLs
      const parts = line.split(urlRegex);
      
      const elements = parts.map((part, partIndex) => {
        if (part.match(urlRegex)) {
          return (
            <Link
              key={`${lineIndex}-${partIndex}`}
              href={part}
              target="_blank"
              rel="noopener noreferrer"
              sx={{ color: '#6366F1', textDecoration: 'underline' }}
            >
              {part}
            </Link>
          );
        }
        return part;
      });

      // Add line break after each line except the last one
      return (
        <React.Fragment key={lineIndex}>
          {elements}
          {lineIndex < lines.length - 1 && <br />}
        </React.Fragment>
      );
    });
  };

  return (
    <DarkDialog 
      open={open} 
      onClose={handleClose} 
      maxWidth="md" 
      fullWidth
      TransitionProps={{
        onExited: () => {
          setSkill({
            name: '',
            description: '',
            systemPrompt: '',
            apiSkill: {
              schema: '',
              url: '',
              requiredParameters: [],
              query: {},
              headers: {},
            },
            configurable: true,
          });
          setExistingSkill(null);
          setExistingSkillIndex(null);
          setParameterValues({});
          setSchemaError(null);
          setUrlError(null);
          onClosed?.();
        }
      }}
    >
      <DialogContent sx={lightTheme.scrollbar}>
        <Box sx={{ 
          mt: 2,          
          }}>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <NameTypography>
              {skill.name || 'New API Skill'}
            </NameTypography>
            <Button
              onClick={handleMenuClick}
              variant="outlined"
              size="small"
              sx={{ 
                color: '#A0AEC0',
                borderColor: '#353945',
                '&:hover': {
                  borderColor: '#6366F1',
                  color: '#6366F1',
                },
                textTransform: 'none',
                fontSize: '0.875rem',
                py: 0.5,
                px: 1.5,
              }}
            >
              Load from examples
            </Button>
            <Menu
              anchorEl={anchorEl}
              open={Boolean(anchorEl)}
              onClose={handleMenuClose}
              PaperProps={{
                sx: {
                  bgcolor: '#23262F',
                  color: '#F1F1F1',
                  '& .MuiMenuItem-root': {
                    '&:hover': {
                      bgcolor: '#353945',
                    },
                  },
                },
              }}
            >
              <MenuItem onClick={() => handleExampleSelect(coindeskTool)}>CoinDesk API</MenuItem>
              <MenuItem onClick={() => handleExampleSelect(climateTool)}>Climate API</MenuItem>
              <MenuItem onClick={() => handleExampleSelect(jobVacanciesTool)}>Job Vacancies API</MenuItem>
              <MenuItem onClick={() => handleExampleSelect(exchangeRatesTool)}>Exchange Rates API</MenuItem>
              <MenuItem onClick={() => handleExampleSelect(productsTool)}>Products API</MenuItem>
            </Menu>
          </Box>
          <DescriptionTypography>
            {renderDescriptionWithLinks(skill.description || 'No description provided.')}
          </DescriptionTypography>

          {skill.configurable && (
            <SectionCard>
              <DarkTextField
                fullWidth
                label="Name"
                value={skill.name}
                helperText="The name of the skill, make it informative and unique for the AI"
                onChange={(e) => handleChange('name', e.target.value)}
                margin="normal"
                required
              />
              <DarkTextField
                fullWidth
                label="Description"
                helperText="A short description of the skill, make it informative and unique for the AI"
                value={skill.description}
                onChange={(e) => handleChange('description', e.target.value)}
                margin="normal"
                required
                multiline
                rows={2}
              />
              <DarkTextField
                fullWidth
                label="Skill System Prompt"
                helperText="Will be used when running the skill, add special instructions that could help the AI understand the skill better"
                value={skill.systemPrompt}
                onChange={(e) => handleChange('systemPrompt', e.target.value)}
                margin="normal"
                required
                multiline
                rows={4}
              />
              <DarkTextField
                fullWidth
                label="Server URL"
                helperText={urlError || "This URL will be used to make API calls"}
                value={skill.apiSkill.url}
                onChange={(e) => handleApiSkillChange('url', e.target.value)}
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

              <Box sx={{ mt: 2, mb: 2, ml: 1 }}>
                <Typography variant="subtitle1" sx={{ mb: 2, color: '#F8FAFC' }}>
                  API Configuration
                </Typography>
                <Grid container spacing={2}>
                  <Grid item xs={6}>
                    <Tooltip title="Add additional query parameters that will always be set to the API calls that Helix makes.">
                      <Typography variant="subtitle2" sx={{ mb: 1, color: '#A0AEC0' }}>
                        Query Parameters
                      </Typography>
                    </Tooltip>
                    <List>
                      {Object.entries(skill.apiSkill.query || {}).map(([key, value], index) => (
                        <ListItem key={`query-${index}`} sx={{ px: 0 }}>
                          <Grid container spacing={1}>
                            <Grid item xs={5}>
                              <DarkTextField
                                size="small"
                                placeholder="Key"
                                value={key}
                                onChange={(e) => {
                                  const newQuery = { ...skill.apiSkill.query };
                                  delete newQuery[key];
                                  newQuery[e.target.value] = value;
                                  handleApiSkillChange('query', newQuery);
                                }}
                                fullWidth
                              />
                            </Grid>
                            <Grid item xs={5}>
                              <DarkTextField
                                size="small"
                                placeholder="Value"
                                value={value}
                                onChange={(e) => {
                                  const newQuery = { ...skill.apiSkill.query };
                                  newQuery[key] = e.target.value;
                                  handleApiSkillChange('query', newQuery);
                                }}
                                fullWidth
                              />
                            </Grid>
                            <Grid item xs={2}>
                              <IconButton
                                size="small"
                                onClick={() => {
                                  const newQuery = { ...skill.apiSkill.query };
                                  delete newQuery[key];
                                  handleApiSkillChange('query', newQuery);
                                }}
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
                      onClick={() => {
                        const newQuery = { ...skill.apiSkill.query, '': '' };
                        handleApiSkillChange('query', newQuery);
                      }}
                      size="small"
                      sx={{ mt: 1 }}
                    >
                      Add Query Parameter
                    </Button>
                  </Grid>
                  <Grid item xs={6}>
                    <Tooltip title="Add additional headers that will always be set to the API calls that Helix makes.">
                      <Typography variant="subtitle2" sx={{ mb: 1, color: '#A0AEC0' }}>
                        Headers
                      </Typography>
                    </Tooltip>
                    <List>
                      {Object.entries(skill.apiSkill.headers || {}).map(([key, value], index) => (
                        <ListItem key={`header-${index}`} sx={{ px: 0 }}>
                          <Grid container spacing={1}>
                            <Grid item xs={5}>
                              <DarkTextField
                                size="small"
                                placeholder="Key"
                                value={key}
                                onChange={(e) => {
                                  const newHeaders = { ...skill.apiSkill.headers };
                                  delete newHeaders[key];
                                  newHeaders[e.target.value] = value;
                                  handleApiSkillChange('headers', newHeaders);
                                }}
                                fullWidth
                              />
                            </Grid>
                            <Grid item xs={5}>
                              <DarkTextField
                                size="small"
                                placeholder="Value"
                                value={value}
                                onChange={(e) => {
                                  const newHeaders = { ...skill.apiSkill.headers };
                                  newHeaders[key] = e.target.value;
                                  handleApiSkillChange('headers', newHeaders);
                                }}
                                fullWidth
                              />
                            </Grid>
                            <Grid item xs={2}>
                              <IconButton
                                size="small"
                                onClick={() => {
                                  const newHeaders = { ...skill.apiSkill.headers };
                                  delete newHeaders[key];
                                  handleApiSkillChange('headers', newHeaders);
                                }}
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
                      onClick={() => {
                        const newHeaders = { ...skill.apiSkill.headers, '': '' };
                        handleApiSkillChange('headers', newHeaders);
                      }}
                      size="small"
                      sx={{ mt: 1 }}
                    >
                      Add Header
                    </Button>
                  </Grid>
                </Grid>
              </Box>

              <DarkTextField
                fullWidth
                label="OpenAPI Schema"
                value={skill.apiSkill.schema}
                onChange={(e) => handleApiSkillChange('schema', e.target.value)}
                margin="normal"
                required
                multiline
                rows={10}
                error={!!schemaError}
                helperText={schemaError || "OpenAPI (Swagger) schema of the API, can be YAML or JSON"}
                sx={{
                  '& .MuiFormHelperText-root': {
                    color: schemaError ? '#EF4444' : '#A0AEC0',
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
            </SectionCard>
          )}

          {skill.apiSkill.requiredParameters.length > 0 && (
            <SectionCard>
              <Typography variant="h6" gutterBottom sx={{ color: '#F8FAFC' }}>
                Settings
              </Typography>
              <List>
                {skill.apiSkill.requiredParameters.map((param, index) => (
                  <ListItem key={index} alignItems="flex-start" sx={{ background: '#181A20', borderRadius: 2, mb: 1 }}>
                    <Box sx={{ flex: 1, mb: 2 }}>
                      <Typography variant="subtitle2" sx={{ mb: 0.5, color: '#F1F1F1' }}>
                        {param.name}
                      </Typography>
                      <Typography variant="caption" color="#A0AEC0" sx={{ mb: 1, display: 'block' }}>
                        {renderDescriptionWithLinks(param.description)}
                      </Typography>
                      <DarkTextField                      
                        value={parameterValues[param.name] || ''}
                        onChange={e => handleParameterValueChange(param.name, e.target.value)}
                        size="small"
                        fullWidth
                        required={param.required}
                        sx={{ mt: 0.5 }}
                      />
                    </Box>
                    {param.required === false && skill.configurable && (
                      <ListItemSecondaryAction>
                        <IconButton
                          edge="end"
                          aria-label="delete"
                          onClick={() => removeRequiredParameter(index)}
                          sx={{ color: '#F87171' }}
                        >
                          <DeleteIcon />
                        </IconButton>
                      </ListItemSecondaryAction>
                    )}
                  </ListItem>
                ))}
              </List>
              {skill.configurable && (
                <DarkButton
                  startIcon={<AddIcon />}
                  onClick={addRequiredParameter}
                  variant="outlined"
                  size="small"
                  sx={{ mt: 1, borderColor: '#353945' }}
                >
                  Add Parameter
                </DarkButton>
              )}
            </SectionCard>
          )}
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
            Cancel
          </Button>
          {/* Add spacer here */}
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 1, mr: 2 }}>
            <Box sx={{ display: 'flex', gap: 1 }}>
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
                disabled={!areAllParametersFilled()}
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

export default AddApiSkillDialog;
