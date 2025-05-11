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
  reasoning_effort: 'medium'
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

  // Local state for form values
  const [name, setName] = useState(app.name || '')
  const [description, setDescription] = useState(app.description || '')
  const [system_prompt, setSystemPrompt] = useState(app.system_prompt || '')
  const [avatar, setAvatar] = useState(app.avatar || '')
  const [image, setImage] = useState(app.image || '')
  const [global, setGlobal] = useState(app.global || false)
  const [model, setModel] = useState(app.model || '')
  const [provider, setProvider] = useState(app.provider || '')
  
  // Advanced settings state
  const [contextLimit, setContextLimit] = useState(app.context_limit || 0)
  const [frequencyPenalty, setFrequencyPenalty] = useState(app.frequency_penalty || 0)
  const [maxTokens, setMaxTokens] = useState(app.max_tokens || 2000)
  const [presencePenalty, setPresencePenalty] = useState(app.presence_penalty || 0)
  const [reasoningEffort, setReasoningEffort] = useState(app.reasoning_effort || 'medium')
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
      setProvider(app.provider || '')
      setContextLimit(app.context_limit || 0)
      setFrequencyPenalty(app.frequency_penalty || 0)
      setMaxTokens(app.max_tokens || 0)
      setPresencePenalty(app.presence_penalty || 0)
      setReasoningEffort(app.reasoning_effort || DEFAULT_VALUES.reasoning_effort)
      setTemperature(app.temperature || 0)
      setTopP(app.top_p || 0)
      
      // Mark as initialized
      isInitialized.current = true
    }
  }, [app]) // Still depend on app, but we'll only use it for initialization

  // Handle blur event - gather all current state values and call onUpdate
  const handleBlur = (field: 'name' | 'description' | 'system_prompt' | 'avatar' | 'image' | 'max_tokens') => {
    // Get current value based on field name
    const currentValue = {
      name,
      description,
      system_prompt,
      avatar,
      image,
      max_tokens: maxTokens
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
      }
      
      // Call onUpdate with the complete updated state
      onUpdate(updatedApp)
    }
  }

  // Modify the handleAdvancedChange function to separate immediate state updates from debounced API calls
  const handleAdvancedChange = (field: string, value: number | string) => {
    const numericValue = typeof value === 'string' ? parseFloat(value) : value
    
    // Update local state immediately
    switch(field) {
      case 'contextLimit':
        setContextLimit(numericValue as number)
        break
      case 'frequencyPenalty':
        setFrequencyPenalty(numericValue as number)
        break
      case 'maxTokens':
        setMaxTokens(numericValue as number)
        break
      case 'presencePenalty':
        setPresencePenalty(numericValue as number)
        break
      case 'reasoningEffort':
        setReasoningEffort(value as string)
        break
      case 'temperature':
        setTemperature(numericValue as number)
        break
      case 'topP':
        setTopP(numericValue as number)
        break
      case 'system_prompt':
        setSystemPrompt(value as string)
        break
    }
  }

  // Create debounced version of the update function
  const debouncedUpdate = useDebounce((field: string, value: number | string) => {
    const updatedApp: IAppFlatState = {
      ...app,
      name,
      description,      
      avatar,
      image,
      global,
      model,
      provider,
      context_limit: field === 'contextLimit' ? value as number : contextLimit,
      frequency_penalty: field === 'frequencyPenalty' ? value as number : frequencyPenalty,
      max_tokens: field === 'maxTokens' ? value as number : maxTokens,
      presence_penalty: field === 'presencePenalty' ? value as number : presencePenalty,
      reasoning_effort: field === 'reasoningEffort' ? value as string : reasoningEffort,
      temperature: field === 'temperature' ? value as number : temperature,
      top_p: field === 'topP' ? value as number : topP,
      system_prompt: field === 'system_prompt' ? value as string : system_prompt,
    }
    
    onUpdate(updatedApp)
  }, 300)

  // Combine immediate state update with debounced API call
  const handleAdvancedChangeWithDebounce = (field: string, value: number | string) => {
    handleAdvancedChange(field, value)
    debouncedUpdate(field, value)
  }

  // Handle checkbox changes - these update immediately since they're not typing events
  const handleCheckboxChange = (field: 'global', value: boolean) => {
    if (field === 'global') {
      setGlobal(value)
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
      model,
      provider,
      context_limit: contextLimit,
      frequency_penalty: frequencyPenalty,
      max_tokens: maxTokens,
      presence_penalty: presencePenalty,
      reasoning_effort: reasoningEffort,
      temperature,
      top_p: topP,
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
    }
  }

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
          <Box flexGrow={1}>
            <AdvancedModelPicker
              selectedProvider={provider}
              selectedModelId={model}
              onSelectModel={handleModelChange}
              currentType="text"
              displayMode="short"
            />
          </Box>
          <FormControlLabel
            control={
              <Switch
                checked={showAdvanced}
                onChange={(e) => setShowAdvanced(e.target.checked)}
                disabled={readOnly}
              />
            }
            label="Advanced Model Settings"
          />
        </Stack>
      </Box>

      {showAdvanced && (
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