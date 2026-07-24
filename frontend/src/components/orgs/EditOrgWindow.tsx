import React, { FC, useState, useEffect } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import Typography from '@mui/material/Typography'

import { TypesOrganization } from '../../api/api'
import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import AgentConfigForm, { AgentConfigValue } from '../helix-org/BotRuntimeForm'
import { SELECTED_ORG_STORAGE_KEY } from '../../utils/localStorage'
import { orgLandingRoute } from '../../utils/organizations'

export interface EditOrgWindowProps {
  open: boolean
  org?: TypesOrganization
  onClose: () => void
  onSubmit: (org: TypesOrganization) => Promise<TypesOrganization | null | void>
}

const DEFAULT_AGENT_CONFIG: AgentConfigValue = {
  runtime: 'claude_code',
  credentials: 'subscription',
  provider: '',
  model: '',
}

const EditOrgWindow: FC<EditOrgWindowProps> = ({
  open,
  org,
  onClose,
  onSubmit,
}) => {
  const account = useAccount()
  const api = useApi()
  const router = useRouter()
  const snackbar = useSnackbar()
  const helixOrgEnabled = account.user?.alpha_features?.includes('helix-org') ?? false

  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false)
  const [errors, setErrors] = useState<{slug?: string, name?: string}>({})

  // Default Agent Configuration is optional at creation: untouched means "write
  // nothing, use the backend defaults" - so an org with no providers or Claude
  // subscription set up can still be created without picking anything.
  const [agentDefaults, setAgentDefaults] = useState<AgentConfigValue>(DEFAULT_AGENT_CONFIG)
  const [agentDefaultsDirty, setAgentDefaultsDirty] = useState(false)

  // Reset state when modal opens/closes or org changes
  useEffect(() => {
    if (org) {
      setSlug(org.name || '')
      setName(org.display_name || '')
    } else {
      setSlug('')
      setName('')
    }
    setSlugManuallyEdited(false)
    setErrors({})
    setAgentDefaults(DEFAULT_AGENT_CONFIG)
    setAgentDefaultsDirty(false)
  }, [org, open])

  // Generate slug from name for new organizations
  const handleNameBlur = () => {
    // Only auto-generate slug if:
    // 1. Creating a new org (not editing)
    // 2. Current slug is empty
    // 3. User hasn't manually edited the slug
    if (!org && slug === '' && !slugManuallyEdited && name) {
      // Convert name to slug format: lowercase, replace spaces with hyphens
      const generatedSlug = name.toLowerCase().replace(/\s+/g, '-')
      setSlug(generatedSlug)
    }
  }

  // Mark slug as manually edited when user changes it
  const handleSlugChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSlugManuallyEdited(true)
    setSlug(e.target.value)
  }

  // Validate form before submission
  const validateForm = () => {
    const newErrors: {slug?: string, name?: string} = {}

    // Validate name (required)
    if (!name) {
      newErrors.name = 'Name is required'
    }

    // Validate slug (required and no spaces)
    if (!slug) {
      newErrors.slug = 'Slug is required'
    } else if (slug.includes(' ')) {
      newErrors.slug = 'Slug cannot contain spaces'
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  // Persist the chosen default agent config to the freshly-created org. Only
  // called on create, only when the operator actually touched the form, and
  // writes the object once. Best-effort: a failure here doesn't undo the
  // (already-created) org, so we surface it as a soft warning.
  const persistAgentDefaults = async (createdSlug: string) => {
    const client = api.getApiClient()
    await client.v1OrgsSettingsUpdate('agent.default', createdSlug, { value: JSON.stringify({
      code_agent_runtime: agentDefaults.runtime,
      code_agent_credential_type: agentDefaults.credentials,
      provider: agentDefaults.provider,
      model: agentDefaults.model,
    }) })
  }

  const handleSubmit = async () => {
    // Validate form before submission
    if (!validateForm()) {
      return
    }

    try {
      setLoading(true)

      // Create the updated organization object
      // If editing existing org, merge with original data
      // If creating new org, create minimal object
      const updatedOrg = org
        ? {
            ...org,            // Preserve all existing fields
            name: slug,        // Update the slug
            display_name: name // Update the display name
          }
        : {
            name: slug,        // 'name' field in API is our 'slug'
            display_name: name // 'display_name' in API is our 'name'
          } as TypesOrganization;

      const created = await onSubmit(updatedOrg)

      // New org + operator picked a default agent config: persist it against
      // the real created slug (the backend may have suffixed it for uniqueness).
      if (!org && agentDefaultsDirty && helixOrgEnabled && created && created.name) {
        try {
          await persistAgentDefaults(created.name)
        } catch (e) {
          snackbar.error('Organization created, but saving the default agent configuration failed - set it in Settings.')
        }
      }

      // Chief of Staff (+ the creator's human node) is now seeded by the
      // backend on org create — see api/pkg/server/org_graph_seed.go. The
      // frontend no longer creates it.

      // Land the operator on the freshly created org: the org chart when
      // helix-org is enabled, otherwise projects.
      if (!org && created && created.name) {
        localStorage.setItem(SELECTED_ORG_STORAGE_KEY, created.name)
        router.navigate(orgLandingRoute(account.user), { org_id: created.name })
      }

      onClose()
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>
        {org ? 'Edit Organization' : 'Create Organization'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          {/* Name field (formerly Display Name) */}
          <TextField
            label="Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={handleNameBlur}
            disabled={loading}
            required
            error={!!errors.name}
            helperText={errors.name || "Human-readable name for the organization"}
            sx={{ mb: 2 }}
          />

          {/* Slug field (formerly Name) */}
          <TextField
            label="Slug"
            fullWidth
            value={slug}
            onChange={handleSlugChange}
            disabled={loading}
            required
            error={!!errors.slug}
            helperText={errors.slug || "Unique identifier for the organization (no spaces allowed)"}
          />

          {/* Default Agent Configuration - only when creating, and only for helix-org
              alpha users. Optional: leave untouched to configure later. */}
          {!org && helixOrgEnabled && (
            <Box sx={{ mt: 3 }}>
              <Divider sx={{ mb: 2 }} />
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                Default Agent Configuration
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Optional agent settings copied to Bots when they are first provisioned.
              </Typography>
              <AgentConfigForm
                value={agentDefaults}
                onChange={(patch) => {
                  setAgentDefaults((v) => ({ ...v, ...patch }))
                  setAgentDefaultsDirty(true)
                }}
              />
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button
          onClick={onClose}
          disabled={loading}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          color="primary"
          disabled={loading}
        >
          {org ? 'Update' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default EditOrgWindow
