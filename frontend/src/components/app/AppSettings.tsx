import React, { useState, useEffect, FC, useRef } from 'react'
import Box from '@mui/material/Box'
import Checkbox from '@mui/material/Checkbox'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import ModelPicker from '../create/ModelPicker'
import ProviderEndpointPicker from '../create/ProviderEndpointPicker'
import {
  TypesProviderEndpoint,
} from '../../api/api'
import {
  IAppFlatState,
} from '../../types'

interface AppSettingsProps {
  id: string,
  app: IAppFlatState,
  onUpdate: (updates: IAppFlatState) => Promise<void>,
  readOnly?: boolean,
  showErrors?: boolean,
  isAdmin?: boolean,
  providerEndpoints?: TypesProviderEndpoint[],
}

const AppSettings: FC<AppSettingsProps> = ({
  id,
  app,
  onUpdate,
  readOnly = false,
  showErrors = true,
  isAdmin = false,
  providerEndpoints = [],
}) => {
  // Local state for form values
  const [name, setName] = useState(app.name || '')
  const [description, setDescription] = useState(app.description || '')
  const [systemPrompt, setSystemPrompt] = useState(app.systemPrompt || '')
  const [avatar, setAvatar] = useState(app.avatar || '')
  const [image, setImage] = useState(app.image || '')
  const [shared, setShared] = useState(app.shared || false)
  const [global, setGlobal] = useState(app.global || false)
  const [model, setModel] = useState(app.model || '')
  const [provider, setProvider] = useState(app.provider || '')
  
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
      setShared(app.shared || false)
      setGlobal(app.global || false)
      setModel(app.model || '')
      setProvider(app.provider || '')
      
      // Mark as initialized
      isInitialized.current = true
    }
  }, [app]) // Still depend on app, but we'll only use it for initialization

  // Handle blur event - gather all current state values and call onUpdate
  const handleBlur = (field: 'name' | 'description' | 'systemPrompt' | 'avatar' | 'image') => {
    // Get current value based on field name
    const currentValue = {
      name,
      description,
      systemPrompt,
      avatar,
      image
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
        shared,
        global,
        model,
        provider,
      }
      
      // Call onUpdate with the complete updated state
      onUpdate(updatedApp)
    }
  }

  // Handle checkbox changes - these update immediately since they're not typing events
  const handleCheckboxChange = (field: 'shared' | 'global', value: boolean) => {
    if (field === 'shared') {
      setShared(value)
    } else {
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
      shared: field === 'shared' ? value : shared,
      global: field === 'global' ? value : global,
      model,
      provider,
    }
    
    onUpdate(updatedApp)
  }

  // Handle picker changes - these update immediately since they're selection events
  const handlePickerChange = (field: 'model' | 'provider', value: string) => {
    if (field === 'model') {
      setModel(value)
    } else {
      setProvider(value)
    }
    
    // Create updated state and call onUpdate immediately for pickers
    const updatedApp: IAppFlatState = {
      ...app,
      name,
      description,
      systemPrompt,
      avatar,
      image,
      shared,
      global,
      model: field === 'model' ? value : model,
      provider: field === 'provider' ? value : provider,
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
        <Typography variant="subtitle1" sx={{ mb: 1 }}>Provider</Typography>
        <ProviderEndpointPicker
          providerEndpoint={provider}
          onSetProviderEndpoint={(newProvider) => {
            handlePickerChange('provider', newProvider)
          }}
          providerEndpoints={providerEndpoints}
        />
      </Box>
      <Box sx={{ mb: 3 }}>
        <Typography variant="subtitle1" sx={{ mb: 1 }}>Model</Typography>
        <ModelPicker
          type="text"
          model={model}
          provider={provider}
          onSetModel={(newModel) => {
            handlePickerChange('model', newModel)
          }}
        />
      </Box>
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
      <Tooltip title="Share this app with other users in your organization">
        <FormGroup>
          <FormControlLabel
            control={
              <Checkbox
                checked={shared}
                onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                  handleCheckboxChange('shared', event.target.checked)
                }}
                // Never disable share checkbox -- required for github apps and normal apps
              />
            }
            label="Shared?"
          />
        </FormGroup>
      </Tooltip>
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