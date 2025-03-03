import { useState, useCallback, useEffect } from 'react'
import { TypesOrganization } from '../api/api'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'
import { extractErrorMessage } from './useErrorCallback'

export interface IOrganizationTools {
  organizations: TypesOrganization[],
  loading: boolean,
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

  const loadOrganizations = useCallback(async () => {
    try {
      setLoading(true)
      const result = await api.getApiClient().v1OrganizationsList()
      setOrganizations(result.data)
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error loading organizations')
    } finally {
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
    loadOrganizations,
    createOrganization,
    updateOrganization,
    deleteOrganization,
  }
} 