import { useState, useCallback, useEffect } from 'react'
import { TypesOrganization } from '../api/api'
import useApi from './useApi'
import useSnackbar from './useSnackbar'

export default function useOrganizations() {
  const [organizations, setOrganizations] = useState<TypesOrganization[]>([])
  const [loading, setLoading] = useState(false)
  const api = useApi()
  const snackbar = useSnackbar()

  const loadOrganizations = useCallback(async () => {
    try {
      setLoading(true)
      const result = await api.getApiClient().v1OrganizationsList()
      setOrganizations(result.data)
    } catch (error) {
      console.error(error)
      snackbar.error('Error loading organizations')
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
      snackbar.error('Error creating organization')
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
      snackbar.error('Error updating organization')
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
      snackbar.error('Error deleting organization')
      return false
    }
  }, [loadOrganizations])

  useEffect(() => {
    loadOrganizations()
  }, [])

  return {
    organizations,
    loading,
    loadOrganizations,
    createOrganization,
    updateOrganization,
    deleteOrganization,
  }
} 