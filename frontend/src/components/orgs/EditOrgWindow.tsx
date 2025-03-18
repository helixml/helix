import React, { FC, useState, useEffect } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Box from '@mui/material/Box'

import { TypesOrganization } from '../../api/api'

export interface EditOrgWindowProps {
  open: boolean
  org?: TypesOrganization
  onClose: () => void
  onSubmit: (org: TypesOrganization) => Promise<void>
}

const EditOrgWindow: FC<EditOrgWindowProps> = ({
  open,
  org,
  onClose,
  onSubmit,
}) => {
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false)
  const [errors, setErrors] = useState<{slug?: string, name?: string}>({})

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
      
      await onSubmit(updatedOrg)
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