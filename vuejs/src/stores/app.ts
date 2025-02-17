import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAppStore = defineStore('app', () => {
  const darkMode = ref(false)
  const isAuthenticated = ref(false)
  const user = ref(null)
  const loading = ref(false)

  function setDarkMode(value: boolean) {
    darkMode.value = value
  }

  function setAuthenticated(value: boolean) {
    isAuthenticated.value = value
  }

  function setUser(userData: any) {
    user.value = userData
  }

  function setLoading(value: boolean) {
    loading.value = value
  }

  return {
    // State
    darkMode,
    isAuthenticated,
    user,
    loading,
    // Actions
    setDarkMode,
    setAuthenticated,
    setUser,
    setLoading,
  }
}) 