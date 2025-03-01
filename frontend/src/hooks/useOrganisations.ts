import React, { FC, useState, useCallback } from 'react'
import { TypesOrganization } from '../api/api'
import useApi from './useApi'

export const useOrganizations = () => {
  const [organizations, setOrganizations] = useState<TypesOrganization[]>([])

  const api = useApi()

  const loadData = useCallback(async () => {
    try {      
      const organizationsResult = await api.getApiClient().v1OrganizationsList();
      
      setOrganizations(organizationsResult.data)
    } catch (error) {
      console.error('Error loading organizations:', error)
    }
  }, [api])

  return {
    organizations,
    loadData,
  }
}

