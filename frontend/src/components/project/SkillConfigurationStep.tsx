import React, { FC, useState, useEffect } from 'react'
import {
  Box,
  Typography,
  TextField,
  Alert,
  Checkbox,
  FormControlLabel,
  InputAdornment,
  IconButton,
  Link,
  Chip,
} from '@mui/material'
import {
  Visibility,
  VisibilityOff,
  CheckCircle,
  Warning,
} from '@mui/icons-material'
import { TypesAssistantMCP, TypesAssistantSkills } from '../../api/api'

interface SkillConfigurationStepProps {
  skills: TypesAssistantSkills
  currentSkillIndex: number
  totalSkills: number
  configuredEnvVars: Record<string, Record<string, string>> // skillName -> envVar -> value
  onEnvVarChange: (skillName: string, envVar: string, value: string) => void
  oauthConsent: Record<string, boolean> // skillName -> consent given
  onOAuthConsentChange: (skillName: string, consent: boolean) => void
  userOAuthConnections: Array<{ provider?: { type?: string } }>
}

// Check if an env var looks like it needs user input (empty or placeholder)
const needsConfiguration = (value: string): boolean => {
  return value === '' || value === undefined || value === null
}

// Check if an env var name looks like a secret
const isSecretEnvVar = (name: string): boolean => {
  const secretPatterns = ['TOKEN', 'KEY', 'SECRET', 'PASSWORD', 'CREDENTIAL']
  const upperName = name.toUpperCase()
  return secretPatterns.some(pattern => upperName.includes(pattern))
}

// Get a human-readable label from an env var name
const getEnvVarLabel = (name: string): string => {
  return name
    .replace(/_/g, ' ')
    .toLowerCase()
    .replace(/\b\w/g, c => c.toUpperCase())
}

const SkillConfigurationStep: FC<SkillConfigurationStepProps> = ({
  skills,
  currentSkillIndex,
  totalSkills,
  configuredEnvVars,
  onEnvVarChange,
  oauthConsent,
  onOAuthConsentChange,
  userOAuthConnections,
}) => {
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({})

  // Get all MCPs that need configuration
  const configurableMcps = (skills.mcps || []).filter(mcp => {
    // Has OAuth provider
    if (mcp.oauth_provider) return true
    // Has empty env vars that need values
    if (mcp.env) {
      return Object.entries(mcp.env).some(([_, value]) => needsConfiguration(value))
    }
    return false
  })

  if (configurableMcps.length === 0) {
    return null
  }

  const currentMcp = configurableMcps[currentSkillIndex]
  if (!currentMcp) {
    return null
  }

  const skillName = currentMcp.name || 'Unknown Skill'
  const hasOAuth = !!currentMcp.oauth_provider
  const isOAuthConnected = hasOAuth && userOAuthConnections.some(
    conn => conn.provider?.type?.toLowerCase() === currentMcp.oauth_provider?.toLowerCase()
  )

  // Get env vars that need configuration
  const envVarsToConfig = Object.entries(currentMcp.env || {}).filter(
    ([_, value]) => needsConfiguration(value)
  )

  const toggleSecretVisibility = (envVar: string) => {
    setShowSecrets(prev => ({
      ...prev,
      [envVar]: !prev[envVar],
    }))
  }

  return (
    <Box sx={{ py: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
        <Typography variant="h6">
          Configure: {skillName}
        </Typography>
        <Chip
          label={`${currentSkillIndex + 1} of ${totalSkills}`}
          size="small"
          color="primary"
          variant="outlined"
        />
      </Box>

      {currentMcp.description && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          {currentMcp.description}
        </Typography>
      )}

      {/* OAuth Configuration */}
      {hasOAuth && (
        <Box sx={{ mb: 3 }}>
          {isOAuthConnected ? (
            <Alert severity="success" icon={<CheckCircle />} sx={{ mb: 2 }}>
              Your {currentMcp.oauth_provider} account is connected and will be used for this skill.
            </Alert>
          ) : (
            <Alert severity="warning" sx={{ mb: 2 }}>
              This skill requires a {currentMcp.oauth_provider} connection.
              Please connect your account in OAuth settings first.
            </Alert>
          )}

          {/* OAuth Risk Warning */}
          <Alert severity="warning" icon={<Warning />} sx={{ mb: 2, bgcolor: 'rgba(237, 108, 2, 0.1)' }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
              OAuth Token Sharing Warning
            </Typography>
            <Typography variant="body2" sx={{ mb: 2 }}>
              This skill will use your {currentMcp.oauth_provider} OAuth connection. Please be aware:
            </Typography>
            <Box component="ul" sx={{ pl: 2, m: 0, '& li': { mb: 0.5 } }}>
              <li>
                <Typography variant="body2">
                  This desktop environment may be shared with other team members
                </Typography>
              </li>
              <li>
                <Typography variant="body2">
                  In theory, team members could potentially access your OAuth token
                </Typography>
              </li>
              <li>
                <Typography variant="body2">
                  Only proceed if you trust all team members with access to this agent
                </Typography>
              </li>
            </Box>
          </Alert>

          <FormControlLabel
            control={
              <Checkbox
                checked={oauthConsent[skillName] || false}
                onChange={(e) => onOAuthConsentChange(skillName, e.target.checked)}
                color="primary"
              />
            }
            label={
              <Typography variant="body2">
                I understand and accept these risks
              </Typography>
            }
          />
        </Box>
      )}

      {/* Environment Variables Configuration */}
      {envVarsToConfig.length > 0 && (
        <Box>
          <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>
            Configuration
          </Typography>

          {envVarsToConfig.map(([envVar, defaultValue]) => {
            const isSecret = isSecretEnvVar(envVar)
            const currentValue = configuredEnvVars[skillName]?.[envVar] || ''

            return (
              <TextField
                key={envVar}
                fullWidth
                label={getEnvVarLabel(envVar)}
                value={currentValue}
                onChange={(e) => onEnvVarChange(skillName, envVar, e.target.value)}
                type={isSecret && !showSecrets[envVar] ? 'password' : 'text'}
                placeholder={defaultValue || `Enter ${getEnvVarLabel(envVar).toLowerCase()}`}
                margin="normal"
                InputProps={isSecret ? {
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        aria-label="toggle visibility"
                        onClick={() => toggleSecretVisibility(envVar)}
                        onMouseDown={(e) => e.preventDefault()}
                        edge="end"
                        size="small"
                      >
                        {showSecrets[envVar] ? <VisibilityOff /> : <Visibility />}
                      </IconButton>
                    </InputAdornment>
                  ),
                } : undefined}
                helperText={
                  envVar === 'DRONE_ACCESS_TOKEN' ? (
                    <Link
                      href="https://drone.lukemarsden.net/account"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      Get your Drone CI access token
                    </Link>
                  ) : undefined
                }
              />
            )
          })}
        </Box>
      )}

      {/* Show pre-configured env vars (read-only) */}
      {Object.entries(currentMcp.env || {}).filter(([_, value]) => !needsConfiguration(value)).length > 0 && (
        <Box sx={{ mt: 3 }}>
          <Typography variant="subtitle2" sx={{ mb: 1, color: 'text.secondary' }}>
            Pre-configured Settings
          </Typography>
          {Object.entries(currentMcp.env || {})
            .filter(([_, value]) => !needsConfiguration(value))
            .map(([envVar, value]) => (
              <Typography key={envVar} variant="body2" sx={{ color: 'text.secondary' }}>
                {getEnvVarLabel(envVar)}: {value}
              </Typography>
            ))}
        </Box>
      )}
    </Box>
  )
}

export default SkillConfigurationStep

// Helper to count configurable skills
export const countConfigurableSkills = (skills?: TypesAssistantSkills): number => {
  if (!skills?.mcps) return 0
  return skills.mcps.filter(mcp => {
    if (mcp.oauth_provider) return true
    if (mcp.env) {
      return Object.entries(mcp.env).some(([_, value]) => needsConfiguration(value))
    }
    return false
  }).length
}

// Helper to get configurable MCPs
export const getConfigurableMcps = (skills?: TypesAssistantSkills): TypesAssistantMCP[] => {
  if (!skills?.mcps) return []
  return skills.mcps.filter(mcp => {
    if (mcp.oauth_provider) return true
    if (mcp.env) {
      return Object.entries(mcp.env).some(([_, value]) => needsConfiguration(value))
    }
    return false
  })
}
