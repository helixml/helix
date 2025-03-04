import { useState, useCallback, useEffect, useMemo } from 'react'
import { TypesOrganization, TypesOrganizationMembership } from '../api/api'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'
import useRouter from './useRouter'
import { extractErrorMessage } from './useErrorCallback'
import bluebird from 'bluebird'

export interface IOrganizationTools {
  organizations: TypesOrganization[],
  loading: boolean,
  organization?: TypesOrganization,
  loadOrganizations: () => Promise<void>,
  createOrganization: (org: TypesOrganization) => Promise<boolean>,
  updateOrganization: (id: string, org: TypesOrganization) => Promise<boolean>,
  deleteOrganization: (id: string) => Promise<boolean>,
}

export const defaultOrganizationTools: IOrganizationTools = {
  organizations: [],
  loading: false,
  loadOrganizations: async () => {},
  createOrganization: async () => false,
  updateOrganization: async () => false,
  deleteOrganization: async () => false,
}

export default function useOrganizations(): IOrganizationTools {
  const [organizations, setOrganizations] = useState<TypesOrganization[]>([])
  const [loading, setLoading] = useState(false)
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const router = useRouter()

  // Extract org_id parameter from router
  const orgIdParam = router.params.org_id

  // Find the current organization based on org_id parameter (which can be ID or name)
  const organization = useMemo(() => {
    if (!orgIdParam || organizations.length === 0) return undefined
    
    // Try to find by ID first
    let org = organizations.find(o => o.id === orgIdParam)
    
    // If not found by ID, try to find by name (slug)
    if (!org) {
      org = organizations.find(o => o.name === orgIdParam)
    }
    
    return org
  }, [organizations, orgIdParam])

  const loadOrganizations = useCallback(async () => {
    try {
      setLoading(true)
      const result = await api.getApiClient().v1OrganizationsList()
      
      // Fetch members for each organization in parallel
      const orgsWithMembers = await bluebird.map(result.data, async (org) => {
        try {
          // Only fetch members if org has an ID
          if (org.id) {
            // Call the API to get members for this organization
            const membersResult = await api.getApiClient().v1OrganizationsMembersDetail(org.id)
            // Create a new object with the members field populated
            return {
              ...org,
              memberships: membersResult.data
            }
          }
          return org
        } catch (error) {
          console.error(`Error fetching members for organization ${org.id}:`, error)
          // Return the original org if there was an error fetching members
          return org
        }
      })

      // Sort organizations by display_name (or name if display_name is not available)
      const sortedOrgs = [...orgsWithMembers].sort((a, b) => {
        const aName = (a.display_name || a.name || '').toLowerCase()
        const bName = (b.display_name || b.name || '').toLowerCase()
        return aName.localeCompare(bName)
      })

      setOrganizations(sortedOrgs)
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error loading organizations')
    } finally {
      setLoading(false)
    }
  }, [api])

  const createOrganization = useCallback(async (org: TypesOrganization) => {
    try {
      await api.getApiClient().v1OrganizationsCreate(org)
      snackbar.success('Organization created')
      await loadOrganizations()
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error creating organization')
      return false
    }
  }, [loadOrganizations])

  const updateOrganization = useCallback(async (id: string, org: TypesOrganization) => {
    try {
      await api.getApiClient().v1OrganizationsUpdate(id, org)
      snackbar.success('Organization updated')
      await loadOrganizations()
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error updating organization')
      return false
    }
  }, [loadOrganizations])

  const deleteOrganization = useCallback(async (id: string) => {
    try {
      await api.getApiClient().v1OrganizationsDelete(id)
      snackbar.success('Organization deleted')
      await loadOrganizations()
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error deleting organization')
      return false
    }
  }, [loadOrganizations])

  return {
    organizations,
    loading,
    organization,
    loadOrganizations,
    createOrganization,
    updateOrganization,
    deleteOrganization,
  }
} 