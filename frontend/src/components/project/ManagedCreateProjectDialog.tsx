import { FC } from 'react'

import {
  TypesExternalRepositoryType,
  TypesGitRepositoryType,
} from '../../api/api'
import type {
  TypesAzureDevOps,
  TypesGitRepository,
} from '../../api/api'
import useAccount from '../../hooks/useAccount'
import {
  useCreateGitRepository,
  useGitRepositories,
} from '../../services/gitRepositoryService'
import CreateProjectDialog from './CreateProjectDialog'

interface ManagedCreateProjectDialogProps {
  open: boolean
  onClose: () => void
  onSuccess?: (projectId: string) => void
}

const ManagedCreateProjectDialog: FC<ManagedCreateProjectDialogProps> = ({
  open,
  onClose,
  onSuccess,
}) => {
  const account = useAccount()
  const organizationId = account.organizationTools.organization?.id || ''
  const userId = account.user?.id || ''
  const { data: repositories = [], isLoading: repositoriesLoading } =
    useGitRepositories({
      organizationId,
      enabled: open && !!userId && !!organizationId,
    })
  const createGitRepository = useCreateGitRepository()

  const createRepository = async (
    name: string,
    description: string,
  ): Promise<TypesGitRepository | null> => {
    if (!userId || !organizationId) return null

    try {
      const repository = await createGitRepository.mutateAsync({
        name,
        description,
        owner_id: userId,
        organization_id: organizationId,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: 'main',
      })
      return repository || null
    } catch (error) {
      console.error('Failed to create repository:', error)
      return null
    }
  }

  const linkRepository = async (
    url: string,
    name: string,
    type: TypesExternalRepositoryType,
    username?: string,
    password?: string,
    azureDevOps?: TypesAzureDevOps,
    oauthConnectionId?: string,
    gitProviderConnectionId?: string,
  ): Promise<TypesGitRepository | null> => {
    if (!userId || !organizationId) return null

    try {
      const repository = await createGitRepository.mutateAsync({
        name,
        description: `External ${type} repository`,
        owner_id: userId,
        organization_id: organizationId,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: 'main',
        is_external: true,
        external_url: url,
        external_type: type,
        username,
        password,
        azure_devops: azureDevOps,
        oauth_connection_id: oauthConnectionId,
        git_provider_connection_id: gitProviderConnectionId,
      })
      return repository || null
    } catch (error: any) {
      const message = error?.response?.data?.message
        || error?.response?.data
        || error?.message
        || 'Failed to link repository'
      throw new Error(typeof message === 'string' ? message : JSON.stringify(message))
    }
  }

  return (
    <CreateProjectDialog
      open={open}
      onClose={onClose}
      onSuccess={onSuccess}
      repositories={repositories}
      reposLoading={repositoriesLoading}
      onCreateRepo={createRepository}
      onLinkRepo={linkRepository}
    />
  )
}

export default ManagedCreateProjectDialog
