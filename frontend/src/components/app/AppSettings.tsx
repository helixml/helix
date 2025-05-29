import React, { useState, useEffect, FC, useRef, useCallback } from 'react'
import Box from '@mui/material/Box'
import Checkbox from '@mui/material/Checkbox'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Switch from '@mui/material/Switch'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import Slider from '@mui/material/Slider'
import FormControl from '@mui/material/FormControl'
import Typography from '@mui/material/Typography'
import Stack from '@mui/material/Stack'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown'
import Menu from '@mui/material/Menu'

import {
  IAppFlatState,
} from '../../types'
import { AdvancedModelPicker } from '../create/AdvancedModelPicker'
import Divider from '@mui/material/Divider'

interface AppSettingsProps {
  id: string,
  app: IAppFlatState,
  onUpdate: (updates: IAppFlatState) => Promise<void>,
  readOnly?: boolean,
  showErrors?: boolean,
  isAdmin?: boolean,
}

// Add this custom hook after the imports and before the AppSettings component
const useDebounce = (callback: Function, delay: number) => {
  const timeoutRef = useRef<NodeJS.Timeout>()

  return useCallback((...args: any[]) => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
    }

    timeoutRef.current = setTimeout(() => {
      callback(...args)
    }, delay)
  }, [callback, delay])
}

const DEFAULT_SYSTEM_PROMPT = `You are a helpful AI assistant called Helix. Today is {{ .LocalDate }}, local time is {{ .LocalTime }}.`

// Define default values.
// If you are updating these, also update 
// 'func setAppDefaults(apps ...*types.App)' in api/pkg/store/store_apps.go
const DEFAULT_VALUES = {
  system_prompt: DEFAULT_SYSTEM_PROMPT,
  context_limit: 0,
  temperature: 0.1,
  frequency_penalty: 0,
  presence_penalty: 0,
  top_p: 1,
  max_tokens: 2000,
  reasoning_effort: 'medium',
  max_iterations: 10,
} as const

const AppSettings: FC<AppSettingsProps> = ({
  id,
  app,
  onUpdate,
  readOnly = false,
  showErrors = true,
  isAdmin = false,
}) => {
  // Get initial showAdvanced value from URL
  const [showAdvanced, setShowAdvanced] = useState(() => {
    const params = new URLSearchParams(window.location.search)
    return params.get('showAdvanced') === 'true'
  })

  // Update URL when showAdvanced changes
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (showAdvanced) {
      params.set('showAdvanced', 'true')
    } else {
      params.delete('showAdvanced')
    }
    // Update URL without causing a page reload
    window.history.replaceState({}, '', `${window.location.pathname}?${params}`)
  }, [showAdvanced])

  // State for form fields
  const [name, setName] = useState(app.name || '')
  const [description, setDescription] = useState(app.description || '')
  const [system_prompt, setSystemPrompt] = useState(app.system_prompt || '')
  const [avatar, setAvatar] = useState(app.avatar || '')
  const [image, setImage] = useState(app.image || '')
  const [global, setGlobal] = useState(app.global || false)
  const [model, setModel] = useState(app.model || '')
  const [provider, setProvider] = useState(app.provider || '')

  // Agent mode settings
  const [agent_mode, setAgentMode] = useState(app.agent_mode || false)
  const [max_iterations, setMaxIterations] = useState(app.max_iterations ?? DEFAULT_VALUES.max_iterations)
  const [reasoning_model, setReasoningModel] = useState(app.reasoning_model || '')
  const [reasoning_model_provider, setReasoningModelProvider] = useState(app.reasoning_model_provider || '')
  const [reasoning_model_effort, setReasoningModelEffort] = useState(app.reasoning_model_effort || 'medium')
  const [generation_model, setGenerationModel] = useState(app.generation_model || '')
  const [generation_model_provider, setGenerationModelProvider] = useState(app.generation_model_provider || '')
  const [small_reasoning_model, setSmallReasoningModel] = useState(app.small_reasoning_model || '')
  const [small_reasoning_model_provider, setSmallReasoningModelProvider] = useState(app.small_reasoning_model_provider || '')
  const [small_reasoning_model_effort, setSmallReasoningModelEffort] = useState(app.small_reasoning_model_effort || 'medium')
  const [small_generation_model, setSmallGenerationModel] = useState(app.small_generation_model || '')
  const [small_generation_model_provider, setSmallGenerationModelProvider] = useState(app.small_generation_model_provider || '')
  
  // Advanced settings state
  const [contextLimit, setContextLimit] = useState(app.context_limit || 0)
  const [frequencyPenalty, setFrequencyPenalty] = useState(app.frequency_penalty || 0)
  const [maxTokens, setMaxTokens] = useState(app.max_tokens || 2000)
  const [presencePenalty, setPresencePenalty] = useState(app.presence_penalty || 0)
  const [reasoningEffort, setReasoningEffort] = useState(app.reasoning_effort || 'none')
  const [temperature, setTemperature] = useState(app.temperature || DEFAULT_VALUES.temperature)
  const [topP, setTopP] = useState(app.top_p || 1)
  
  // Track if component has been initialized
  const isInitialized = useRef(false)

  // Update local state ONLY on initial mount, not when app prop changes
  useEffect(() => {
    let useAppName = app.name || ''
    if(app.name && app.name == id) {
      useAppName = ''
    }

    // Only initialize values if not already initialized
    if (!isInitialized.current) {
      setName(useAppName)
      setDescription(app.description || '')
      setSystemPrompt(app.system_prompt || DEFAULT_SYSTEM_PROMPT)
      setAvatar(app.avatar || '')
      setImage(app.image || '')
      setGlobal(app.global || false)
      setModel(app.model || '')
      // Agent configuration
      setAgentMode(app.agent_mode || false)
      // Reasoning configuration
      setReasoningModel(app.reasoning_model || '')
      setReasoningModelProvider(app.reasoning_model_provider || '')
      
      setGenerationModel(app.generation_model || '')
      setGenerationModelProvider(app.generation_model_provider || '')
      
      setSmallReasoningModel(app.small_reasoning_model || '')
      setSmallReasoningModelProvider(app.small_reasoning_model_provider || '')
      
      setSmallGenerationModel(app.small_generation_model || '')
      setSmallGenerationModelProvider(app.small_generation_model_provider || '')
      
      setProvider(app.provider || '')
      setContextLimit(app.context_limit || 0)
      setFrequencyPenalty(app.frequency_penalty || 0)
      setMaxTokens(app.max_tokens || 0)
      setPresencePenalty(app.presence_penalty || 0)
      setReasoningEffort(app.reasoning_effort || DEFAULT_VALUES.reasoning_effort)
      setTemperature(app.temperature || 0)
      setTopP(app.top_p || 0)
      setMaxIterations(app.max_iterations ?? DEFAULT_VALUES.max_iterations)
      
      // Mark as initialized
      isInitialized.current = true
    }
  }, [app]) // Still depend on app, but we'll only use it for initialization
 
  // Handle blur event - gather all current state values and call onUpdate
  const handleBlur = (field: 'name' | 'description' | 'system_prompt' | 'avatar' | 'image' | 'max_tokens' | 'max_iterations') => {
    // Get current value based on field name
    const currentValue = {
      name,
      description,
      system_prompt,
      avatar,
      image,
      max_tokens: maxTokens,
      max_iterations: max_iterations
    }[field]
    
    // Get original value from app prop
    const originalValue = (app[field] || '') as string
    
    // Only update if the value has changed
    if (currentValue !== originalValue) {
      // Create a new IAppFlatState with all current state values
      const updatedApp: IAppFlatState = {
        ...app, // Keep any properties we're not explicitly managing
        name,
        description,
        system_prompt,
        avatar,
        image,
        global,
        model,
        provider,
        context_limit: contextLimit,
        frequency_penalty: frequencyPenalty,
        max_tokens: maxTokens,
        presence_penalty: presencePenalty,
        reasoning_effort: reasoningEffort,
        temperature,
        top_p: topP,
        max_iterations: max_iterations
      }
      
      onUpdate(updatedApp)
    }
  }

  // Create debounced version of the update function
  const debouncedUpdate = useDebounce((field: 'contextLimit' | 'frequencyPenalty' | 'maxTokens' | 'presencePenalty' | 'reasoningEffort' | 'temperature' | 'topP' | 'system_prompt' | 'maxIterations', value: number | string) => {
    const updatedApp: IAppFlatState = {
      ...app,
      name,
      description,      
      avatar,
      image,
      global,
      model,
      agent_mode,
      reasoning_model,
      reasoning_model_provider,
      reasoning_model_effort,
      generation_model,
      generation_model_provider,
      small_reasoning_model,
      small_reasoning_model_provider,
      small_reasoning_model_effort,
      small_generation_model,
      small_generation_model_provider,
      provider,
      context_limit: field === 'contextLimit' ? value as number : contextLimit,
      frequency_penalty: field === 'frequencyPenalty' ? value as number : frequencyPenalty,
      max_tokens: field === 'maxTokens' ? value as number : maxTokens,
      presence_penalty: field === 'presencePenalty' ? value as number : presencePenalty,
      reasoning_effort: field === 'reasoningEffort' ? value as string : reasoningEffort,
      temperature: field === 'temperature' ? value as number : temperature,
      top_p: field === 'topP' ? value as number : topP,
      system_prompt: field === 'system_prompt' ? value as string : system_prompt,
      max_iterations: field === 'maxIterations' ? value as number : max_iterations
    }
    
    onUpdate(updatedApp)
  }, 300)

  // Combine immediate state update with debounced API call
  const handleAdvancedChangeWithDebounce = (field: 'contextLimit' | 'frequencyPenalty' | 'maxTokens' | 'presencePenalty' | 'reasoningEffort' | 'temperature' | 'topP' | 'system_prompt' | 'maxIterations', value: number | string) => {
    console.log('handleAdvancedChangeWithDebounce', field, value)
    debouncedUpdate(field, value)
  }

  // Handle checkbox changes - these update immediately since they're not typing events
  const handleCheckboxChange = (field: 'global' | 'agent_mode', value: boolean) => {
    if (field === 'global') {
      setGlobal(value)
    } else if (field === 'agent_mode') {
      setAgentMode(value)
    }
    
    // Create updated state and call onUpdate immediately for checkboxes
    const updatedApp: IAppFlatState = {
      ...app,
      name,
      description,
      system_prompt,
      avatar,
      image,
      global: field === 'global' ? value : global,
      agent_mode: field === 'agent_mode' ? value : agent_mode,
      model,
      provider,
      context_limit: contextLimit,
      frequency_penalty: frequencyPenalty,
      max_tokens: maxTokens,
      presence_penalty: presencePenalty,
      reasoning_effort: reasoningEffort,
      temperature,
      top_p: topP,
      max_iterations: max_iterations
    }
    
    onUpdate(updatedApp)
  }

  const handleModelChange = (provider: string, model: string) => {
    setModel(model)
    setProvider(provider)
    
    // Create updated state and call onUpdate immediately for pickers
    const updatedApp: IAppFlatState = {
      ...app,
      name,
      description,
      system_prompt,
      avatar,
      image,
      global,
      model,
      provider,
      context_limit: contextLimit,
      frequency_penalty: frequencyPenalty,
      max_tokens: maxTokens,
      presence_penalty: presencePenalty,
      reasoning_effort: reasoningEffort,
      temperature,
      top_p: topP,
      max_iterations: max_iterations
    }
    
    onUpdate(updatedApp)
  }

  // Helper function to check if a value matches its default
  const isDefault = (field: keyof typeof DEFAULT_VALUES, value: number | string) => {
    return DEFAULT_VALUES[field] === value
  }

  // Helper function to create reset link
  const ResetLink = ({ field, value, onClick }: { field: keyof typeof DEFAULT_VALUES, value: number | string, onClick: () => void }) => {
    if (isDefault(field, value)) {
      return <Typography component="span" color="text.secondary" sx={{ ml: 1, fontSize: '0.875rem' }}>(Default)</Typography>
    }
    return (
      <Link
        component="button"
        variant="body2"
        onClick={onClick}
        sx={{ ml: 1, fontSize: '0.875rem' }}
      >
        (Reset to default)
      </Link>
    )
  }

  // Reset handlers for each field
  const handleReset = (field: keyof typeof DEFAULT_VALUES) => {
    const value = DEFAULT_VALUES[field]
    switch(field) {
      case 'context_limit':
        setContextLimit(value as number)
        handleAdvancedChangeWithDebounce('contextLimit', value as number)
        break
      case 'temperature':
        setTemperature(value as number)
        handleAdvancedChangeWithDebounce('temperature', value as number)
        break
      case 'frequency_penalty':
        setFrequencyPenalty(value as number)
        handleAdvancedChangeWithDebounce('frequencyPenalty', value as number)
        break
      case 'presence_penalty':
        setPresencePenalty(value as number)
        handleAdvancedChangeWithDebounce('presencePenalty', value as number)
        break
      case 'top_p':
        setTopP(value as number)
        handleAdvancedChangeWithDebounce('topP', value as number)
        break
      case 'max_tokens':
        setMaxTokens(value as number)
        handleAdvancedChangeWithDebounce('maxTokens', value as number)
        break
      case 'reasoning_effort':
        setReasoningEffort(value as string)
        handleAdvancedChangeWithDebounce('reasoningEffort', value as string)
        break
      case 'system_prompt':
        setSystemPrompt(value as string)
        handleAdvancedChangeWithDebounce('system_prompt', value as string)
        break
      case 'max_iterations':
        setMaxIterations(value as number)
        handleAdvancedChangeWithDebounce('maxIterations', value as number)
        break
    }
  }

  const [mainEffortMenuAnchor, setMainEffortMenuAnchor] = useState<null | HTMLElement>(null);
  const [smallEffortMenuAnchor, setSmallEffortMenuAnchor] = useState<null | HTMLElement>(null);

  const handleMainEffortClick = (event: React.MouseEvent<HTMLElement>) => {
    setMainEffortMenuAnchor(event.currentTarget);
  };

  const handleSmallEffortClick = (event: React.MouseEvent<HTMLElement>) => {
    setSmallEffortMenuAnchor(event.currentTarget);
  };

  const handleMainEffortClose = () => {
    setMainEffortMenuAnchor(null);
  };

  const handleSmallEffortClose = () => {
    setSmallEffortMenuAnchor(null);
  };

  const handleEffortSelect = (effort: string, isMain: boolean) => {
    if (isMain) {
      setReasoningModelEffort(effort);
      const updatedApp: IAppFlatState = {
        ...app,
        reasoning_model_effort: effort,
      };
      onUpdate(updatedApp);
      handleMainEffortClose();
    } else {
      setSmallReasoningModelEffort(effort);
      const updatedApp: IAppFlatState = {
        ...app,
        small_reasoning_model_effort: effort,
      };
      onUpdate(updatedApp);
      handleSmallEffortClose();
    }
  };

  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ mb: 3 }}>
        <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
          Appearance
        </Typography>
        <TextField
          sx={{ mb: 2 }}
          id="app-name"
          name="app-name"
          error={showErrors && !name}
          value={name}
          disabled={readOnly}
          onChange={(e) => setName(e.target.value)}
          onBlur={() => handleBlur('name')}
          fullWidth
          label="Name"
          helperText="Name your app"
        />
        <TextField
          sx={{ mb: 2 }}
          id="app-description"
          name="app-description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          onBlur={() => handleBlur('description')}
          disabled={readOnly}
          fullWidth
          rows={2}
          label="Description"
          helperText="Enter a short description of what this app does, e.g. 'Tax filing assistant'"
        />
        <TextField
          sx={{ mb: 2 }}
          id="app-avatar"
          name="app-avatar"
          value={avatar}
          onChange={(e) => setAvatar(e.target.value)}
          onBlur={() => handleBlur('avatar')}
          disabled={readOnly}
          fullWidth
          label="Avatar"
          helperText="URL for the app's avatar image"
        />
        <TextField
          sx={{ mb: 2 }}
          id="app-image"
          name="app-image"
          value={image}
          onChange={(e) => setImage(e.target.value)}
          onBlur={() => handleBlur('image')}
          disabled={readOnly}
          fullWidth
          label="Background Image"
          helperText="URL for the app's main image"
        />
      </Box>

      <Box sx={{ mb: 3 }}>
        <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
          App Configuration
        </Typography>
        <Stack direction="row" alignItems="center">
          <Typography gutterBottom>System Instructions</Typography>
          <ResetLink field="system_prompt" value={system_prompt} onClick={() => handleReset('system_prompt')} />
        </Stack>
        <Typography variant="body2" color="text.secondary">
          
        </Typography>
        <TextField
          sx={{ mb: 3, mt: 1 }}
          id="app-instructions"
          name="app-instructions"
          value={system_prompt}
          onChange={(e) => setSystemPrompt(e.target.value)}
          onBlur={() => handleBlur('system_prompt')}
          disabled={readOnly}
          fullWidth
          multiline
          rows={4}
          helperText="What does this app do? How does it behave? What should it avoid doing?"
        />           
      </Box>

      <Box sx={{ mb: 3 }}>
        <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between">
          <Box>
            {/* <Typography variant="h6" gutterBottom>
              Agent Mode
            </Typography> */}
            <Typography variant="body2" color="text.secondary">
              Use a more sophisticated reasoning process with separate models for different tasks.
            </Typography>
          </Box>
          <FormControlLabel
            control={
              <Switch
                checked={agent_mode}
                onChange={(e) => handleCheckboxChange('agent_mode', e.target.checked)}
                disabled={readOnly}
              />
            }
            label="Agent Mode"
          />
        </Stack>

        {agent_mode && (
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle1" sx={{ mb: 2 }}>Agent Configuration</Typography>
            
            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Main Reasoning and Planning Model</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                The model used for reasoning and planning tasks. Adjust reasoning effort based on complexity of the task.
              </Typography>
              <Stack direction="row" spacing={2} alignItems="flex-start">
                <AdvancedModelPicker
                  recommendedModels={['o3-mini', 'o4-mini']}
                  hint='Recommended to use o3-mini level models, should be a strong model capable of using tools and reasoning.'
                  selectedProvider={reasoning_model_provider}
                  selectedModelId={reasoning_model}
                  onSelectModel={(provider, model) => {
                    setReasoningModel(model);
                    setReasoningModelProvider(provider);
                    const updatedApp: IAppFlatState = {
                      ...app,
                      reasoning_model: model,
                      reasoning_model_provider: provider,
                    };
                    onUpdate(updatedApp);
                  }}
                  currentType="text"
                  displayMode="short"
                />
                <Box>
                  <Button
                    variant="text"
                    onClick={handleMainEffortClick}
                    endIcon={<ArrowDropDownIcon />}
                    disabled={readOnly}
                    sx={{
                      borderRadius: '8px',
                      color: 'text.primary',
                      textTransform: 'none',
                      fontSize: '0.875rem',
                      padding: '4px 8px',
                      height: '32px',
                      minWidth: 'auto',
                      maxWidth: '120px',
                      display: 'flex',
                      alignItems: 'center',
                      border: '1px solid #fff',
                      '&:hover': {
                        backgroundColor: (theme) => theme.palette.mode === 'light' ? "#efefef" : "#13132b",
                      },
                      ...(readOnly && {
                        opacity: 0.5,
                        pointerEvents: 'none',
                      }),
                    }}
                  >
                    <Typography 
                      variant="caption" 
                      component="span"
                      sx={{ 
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        display: 'inline-block',
                        lineHeight: 1.2,
                        verticalAlign: 'middle',
                        fontSize: '0.875rem',
                      }}
                    >
                      {reasoning_model_effort.charAt(0).toUpperCase() + reasoning_model_effort.slice(1)}
                    </Typography>
                  </Button>
                  <Menu
                    anchorEl={mainEffortMenuAnchor}
                    open={Boolean(mainEffortMenuAnchor)}
                    onClose={handleMainEffortClose}
                  >
                    {['none', 'low', 'medium', 'high'].map((effort) => (
                      <MenuItem 
                        key={effort} 
                        onClick={() => handleEffortSelect(effort, true)}
                        selected={effort === reasoning_model_effort}
                      >
                        {effort.charAt(0).toUpperCase() + effort.slice(1)}
                      </MenuItem>
                    ))}
                  </Menu>
                </Box>
              </Stack>
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Generation Model</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                The model used for generating responses. Recommended to use gpt-4o level models.
              </Typography>
              <AdvancedModelPicker
                recommendedModels={['gpt-4o', 'gpt-4o-mini']}
                hint='Recommended to use gpt-4o level models, should be a strong model capable of planning next actions and interpreting tool responses.'
                selectedProvider={generation_model_provider}
                selectedModelId={generation_model}
                onSelectModel={(provider, model) => {
                  setGenerationModel(model);
                  setGenerationModelProvider(provider);
                  const updatedApp: IAppFlatState = {
                    ...app,
                    generation_model: model,
                    generation_model_provider: provider,
                  };
                  onUpdate(updatedApp);
                }}
                currentType="text"
                displayMode="short"
              />
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Small Reasoning Model</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                A smaller model used for quick reasoning tasks. Recommended to use o3-mini level models.
              </Typography>
              <Stack direction="row" spacing={2} alignItems="flex-start">
                <AdvancedModelPicker
                  recommendedModels={['o3-mini', 'o4-mini']}
                  hint='Recommended to use o3-mini level models, should be a strong model capable of using tools and reasoning.'
                  selectedProvider={small_reasoning_model_provider}
                  selectedModelId={small_reasoning_model}
                  onSelectModel={(provider, model) => {
                    setSmallReasoningModel(model);
                    setSmallReasoningModelProvider(provider);
                    const updatedApp: IAppFlatState = {
                      ...app,
                      small_reasoning_model: model,
                      small_reasoning_model_provider: provider,
                    };
                    onUpdate(updatedApp);
                  }}
                  currentType="text"
                  displayMode="short"
                />
                <Box>
                  <Button
                    variant="text"
                    onClick={handleSmallEffortClick}
                    endIcon={<ArrowDropDownIcon />}
                    disabled={readOnly}
                    sx={{
                      borderRadius: '8px',
                      color: 'text.primary',
                      textTransform: 'none',
                      fontSize: '0.875rem',
                      padding: '4px 8px',
                      height: '32px',
                      minWidth: 'auto',
                      maxWidth: '120px',
                      display: 'flex',
                      alignItems: 'center',
                      border: '1px solid #fff',
                      '&:hover': {
                        backgroundColor: (theme) => theme.palette.mode === 'light' ? "#efefef" : "#13132b",
                      },
                      ...(readOnly && {
                        opacity: 0.5,
                        pointerEvents: 'none',
                      }),
                    }}
                  >
                    <Typography 
                      variant="caption" 
                      component="span"
                      sx={{ 
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        display: 'inline-block',
                        lineHeight: 1.2,
                        verticalAlign: 'middle',
                        fontSize: '0.875rem',
                      }}
                    >
                      {small_reasoning_model_effort.charAt(0).toUpperCase() + small_reasoning_model_effort.slice(1)}
                    </Typography>
                  </Button>
                  <Menu
                    anchorEl={smallEffortMenuAnchor}
                    open={Boolean(smallEffortMenuAnchor)}
                    onClose={handleSmallEffortClose}
                  >
                    {['none', 'low', 'medium', 'high'].map((effort) => (
                      <MenuItem 
                        key={effort} 
                        onClick={() => handleEffortSelect(effort, false)}
                        selected={effort === small_reasoning_model_effort}
                      >
                        {effort.charAt(0).toUpperCase() + effort.slice(1)}
                      </MenuItem>
                    ))}
                  </Menu>
                </Box>
              </Stack>
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Small Generation Model</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                A smaller model used for quick response generation. Recommended to use gpt-4o-mini level models.
              </Typography>
              <AdvancedModelPicker
                recommendedModels={['gpt-4o', 'gpt-4o-mini']}
                hint='Recommended to use gpt-4o level models, should be a strong model capable of planning next actions and interpreting tool responses.'
                selectedProvider={provider}
                selectedModelId={small_generation_model}
                onSelectModel={(provider, model) => {
                  setSmallGenerationModel(model);
                  setSmallGenerationModelProvider(provider);
                  const updatedApp: IAppFlatState = {
                    ...app,
                    small_generation_model: model,
                    small_generation_model_provider: provider,
                  };
                  onUpdate(updatedApp);
                }}
                currentType="text"
                displayMode="short"
              />
            </Box>

            <Box sx={{ mb: 3 }}>
              <Typography gutterBottom>Max Iterations</Typography>
              <Typography variant="body2" color="text.secondary">
                The maximum number of reasoning iterations to perform before stopping.
                <ResetLink field="max_iterations" value={max_iterations} onClick={() => handleReset('max_iterations')} />
              </Typography>

              <TextField
                sx={{ mt: 1 }}
                type="number"              
                value={max_iterations}
                onChange={(e) => {
                  const value = parseInt(e.target.value) || DEFAULT_VALUES.max_iterations;
                  setMaxIterations(value);
                  handleAdvancedChangeWithDebounce('maxIterations', value);
                }}
                onBlur={() => handleBlur('max_iterations')}
                fullWidth
                disabled={readOnly}
                inputProps={{ min: 1 }}
              />
            </Box>
          </Box>
        )}
      </Box>

      <Box sx={{ mb: 3 }}>
        <Stack direction="row" spacing={2} alignItems="center" justifyContent="space-between">
          <Box flexGrow={1}>
            <AdvancedModelPicker
              selectedProvider={provider}
              selectedModelId={model}
              onSelectModel={handleModelChange}
              currentType="text"
              displayMode="short"
              disabled={agent_mode}
            />
          </Box>
          <FormControlLabel
            control={
              <Switch
                checked={showAdvanced && !agent_mode}
                onChange={(e) => setShowAdvanced(e.target.checked)}
                disabled={readOnly || agent_mode}
              />
            }
            label="Advanced Model Settings"
          />
        </Stack>
      </Box>

      {showAdvanced && !agent_mode && (
        <Box sx={{ mb: 3 }}>
          <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center">
              <Typography gutterBottom>Context Limit</Typography>
              <ResetLink field="context_limit" value={contextLimit} onClick={() => handleReset('context_limit')} />
            </Stack>
            <Typography variant="body2" color="text.secondary">
              The number of messages to include in the context for the AI assistant. When set to 1, the AI assistant will only see and remember the most recent message.
            </Typography>
            <FormControl fullWidth sx={{ mt: 1 }}>
              <Select
                value={contextLimit}
                onChange={(e) => handleAdvancedChangeWithDebounce('contextLimit', e.target.value as number)}
                disabled={readOnly}
              >
                <MenuItem value={0}>All Previous Messages</MenuItem>
                {Array.from({length: 100}, (_, i) => i + 1).map(num => (
                  <MenuItem key={num} value={num}>{num} Message{num > 1 ? 's' : ''}</MenuItem>
                ))}
              </Select>
            </FormControl>           
          </Box>

          <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center">
              <Typography gutterBottom>Temperature ({temperature.toFixed(2)})</Typography>
              <ResetLink field="temperature" value={temperature} onClick={() => handleReset('temperature')} />
            </Stack>
            <Typography variant="body2" color="text.secondary">
              Controls randomness in the output. Lower values make it more focused and precise, while higher values make it more creative.
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', mt: 1 }}>
              <Typography variant="caption" sx={{ mr: 2, ml: 0.9 }}></Typography>
              <Box sx={{ flexGrow: 1 }}>
                <Slider
                  value={temperature}
                  onChange={(_, value) => handleAdvancedChangeWithDebounce('temperature', value as number)}
                  min={0}
                  max={2}
                  step={0.01}
                  marks={[
                    { value: 0, label: 'Precise' },
                    { value: 1, label: 'Neutral' },
                    { value: 2, label: 'Creative' },
                  ]}
                  disabled={readOnly}
                />
              </Box>
              <Typography variant="caption" sx={{ mr: 3 }}></Typography>
            </Box>
          </Box>

          <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center">
              <Typography gutterBottom>Frequency Penalty ({frequencyPenalty.toFixed(2)})</Typography>
              <ResetLink field="frequency_penalty" value={frequencyPenalty} onClick={() => handleReset('frequency_penalty')} />
            </Stack>
            <Typography variant="body2" color="text.secondary">
              Controls how much the model penalizes itself for repeating the same information. Higher values reduce repetition in longer conversations.
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', mt: 1 }}>
              <Typography variant="caption" sx={{ mr: 2, ml: 0.9 }}></Typography>
              <Box sx={{ flexGrow: 1 }}>
                <Slider
                  value={frequencyPenalty}
                  onChange={(_, value) => handleAdvancedChangeWithDebounce('frequencyPenalty', value as number)}
                  min={0}
                  max={2}
                  step={0.01}
                  marks={[
                    { value: 0, label: 'Balanced' },
                    { value: 2, label: 'Less Repetition' },
                  ]}
                  disabled={readOnly}
                />
              </Box>
              <Typography variant="caption" sx={{ mr: 3 }}></Typography>
            </Box>
          </Box>

          <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center">
              <Typography gutterBottom>Presence Penalty ({presencePenalty.toFixed(2)})</Typography>
              <ResetLink field="presence_penalty" value={presencePenalty} onClick={() => handleReset('presence_penalty')} />
            </Stack>
            <Typography variant="body2" color="text.secondary"> 
              Increases the model's likelihood to talk about new topics. Higher values (2) make it more open-minded, while lower values (0) maintain balanced responses.
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', mt: 1 }}>
              <Typography variant="caption" sx={{ mr: 2, ml: 0.9 }}></Typography>
              <Box sx={{ flexGrow: 1 }}>
                <Slider
                value={presencePenalty}
                onChange={(_, value) => handleAdvancedChangeWithDebounce('presencePenalty', value as number)}
                min={0}
                max={2}
                step={0.01}
                marks={[
                  { value: 0, label: 'Balanced' },
                  { value: 2, label: 'Open-Minded' },
                ]}
                disabled={readOnly}
                />
              </Box>
              <Typography variant="caption" sx={{ mr: 3 }}></Typography>
            </Box>
          </Box> 

          <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center">
              <Typography gutterBottom>Top P ({topP.toFixed(2)})</Typography>
              <ResetLink field="top_p" value={topP} onClick={() => handleReset('top_p')} />
            </Stack>
            <Typography variant="body2" color="text.secondary"> 
              Controls diversity via nucleus sampling. Lower values (near 0) make output more focused, while higher values (near 1) allow more diverse responses.
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', mt: 1 }}>
              <Typography variant="caption" sx={{ mr: 2, ml: 0.9 }}></Typography>
              <Box sx={{ flexGrow: 1 }}>
                <Slider
                value={topP}
                onChange={(_, value) => handleAdvancedChangeWithDebounce('topP', value as number)}
                min={0}
                max={1}
                step={0.01}
                marks={[
                  { value: 0, label: 'Precise' },
                  { value: 1, label: 'Creative' },
                ]}
                disabled={readOnly}
                />
              </Box>
              <Typography variant="caption" sx={{ mr: 3 }}></Typography>
            </Box>
          </Box>

          <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center">
              <Typography gutterBottom>Max Tokens</Typography>
              <ResetLink field="max_tokens" value={maxTokens} onClick={() => handleReset('max_tokens')} />
            </Stack>
            <Typography variant="body2" color="text.secondary">
              The maximum number of tokens to generate before stopping.
            </Typography>

            <TextField
              sx={{ mt: 1 }}
              type="number"              
              value={maxTokens}
              onChange={(e) => handleAdvancedChangeWithDebounce('maxTokens', parseInt(e.target.value))}
              onBlur={() => handleBlur('max_tokens')}
              fullWidth
              disabled={readOnly}
            />
          </Box>          

          <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center">
              <Typography gutterBottom>Reasoning Effort</Typography>
              <ResetLink field="reasoning_effort" value={reasoningEffort} onClick={() => handleReset('reasoning_effort')} />
            </Stack>
            <Typography variant="body2" color="text.secondary">
              Controls the effort on reasoning for reasoning models. Reducing reasoning effort can result in faster responses and fewer tokens used on reasoning in a response.
            </Typography>
            <FormControl fullWidth sx={{ mt: 1 }}>              
              <Select              
                value={reasoningEffort}
                onChange={(e) => handleAdvancedChangeWithDebounce('reasoningEffort', e.target.value)}
                disabled={readOnly}
            >
              <MenuItem value="none">None</MenuItem>
              <MenuItem value="low">Low</MenuItem>
              <MenuItem value="medium">Medium</MenuItem>
              <MenuItem value="high">High</MenuItem>
              </Select>
            </FormControl>
          </Box>

          <Divider sx={{ mb: 3, mt: 3 }} />
        </Box>
      )}     
               
      {isAdmin && (
        <Tooltip title="Make this app available to all users">
          <FormGroup>
            <FormControlLabel
              control={
                <Checkbox
                  checked={global}
                  onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                    handleCheckboxChange('global', event.target.checked)
                  }}
                // Never disable global checkbox -- required for github apps and normal apps
                />
              }
              label="Global?"
            />
          </FormGroup>
        </Tooltip>
      )}
    </Box>
  )
}

export default AppSettings