import { describe, it, expect } from 'vitest'
import {
  PROVIDER_TYPES,
  matchesProviderType,
  findOAuthConnectionForProvider,
  findOAuthProviderForType,
  mapProviderToRepoType,
  hasRequiredScopes,
} from './oauthProviders'
import { TypesOAuthConnection, TypesOAuthProvider, TypesExternalRepositoryType } from '../api/api'

describe('oauthProviders utilities', () => {
  describe('PROVIDER_TYPES', () => {
    it('should have expected provider type constants', () => {
      expect(PROVIDER_TYPES.GITHUB).toBe('github')
      expect(PROVIDER_TYPES.GITLAB).toBe('gitlab')
      expect(PROVIDER_TYPES.AZURE_DEVOPS).toBe('azure-devops')
      expect(PROVIDER_TYPES.BITBUCKET).toBe('bitbucket')
      expect(PROVIDER_TYPES.CUSTOM).toBe('custom')
    })
  })

  describe('matchesProviderType', () => {
    describe('GitHub matching', () => {
      it('should match exact type "github"', () => {
        expect(matchesProviderType('github', null, 'github')).toBe(true)
        expect(matchesProviderType('GitHub', null, 'github')).toBe(true)
        expect(matchesProviderType('GITHUB', null, 'github')).toBe(true)
      })

      it('should match name containing "github"', () => {
        expect(matchesProviderType(null, 'GitHub Enterprise', 'github')).toBe(true)
        expect(matchesProviderType(null, 'My GitHub', 'github')).toBe(true)
        expect(matchesProviderType('custom', 'GitHub', 'github')).toBe(true)
      })

      it('should not match unrelated providers', () => {
        expect(matchesProviderType('gitlab', null, 'github')).toBe(false)
        expect(matchesProviderType(null, 'GitLab', 'github')).toBe(false)
      })
    })

    describe('GitLab matching', () => {
      it('should match exact type "gitlab"', () => {
        expect(matchesProviderType('gitlab', null, 'gitlab')).toBe(true)
        expect(matchesProviderType('GitLab', null, 'gitlab')).toBe(true)
      })

      it('should match name containing "gitlab"', () => {
        expect(matchesProviderType(null, 'Self-hosted GitLab', 'gitlab')).toBe(true)
        expect(matchesProviderType('custom', 'GitLab', 'gitlab')).toBe(true)
      })
    })

    describe('Azure DevOps matching', () => {
      it('should match "azure-devops" type', () => {
        expect(matchesProviderType('azure-devops', null, 'azure-devops')).toBe(true)
        expect(matchesProviderType('Azure-DevOps', null, 'azure-devops')).toBe(true)
      })

      it('should match "ado" type', () => {
        expect(matchesProviderType('ado', null, 'azure-devops')).toBe(true)
        expect(matchesProviderType('ADO', null, 'azure-devops')).toBe(true)
      })

      it('should match name containing "azure" or "ado"', () => {
        expect(matchesProviderType(null, 'Azure DevOps', 'azure-devops')).toBe(true)
        expect(matchesProviderType(null, 'My ADO Instance', 'azure-devops')).toBe(true)
        expect(matchesProviderType('custom', 'Azure', 'azure-devops')).toBe(true)
      })
    })

    describe('Bitbucket matching', () => {
      it('should match exact type "bitbucket"', () => {
        expect(matchesProviderType('bitbucket', null, 'bitbucket')).toBe(true)
      })

      it('should match name containing "bitbucket"', () => {
        expect(matchesProviderType(null, 'Bitbucket Server', 'bitbucket')).toBe(true)
        expect(matchesProviderType('custom', 'Bitbucket', 'bitbucket')).toBe(true)
      })
    })

    describe('edge cases', () => {
      it('should handle null/undefined values', () => {
        expect(matchesProviderType(null, null, 'github')).toBe(false)
        expect(matchesProviderType(undefined, undefined, 'github')).toBe(false)
        expect(matchesProviderType('', '', 'github')).toBe(false)
      })

      it('should be case-insensitive', () => {
        expect(matchesProviderType('GITHUB', 'GITHUB ENTERPRISE', 'github')).toBe(true)
        expect(matchesProviderType('gitLAB', null, 'gitlab')).toBe(true)
      })
    })
  })

  describe('findOAuthConnectionForProvider', () => {
    const mockConnections: Partial<TypesOAuthConnection>[] = [
      { id: '1', provider: { type: 'github', name: 'GitHub' } },
      { id: '2', provider: { type: 'gitlab', name: 'GitLab' } },
      { id: '3', provider: { type: 'custom', name: 'Azure DevOps' } },
      { id: '4', provider: { type: 'custom', name: 'My GitHub Enterprise' } },
    ]

    it('should find connection by exact type match', () => {
      const result = findOAuthConnectionForProvider(mockConnections as TypesOAuthConnection[], 'github')
      expect(result?.id).toBe('1')
    })

    it('should find connection by name match when type is custom', () => {
      const result = findOAuthConnectionForProvider(mockConnections as TypesOAuthConnection[], 'azure-devops')
      expect(result?.id).toBe('3')
    })

    it('should return first matching connection', () => {
      // Both id=1 and id=4 match github
      const result = findOAuthConnectionForProvider(mockConnections as TypesOAuthConnection[], 'github')
      expect(result?.id).toBe('1') // First match
    })

    it('should return undefined if no connection matches', () => {
      const result = findOAuthConnectionForProvider(mockConnections as TypesOAuthConnection[], 'bitbucket')
      expect(result).toBeUndefined()
    })

    it('should handle undefined connections array', () => {
      const result = findOAuthConnectionForProvider(undefined, 'github')
      expect(result).toBeUndefined()
    })

    it('should handle empty connections array', () => {
      const result = findOAuthConnectionForProvider([], 'github')
      expect(result).toBeUndefined()
    })
  })

  describe('findOAuthProviderForType', () => {
    const mockProviders: Partial<TypesOAuthProvider>[] = [
      { id: '1', type: 'github', name: 'GitHub', enabled: true },
      { id: '2', type: 'gitlab', name: 'GitLab', enabled: false },
      { id: '3', type: 'custom', name: 'Azure DevOps' }, // enabled not set
      { id: '4', type: 'bitbucket', name: 'Bitbucket', enabled: true },
    ]

    it('should find enabled provider by type', () => {
      const result = findOAuthProviderForType(mockProviders as TypesOAuthProvider[], 'github')
      expect(result?.id).toBe('1')
    })

    it('should skip disabled providers', () => {
      const result = findOAuthProviderForType(mockProviders as TypesOAuthProvider[], 'gitlab')
      expect(result).toBeUndefined()
    })

    it('should include providers where enabled is not set', () => {
      const result = findOAuthProviderForType(mockProviders as TypesOAuthProvider[], 'azure-devops')
      expect(result?.id).toBe('3')
    })

    it('should return undefined if no provider matches', () => {
      const providers = [{ id: '1', type: 'github', name: 'GitHub', enabled: true }] as TypesOAuthProvider[]
      const result = findOAuthProviderForType(providers, 'gitlab')
      expect(result).toBeUndefined()
    })

    it('should handle undefined providers array', () => {
      const result = findOAuthProviderForType(undefined, 'github')
      expect(result).toBeUndefined()
    })
  })

  describe('mapProviderToRepoType', () => {
    it('should map github to ExternalRepositoryTypeGitHub', () => {
      expect(mapProviderToRepoType('github')).toBe(TypesExternalRepositoryType.ExternalRepositoryTypeGitHub)
    })

    it('should map gitlab to ExternalRepositoryTypeGitLab', () => {
      expect(mapProviderToRepoType('gitlab')).toBe(TypesExternalRepositoryType.ExternalRepositoryTypeGitLab)
    })

    it('should map azure-devops to ExternalRepositoryTypeADO', () => {
      expect(mapProviderToRepoType('azure-devops')).toBe(TypesExternalRepositoryType.ExternalRepositoryTypeADO)
    })

    it('should map bitbucket to ExternalRepositoryTypeBitbucket', () => {
      expect(mapProviderToRepoType('bitbucket')).toBe(TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket)
    })
  })

  describe('hasRequiredScopes', () => {
    describe('basic scope checking', () => {
      it('should return true when all required scopes are present', () => {
        expect(hasRequiredScopes(['repo', 'read:user'], ['repo'])).toBe(true)
        expect(hasRequiredScopes(['repo', 'read:user', 'user:email'], ['repo', 'read:user'])).toBe(true)
      })

      it('should return false when some required scopes are missing', () => {
        expect(hasRequiredScopes(['read:user'], ['repo', 'read:user'])).toBe(false)
        expect(hasRequiredScopes(['repo'], ['repo', 'admin'])).toBe(false)
      })

      it('should return true for exact scope matches', () => {
        expect(hasRequiredScopes(['repo'], ['repo'])).toBe(true)
      })
    })

    describe('scope prefix matching', () => {
      it('should match scopes with sub-scopes (e.g., "repo:status" matches "repo")', () => {
        expect(hasRequiredScopes(['repo:status'], ['repo'])).toBe(true)
        expect(hasRequiredScopes(['read:user:email'], ['read:user'])).toBe(true)
      })

      it('should not match partial scope names', () => {
        // "repository" should not match "repo" - must start with "repo:"
        expect(hasRequiredScopes(['repository'], ['repo'])).toBe(false)
      })
    })

    describe('edge cases', () => {
      it('should return false when connectionScopes is undefined', () => {
        expect(hasRequiredScopes(undefined, ['repo'])).toBe(false)
      })

      it('should return false when connectionScopes is empty', () => {
        expect(hasRequiredScopes([], ['repo'])).toBe(false)
      })

      it('should return false when requiredScopes is empty', () => {
        // This is intentional - if you don't specify what you need, you don't have it
        expect(hasRequiredScopes(['repo', 'read:user'], [])).toBe(false)
      })

      it('should return false when requiredScopes is undefined', () => {
        expect(hasRequiredScopes(['repo'], undefined as any)).toBe(false)
      })

      it('should handle null in requiredScopes', () => {
        expect(hasRequiredScopes(['repo'], null as any)).toBe(false)
      })
    })
  })
})
