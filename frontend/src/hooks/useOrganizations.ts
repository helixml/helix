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
  loadOrganization: (id: string) => Promise<void>,
}

export const defaultOrganizationTools: IOrganizationTools = {
  organizations: [],
  loading: false,
  loadOrganizations: async () => {},
  createOrganization: async () => false,
  updateOrganization: async () => false,
  deleteOrganization: async () => false,
  loadOrganization: async () => {},
}

export default function useOrganizations(): IOrganizationTools {
  const [organizations, setOrganizations] = useState<TypesOrganization[]>([])
  const [organization, setOrganization] = useState<TypesOrganization | undefined>(undefined)
  const [loading, setLoading] = useState(false)
  const [initialized, setInitialized] = useState(false)
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const router = useRouter()

  // Extract org_id parameter from router
  const orgIdParam = router.params.org_id

  // Load a single organization with all its details
  const loadOrganization = useCallback(async (id: string) => {
    try {
      setLoading(true)
      // Fetch the organization details
      const orgResult = await api.getApiClient().v1OrganizationsDetail(id)

      // Fetch members for the organization
      const membersResult = await api.getApiClient().v1OrganizationsMembersDetail(id)
      
      // Fetch roles for the organization
      const rolesResult = await api.getApiClient().v1OrganizationsRolesDetail(id)
      
      // Fetch teams for the organization
      const teamsResult = await api.getApiClient().v1OrganizationsTeamsDetail(id)
      
      // Create a complete organization object with all details
      const completeOrg = {
        ...orgResult.data,
        memberships: membersResult.data,
        roles: rolesResult.data,
        teams: teamsResult.data
      }
      
      setOrganization(completeOrg)
    } catch (error) {
      console.error(`Error loading organization ${id}:`, error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || `Error loading organization details`)
      setOrganization(undefined)
    } finally {
      setLoading(false)
    }
  }, [])

  // this is called by the top level account context once we have a login
  // so - we can know that when 'initialized' is true, we have a user
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
      setInitialized(true)
      setLoading(false)
    }
  }, [])

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
  }, [api, loadOrganizations, loadOrganization, orgIdParam, snackbar])

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
  }, [])

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
  }, [])

  // Effect to load organization when orgIdParam changes
  useEffect(() => {
    if (orgIdParam && initialized) {
      const useOrg = organizations.find((org) => org.id === orgIdParam || org.name === orgIdParam)
      if(!useOrg || !useOrg.id) return
      loadOrganization(useOrg.id)
    }
  }, [orgIdParam, initialized])

  return {
    organizations,
    loading,
    organization,
    loadOrganizations,
    createOrganization,
    updateOrganization,
    deleteOrganization,
    loadOrganization,
  }
} 