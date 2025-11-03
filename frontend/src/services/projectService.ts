import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesProject, TypesProjectCreateRequest, TypesProjectUpdateRequest, TypesSampleProject, TypesSampleProjectInstantiateRequest, StoreDBGitRepository, TypesBoardSettings, TypesSession } from '../api/api';

// Query keys
export const projectsListQueryKey = () => ['projects'];
export const projectQueryKey = (id: string) => ['project', id];
export const projectRepositoriesQueryKey = (projectId: string) => ['project-repositories', projectId];
export const sampleProjectsListQueryKey = () => ['sample-projects'];
export const sampleProjectQueryKey = (id: string) => ['sample-project', id];
export const boardSettingsQueryKey = () => ['board-settings'];
export const projectExploratorySessionQueryKey = (projectId: string) => ['project-exploratory-session', projectId];

/**
 * Hook to list all projects for the current user
 */
export const useListProjects = () => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesProject[]>({
    queryKey: projectsListQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsList();
      return response.data || [];
    },
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
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

  return useQuery<StoreDBGitRepository[]>({
    queryKey: projectRepositoriesQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsRepositoriesList(projectId);
      return (response.data as StoreDBGitRepository[]) || [];
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
 * Hook to list all sample projects
 */
export const useListSampleProjects = () => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesSampleProject[]>({
    queryKey: sampleProjectsListQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1SampleProjectsV2List();
      return response.data || [];
    },
  });
};

/**
 * Hook to get a specific sample project
 */
export const useGetSampleProject = (sampleId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesSampleProject>({
    queryKey: sampleProjectQueryKey(sampleId),
    queryFn: async () => {
      const response = await apiClient.v1SampleProjectsV2Detail(sampleId);
      return response.data;
    },
    enabled: enabled && !!sampleId,
  });
};

/**
 * Hook to instantiate a sample project
 */
export const useInstantiateSampleProject = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ sampleId, request }: { sampleId: string; request: TypesSampleProjectInstantiateRequest }) => {
      const response = await apiClient.v1SampleProjectsV2InstantiateCreate(sampleId, request);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    },
  });
};

/**
 * Hook to get board settings
 */
export const useGetBoardSettings = () => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesBoardSettings>({
    queryKey: boardSettingsQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1SpecTasksBoardSettingsList();
      return response.data;
    },
  });
};

/**
 * Hook to update board settings
 */
export const useUpdateBoardSettings = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (settings: TypesBoardSettings) => {
      const response = await apiClient.v1SpecTasksBoardSettingsUpdate(settings);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardSettingsQueryKey() });
    },
  });
};

/**
 * Hook to get project exploratory session
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectExploratorySessionQueryKey(projectId) });
    },
  });
};
