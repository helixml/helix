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
import InputLabel from '@mui/material/InputLabel'
import Typography from '@mui/material/Typography'
import Stack from '@mui/material/Stack'

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

const AppSettings: FC<AppSettingsProps> = ({
  id,
  app,
  onUpdate,
  readOnly = false,
  showErrors = true,
  isAdmin = false,
}) => {
  // Local state for form values
  const [name, setName] = useState(app.name || '')
  const [description, setDescription] = useState(app.description || '')
  const [systemPrompt, setSystemPrompt] = useState(app.systemPrompt || '')
  const [avatar, setAvatar] = useState(app.avatar || '')
  const [image, setImage] = useState(app.image || '')
  const [global, setGlobal] = useState(app.global || false)
  const [model, setModel] = useState(app.model || '')
  const [provider, setProvider] = useState(app.provider || '')
  
  // Advanced settings state
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [contextLimit, setContextLimit] = useState(app.context_limit || 0)
  const [frequencyPenalty, setFrequencyPenalty] = useState(app.frequency_penalty || 0)
  const [maxTokens, setMaxTokens] = useState(app.max_tokens || 1000)
  const [presencePenalty, setPresencePenalty] = useState(app.presence_penalty || 0)
  const [reasoningEffort, setReasoningEffort] = useState(app.reasoning_effort || 'medium')
  const [temperature, setTemperature] = useState(app.temperature || 0.1)
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
      setSystemPrompt(app.systemPrompt || '')
      setAvatar(app.avatar || '')
      setImage(app.image || '')
      setGlobal(app.global || false)
      setModel(app.model || '')
      setProvider(app.provider || '')
      setContextLimit(app.context_limit || 0)
      setFrequencyPenalty(app.frequency_penalty || 0)
      setMaxTokens(app.max_tokens || 1000)
      setPresencePenalty(app.presence_penalty || 0)
      setReasoningEffort(app.reasoning_effort || 'medium')
      setTemperature(app.temperature || 1)
      setTopP(app.top_p || 1)
      
      // Mark as initialized
      isInitialized.current = true
    }
  }, [app]) // Still depend on app, but we'll only use it for initialization

  // Handle blur event - gather all current state values and call onUpdate
  const handleBlur = (field: 'name' | 'description' | 'systemPrompt' | 'avatar' | 'image' | 'max_tokens') => {
    // Get current value based on field name
    const currentValue = {
      name,
      description,
      systemPrompt,
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
        systemPrompt,
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
    }
  }

  // Create debounced version of the update function
  const debouncedUpdate = useDebounce((field: string, value: number | string) => {
    const updatedApp: IAppFlatState = {
      ...app,
      name,
      description,
      systemPrompt,
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
      systemPrompt,
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
      systemPrompt,
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

  return (
    <Box sx={{ mt: 2 }}>
      <TextField
        sx={{ mb: 3 }}
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
        sx={{ mb: 3 }}
        id="app-description"
        name="app-description"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        onBlur={() => handleBlur('description')}
        disabled={readOnly}
        fullWidth
        rows={2}
        label="Description"
        helperText="Enter a short description of what this app does"
      />

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
            label="Advanced"
          />
        </Stack>
      </Box>

      {showAdvanced && (
        <Box sx={{ mb: 3 }}>
          <Typography variant="h6" gutterBottom>Advanced Model Settings</Typography>
          
          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel id="context-limit-label">Context Limit</InputLabel>
            <Select
              labelId="context-limit-label"
              value={contextLimit}
              label="Context Limit"
              onChange={(e) => handleAdvancedChangeWithDebounce('contextLimit', e.target.value as number)}
              disabled={readOnly}
            >
              <MenuItem value={0}>All Previous Messages</MenuItem>
              {Array.from({length: 100}, (_, i) => i + 1).map(num => (
                <MenuItem key={num} value={num}>{num} Message{num > 1 ? 's' : ''}</MenuItem>
              ))}
            </Select>
          </FormControl>

          <Box sx={{ mb: 3 }}>
            <Typography gutterBottom>Frequency Penalty (0-2)</Typography>
            <Tooltip title="Increases the model's likelihood to talk about new topics. Higher values (2) make it less repetitive, while lower values (0) maintain balanced responses.">
              <Slider
                value={frequencyPenalty}
                onChange={(_, value) => handleAdvancedChangeWithDebounce('frequencyPenalty', value as number)}
                min={0}
                max={2}
                step={0.1}
                marks
                disabled={readOnly}
              />
            </Tooltip>
          </Box>

          <TextField
            sx={{ mb: 3 }}
            type="number"
            label="Max Tokens"
            value={maxTokens}
            onChange={(e) => handleAdvancedChangeWithDebounce('maxTokens', parseInt(e.target.value))}
            onBlur={() => handleBlur('max_tokens')}
            fullWidth
            disabled={readOnly}
            helperText="Maximum number of tokens to generate (default: 1000)"
          />

          <Box sx={{ mb: 3 }}>
            <Typography gutterBottom>Presence Penalty (0-2)</Typography>
            <Tooltip title="Increases the model's likelihood to talk about new topics. Higher values (2) make it more open-minded, while lower values (0) maintain balanced responses.">
              <Slider
                value={presencePenalty}
                onChange={(_, value) => handleAdvancedChangeWithDebounce('presencePenalty', value as number)}
                min={0}
                max={2}
                step={0.1}
                marks
                disabled={readOnly}
              />
            </Tooltip>
          </Box>          

          <Box sx={{ mb: 3 }}>
            <Typography gutterBottom>Temperature ({temperature.toFixed(2)})</Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
              {/* This is a hack to get the slider to center the labels */}
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
              {/* This is a hack to get the slider to center the labels */}
              <Typography variant="caption" sx={{ mr: 3 }}></Typography>
            </Box>
            <Typography variant="body2" color="text.secondary">
              Controls randomness in the output. Lower values make it more focused and precise, while higher values make it more creative.
            </Typography>
          </Box>

          <Box sx={{ mb: 3 }}>
            <Typography gutterBottom>Top P (0-1)</Typography>
            <Tooltip title="Controls diversity via nucleus sampling. Lower values (near 0) make output more focused, while higher values (near 1) allow more diverse responses.">
              <Slider
                value={topP}
                onChange={(_, value) => handleAdvancedChangeWithDebounce('topP', value as number)}
                min={0}
                max={1}
                step={0.1}
                marks
                disabled={readOnly}
              />
            </Tooltip>
          </Box>

          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel id="reasoning-effort-label">Reasoning Effort (for thinking models)</InputLabel>
            <Select
              labelId="reasoning-effort-label"
              value={reasoningEffort}
              label="Reasoning Effort"
              onChange={(e) => handleAdvancedChangeWithDebounce('reasoningEffort', e.target.value)}
              disabled={readOnly}
            >
              <MenuItem value="low">Low</MenuItem>
              <MenuItem value="medium">Medium</MenuItem>
              <MenuItem value="high">High</MenuItem>
            </Select>
          </FormControl>
          <Typography variant="body2" color="text.secondary">
            Constrains effort on reasoning for reasoning models. Reducing reasoning effort can result in faster responses and fewer tokens used on reasoning in a response.
          </Typography>
        
          <Divider sx={{ mb: 3, mt: 3 }} />
        </Box>
        
      )}
         
      <TextField
        sx={{ mb: 3 }}
        id="app-instructions"
        name="app-instructions"
        value={systemPrompt}
        onChange={(e) => setSystemPrompt(e.target.value)}
        onBlur={() => handleBlur('systemPrompt')}
        disabled={readOnly}
        fullWidth
        multiline
        rows={4}
        label="Instructions"
        helperText="What does this app do? How does it behave? What should it avoid doing?"
      />
      <TextField
        sx={{ mb: 3 }}
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
        sx={{ mb: 3 }}
        id="app-image"
        name="app-image"
        value={image}
        onChange={(e) => setImage(e.target.value)}
        onBlur={() => handleBlur('image')}
        disabled={readOnly}
        fullWidth
        label="Image"
        helperText="URL for the app's main image"
      />
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