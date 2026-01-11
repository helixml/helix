import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesProject, TypesProjectCreateRequest, TypesProjectUpdateRequest, TypesBoardSettings, TypesSession, ServicesStartupScriptVersion, TypesGitRepository, TypesForkSimpleProjectRequest, TypesGuidelinesHistory } from '../api/api';

// Query keys
export const projectsListQueryKey = (orgId?: string) => ['projects', orgId];
export const projectQueryKey = (id: string) => ['project', id];
export const projectRepositoriesQueryKey = (projectId: string) => ['project-repositories', projectId];
export const sampleProjectsListQueryKey = () => ['sample-projects'];
export const sampleProjectQueryKey = (id: string) => ['sample-project', id];
export const projectExploratorySessionQueryKey = (projectId: string) => ['project-exploratory-session', projectId];
export const projectStartupScriptHistoryQueryKey = (projectId: string) => ['project-startup-script-history', projectId];
export const projectGuidelinesHistoryQueryKey = (projectId: string) => ['project-guidelines-history', projectId];

/**
 * Hook to list all projects for the current user
 */
export const useListProjects = (orgId?: string, options?: { enabled?: boolean }) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesProject[]>({
    queryKey: projectsListQueryKey(orgId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsList({ organization_id: orgId || undefined });
      return response.data || [];
    },
    enabled: options?.enabled ?? true,
  });
};

/**
 * Hook to get a specific project by ID
 */
export const useGetProject = (projectId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesProject>({
    queryKey: projectQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsDetail(projectId);
      return response.data;
    },
    enabled: enabled && !!projectId,
  });
};

/**
 * Hook to create a new project
 */
export const useCreateProject = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: TypesProjectCreateRequest) => {
      const response = await apiClient.v1ProjectsCreate(request);
      return response.data;
    },
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey(variables.organization_id) });
    },
  });
};

/**
 * Hook to update a project
 */
export const useUpdateProject = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: TypesProjectUpdateRequest) => {
      const response = await apiClient.v1ProjectsUpdate(projectId, request);
      return response.data;
    },
    onSuccess: () => {
      // Standard React Query pattern: invalidate to refetch latest data
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    },
  });
};

/**
 * Hook to delete a project
 */
export const useDeleteProject = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (projectId: string) => {
      const response = await apiClient.v1ProjectsDelete(projectId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    },
  });
};

/**
 * Hook to get repositories for a project
 */
export const useGetProjectRepositories = (projectId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesGitRepository[]>({
    queryKey: projectRepositoriesQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.getProjectRepositories(projectId);
      return (response.data as TypesGitRepository[]) || [];
    },
    enabled: enabled && !!projectId,
  });
};

/**
 * Hook to set a repository as primary for a project
 */
export const useSetProjectPrimaryRepository = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (repoId: string) => {
      const response = await apiClient.v1ProjectsRepositoriesPrimaryUpdate(projectId, repoId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectRepositoriesQueryKey(projectId) });
    },
  });
};

/**
 * Hook to attach a repository to a project
 */
export const useAttachRepositoryToProject = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (repoId: string) => {
      const response = await apiClient.v1ProjectsRepositoriesAttachUpdate(projectId, repoId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectRepositoriesQueryKey(projectId) });
    },
  });
};

/**
 * Hook to detach a repository from a project
 */
export const useDetachRepositoryFromProject = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (repoId: string) => {
      const response = await apiClient.v1ProjectsRepositoriesDetachUpdate(projectId, repoId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectRepositoriesQueryKey(projectId) });
    },
  });
};

/**
 * Hook to list all sample projects (using simple in-memory list)
 */
export const useListSampleProjects = (options?: { enabled?: boolean }) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<any[]>({
    queryKey: sampleProjectsListQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1SampleProjectsSimpleList();
      return response.data || [];
    },
    enabled: options?.enabled ?? true,
  });
};

/**
 * Hook to instantiate a sample project (fork from simple in-memory list)
 */
export const useInstantiateSampleProject = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ sampleId, request }: { sampleId: string; request: TypesForkSimpleProjectRequest }) => {
      const response = await apiClient.v1SampleProjectsSimpleForkCreate({
        sample_project_id: sampleId,
        project_name: request.project_name,
        description: request.description,
        organization_id: request.organization_id,
        helix_app_id: request.helix_app_id,
      });
      return response.data;
    },
    onSuccess: (_data, variables) => {
      // Invalidate the specific org's project list (or personal if no org)
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey(variables.request.organization_id) });
    },
  });
};

/**
 * Hook to get project exploratory session
 * Polls every 5 seconds to keep session status up to date (sessions can stop/crash in background)
 */
export const useGetProjectExploratorySession = (projectId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesSession | null>({
    queryKey: projectExploratorySessionQueryKey(projectId),
    queryFn: async () => {
      try {
        const response = await apiClient.v1ProjectsExploratorySessionDetail(projectId);
        return response.data || null;
      } catch (err: any) {
        // 204 No Content means no session exists
        if (err?.response?.status === 204) {
          return null;
        }
        throw err;
      }
    },
    enabled: enabled && !!projectId,
    refetchInterval: 5000, // Poll every 5 seconds for real-time session status updates
  });
};

/**
 * Hook to start project exploratory session
 */
export const useStartProjectExploratorySession = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1ProjectsExploratorySessionCreate(projectId);
      return response.data;
    },
    onSuccess: async () => {
      // Wait for refetch to complete before resolving mutation
      // This ensures floating modal has fresh session data
      await queryClient.refetchQueries({ queryKey: projectExploratorySessionQueryKey(projectId) });
    },
  });
};

/**
 * Hook to stop project exploratory session
 */
export const useStopProjectExploratorySession = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const response = await apiClient.v1ProjectsExploratorySessionDelete(projectId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectExploratorySessionQueryKey(projectId) });
    },
  });
};

/**
 * Hook to resume project exploratory session
 */
export const useResumeProjectExploratorySession = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      // Get the current session first to get the session ID
      const sessionResponse = await apiClient.v1ProjectsExploratorySessionDetail(projectId);
      const session = sessionResponse.data;

      if (!session?.id) {
        throw new Error('No session found to resume');
      }

      // Call the resume endpoint
      const response = await apiClient.v1SessionsResumeCreate(session.id);
      return session;
    },
    onSuccess: async () => {
      // Wait for refetch to complete before resolving mutation
      // This ensures floating modal has fresh session data
      await queryClient.refetchQueries({ queryKey: projectExploratorySessionQueryKey(projectId) });
    },
  });
};

/**
 * Hook to get startup script version history from git commits
 */
export const useGetStartupScriptHistory = (projectId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<ServicesStartupScriptVersion[]>({
    queryKey: projectStartupScriptHistoryQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsStartupScriptHistoryDetail(projectId);
      return response.data || [];
    },
    enabled: enabled && !!projectId,
  });
};

/**
 * Hook to get project guidelines version history
 */
export const useGetProjectGuidelinesHistory = (projectId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesGuidelinesHistory[]>({
    queryKey: projectGuidelinesHistoryQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsGuidelinesHistoryDetail(projectId);
      return response.data || [];
    },
    enabled: enabled && !!projectId,
  });
};
