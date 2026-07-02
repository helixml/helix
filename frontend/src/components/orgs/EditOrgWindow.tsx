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
import useSnackbar from '../../hooks/useSnackbar'
import BotRuntimeForm, { BotRuntimeValue } from '../helix-org/BotRuntimeForm'

export interface EditOrgWindowProps {
  open: boolean
  org?: TypesOrganization
  onClose: () => void
  onSubmit: (org: TypesOrganization) => Promise<TypesOrganization | null | void>
}

const DEFAULT_BOT_RUNTIME: BotRuntimeValue = {
  runtime: 'claude_code',
  credentials: 'subscription',
  provider: '',
  model: '',
}

// Identity for the starter manager Bot seeded into a new org so its chart
// isn't blank. Created with owner=true so it can hire and manage other Bots.
const STARTER_BOT_CONTENT = `# Root

You are this organization's root manager. Hire, prompt, tool, and organize other Bots to get work done: create Bots for concrete responsibilities, set their identity, wire who reports to whom, and subscribe them to the topics they need. Delegate rather than doing everything yourself.`

const EditOrgWindow: FC<EditOrgWindowProps> = ({
  open,
  org,
  onClose,
  onSubmit,
}) => {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()
  const helixOrgEnabled = account.user?.alpha_features?.includes('helix-org') ?? false

  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false)
  const [errors, setErrors] = useState<{slug?: string, name?: string}>({})

  // Default Bot Runtime is optional at creation: untouched means "write
  // nothing, use the backend defaults" — so an org with no providers or Claude
  // subscription set up can still be created without picking anything.
  const [botDefaults, setBotDefaults] = useState<BotRuntimeValue>(DEFAULT_BOT_RUNTIME)
  const [botDirty, setBotDirty] = useState(false)

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
    setBotDefaults(DEFAULT_BOT_RUNTIME)
    setBotDirty(false)
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

  // Persist the chosen bot-runtime defaults to the freshly-created org. Only
  // called on create, only when the operator actually touched the form, and
  // only writes non-empty keys. Best-effort: a failure here doesn't undo the
  // (already-created) org, so we surface it as a soft warning.
  const persistBotDefaults = async (createdSlug: string) => {
    const client = api.getApiClient()
    const writes: Promise<unknown>[] = [
      client.v1OrgsSettingsUpdate('worker.runtime', createdSlug, { value: JSON.stringify(botDefaults.runtime) }),
      client.v1OrgsSettingsUpdate('worker.credentials', createdSlug, { value: JSON.stringify(botDefaults.credentials) }),
    ]
    if (botDefaults.provider) {
      writes.push(client.v1OrgsSettingsUpdate('worker.provider', createdSlug, { value: JSON.stringify(botDefaults.provider) }))
    }
    if (botDefaults.model) {
      writes.push(client.v1OrgsSettingsUpdate('worker.model', createdSlug, { value: JSON.stringify(botDefaults.model) }))
    }
    await Promise.all(writes)
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

      // New org + operator picked bot-runtime defaults → persist them against
      // the real created slug (the backend may have suffixed it for uniqueness).
      if (!org && botDirty && helixOrgEnabled && created && created.name) {
        try {
          await persistBotDefaults(created.name)
        } catch (e) {
          snackbar.error('Organization created, but saving the default bot runtime failed — set it in Settings.')
        }
      }

      // New alpha org → seed a starter manager Bot so the org chart isn't
      // blank. It gets the owner tool set (can hire + manage other Bots) and
      // inherits the org's default runtime. Best-effort.
      if (!org && helixOrgEnabled && created && created.name) {
        try {
          await api.getApiClient().v1OrgsBotsCreate(created.name, {
            id: 'b-root',
            content: STARTER_BOT_CONTENT,
            owner: true,
          })
        } catch (e) {
          snackbar.error('Organization created, but seeding the starter bot failed — add one from the Org Chart.')
        }
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

          {/* Default Bot Runtime — only when creating, and only for helix-org
              alpha users. Optional: leave untouched to configure later. */}
          {!org && helixOrgEnabled && (
            <Box sx={{ mt: 3 }}>
              <Divider sx={{ mb: 2 }} />
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                Default Bot Runtime
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Optional — how Bots in this org run by default. Leave as-is to configure
                later in Settings.
              </Typography>
              <BotRuntimeForm
                value={botDefaults}
                onChange={(patch) => {
                  setBotDefaults((v) => ({ ...v, ...patch }))
                  setBotDirty(true)
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
