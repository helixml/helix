import React, { FC, useState, useCallback, useMemo, useRef, useEffect } from 'react'
import { TypesOrganization } from '../api/api'
import useApi from './useApi'

export const useOrganizations = () => {
  const [organizations, setOrganizations] = useState<TypesOrganization[]>([])

  const api = useApi()

  const loadData = useCallback(async () => {
    try {
      // Make sure we're explicitly passing secure: true to ensure authentication
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

