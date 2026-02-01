const TOKEN_KEY = 'helix_auth_token'

export const tokenStorage = {
  getToken(): string | null {
    try {
      return localStorage.getItem(TOKEN_KEY)
    } catch {
      return null
    }
  },

  setToken(token: string): void {
    try {
      localStorage.setItem(TOKEN_KEY, token)
    } catch {
      console.warn('[TokenStorage] Failed to save token to localStorage')
    }
  },

  clearToken(): void {
    try {
      localStorage.removeItem(TOKEN_KEY)
    } catch {
      console.warn('[TokenStorage] Failed to clear token from localStorage')
    }
  },
}
