import React, { FC, useState, useEffect } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import CircularProgress from '@mui/material/CircularProgress'
import InputAdornment from '@mui/material/InputAdornment'

import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import { TypesOrganization } from '../api/api'
import useSnackbar from '../hooks/useSnackbar'
import CopyButton from '../components/common/CopyButton'

const OrgSettings: FC = () => {
  // Get account context and router
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  
  // Form state
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false)
  const [errors, setErrors] = useState<{slug?: string, name?: string}>({})

  const organization = account.organizationTools.organization

  // Generate slug from name for new organizations
  const handleNameBlur = () => {
    // Only auto-generate slug if:
    // 1. Current slug is empty
    // 2. User hasn't manually edited the slug
    if (slug === '' && !slugManuallyEdited && name) {
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

  // Handle form submission
  const handleSubmit = async () => {
    // Validate form before submission
    if (!validateForm()) {
      return
    }
    
    try {
      setLoading(true)
      
      if (!organization || !organization.id) {
        snackbar.error('Organization not found')
        return
      }
      
      // Create the updated organization object
      const updatedOrg = {
        ...organization,
        name: slug,        // 'name' field in API is our 'slug'
        display_name: name // 'display_name' in API is our 'name'
      } as TypesOrganization
      
      await account.organizationTools.updateOrganization(organization.id, updatedOrg)
      snackbar.success('Organization updated successfully')

      if(slug != organization.name) {
        router.navigate('org_settings', {org_id: slug})
      }
    } catch (error) {
      console.error('Error updating organization:', error)
      snackbar.error('Failed to update organization')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (organization) {
      setSlug(organization.name || '')
      setName(organization.display_name || '')
    }
  }, [organization])

  if(!account.user) return null
  // if(!account.isOrgMember) return null

  // Determine if the user can edit the organization settings
  const isReadOnly = !account.isOrgAdmin

  return (
    <Page
      breadcrumbTitle={ organization ? `Settings` : 'Organization Settings' }
      breadcrumbParent={{ title: 'Organizations', routeName: 'orgs', useOrgRouter: false }}
      breadcrumbShowHome={ true }
      orgBreadcrumbs={ true }
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3, p: 2 }}>
          <Typography variant="h5" component="h2" gutterBottom>
            Organization Settings
            {isReadOnly && (
              <Typography variant="caption" color="text.secondary" sx={{ ml: 2 }}>
                (Read-only: Admin privileges required to make changes)
              </Typography>
            )}
          </Typography>
          
          {organization ? (
            <Box component="form" sx={{ mt: 3 }}>
              {/* Name field (formerly Display Name) */}
              <TextField
                label="Name"
                fullWidth
                value={name}
                onChange={(e) => setName(e.target.value)}
                onBlur={handleNameBlur}
                disabled={loading || isReadOnly}
                required
                error={!!errors.name}
                helperText={errors.name || "Human-readable name for the organization"}
                sx={{ mb: 3 }}
                InputProps={{
                  readOnly: isReadOnly,
                }}
              />
              
              {/* Slug field (formerly Name) */}
              <TextField
                label="Slug"
                fullWidth
                value={slug}
                onChange={handleSlugChange}
                disabled={loading || isReadOnly}
                required
                error={!!errors.slug}
                helperText={errors.slug || "Unique identifier for the organization (no spaces allowed)"}
                sx={{ mb: 3 }}
                InputProps={{
                  readOnly: isReadOnly,
                }}
              />
              
              {/* Organization ID field (read-only) */}
              <TextField
                label="Organization ID"
                fullWidth
                value={organization?.id || ''}
                disabled={true}
                helperText="Unique identifier for the organization"
                sx={{ mb: 3 }}
                InputProps={{
                  readOnly: true,
                  endAdornment: (
                    <InputAdornment position="end">
                      <CopyButton 
                        content={organization?.id || ''}
                        title="Organization ID"
                      />
                    </InputAdornment>
                  ),
                }}
              />
              
              {!isReadOnly && (
                <Box sx={{ mt: 3, display: 'flex', justifyContent: 'flex-end' }}>
                  <Button
                    onClick={handleSubmit}
                    variant="outlined"
                    color="secondary"
                    disabled={loading}
                    startIcon={loading ? <CircularProgress size={20} /> : null}
                  >
                    Update Organization
                  </Button>
                </Box>
              )}
            </Box>
          ) : (
            <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
              <CircularProgress />
            </Box>
          )}
        </Box>
      </Container>
    </Page>
  )
}

export default OrgSettings
