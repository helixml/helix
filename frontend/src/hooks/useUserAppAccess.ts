import { useState, useEffect } from 'react'
import useApi from './useApi'
import { IUserAppAccess, IUserAppAccessState } from '../types'
import useAccount from './useAccount'

const DEFAULT_ACCESS: IUserAppAccess = {
  can_read: false,
  can_write: false,
  is_admin: false
}

/**
 * Hook to get the current user's access rights for a specific app
 * @param appId - The ID of the app to check access for
 * @returns Object containing access rights and loading state
 */
export const useUserAppAccess = (appId: string | null): IUserAppAccessState => {
  const api = useApi()
  const account = useAccount()
  const [loading, setLoading] = useState<boolean>(false)
  const [error, setError] = useState<string | null>(null)
  const [access, setAccess] = useState<IUserAppAccess>(DEFAULT_ACCESS)

  /**
   * Fetch user access rights for the specified app
   */
  const fetchUserAccess = async () => {
    // Don't attempt to fetch if appId is null or empty
    if (!appId) {
      setAccess(DEFAULT_ACCESS)
      return
    }

    setLoading(true)
    setError(null)
    
    try {
      // Call the new API endpoint to get user access rights
      const response = await api.get<IUserAppAccess>(
        `/api/v1/apps/${appId}/user-access`,
        undefined,
        { snackbar: false } // Suppress snackbar errors
      )
      
      if (response) {
        setAccess(response)
      } else {
        // If response is null, default to no access
        setAccess({
          can_read: false,
          can_write: false,
          is_admin: false
        })
        setError('Failed to get access rights')
      }
    } catch (err) {
      console.error('Error fetching user access:', err)
      setAccess({
        can_read: false,
        can_write: false,
        is_admin: false
      })
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  // Fetch access rights when appId changes
  useEffect(() => {
    if(!appId || !account.user) return
    fetchUserAccess()
  }, [
    appId,
    account.user,
  ])

  // Return access state, loading state, and a function to manually refresh
  return {
    loading,
    error,
    access,
    refresh: fetchUserAccess,
    isAdmin: access?.is_admin || false,
    canWrite: access?.can_write || false,
    canRead: access?.can_read || false
  }
}

export default useUserAppAccess 