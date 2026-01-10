import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Api, TypesSpecTaskUpdateRequest } from '../api/api';
import useApi from '../hooks/useApi';

// Re-export generated types for convenience
export type {
  TypesSpecTask as SpecTask,
  TypesSpecTaskWorkSession as WorkSession,
  TypesSpecTaskZedThread as ZedThread,  
  TypesZedInstanceStatus,
  TypesSpecTaskUpdateRequest as SpecTaskUpdateRequest,  
  TypesZedInstanceEvent as ZedInstanceEvent,
  TypesCloneTaskRequest as CloneTaskRequest,
  TypesCloneTaskResponse as CloneTaskResponse,
  TypesCloneGroup as CloneGroup,
  TypesCloneGroupProgress as CloneGroupProgress,
} from '../api/api';

// Query keys
const QUERY_KEYS = {
  specTasks: ['spec-tasks'] as const,
  specTask: (id: string) => ['spec-tasks', id] as const,
  specTaskUsage: (id: string) => ['spec-tasks', id, 'usage'] as const,
  taskProgress: (id: string) => ['spec-tasks', id, 'progress'] as const,  
  workSessions: (id: string) => ['spec-tasks', id, 'work-sessions'] as const,
  implementationTasks: (id: string) => ['spec-tasks', id, 'implementation-tasks'] as const,
  coordinationLog: (id: string) => ['spec-tasks', id, 'coordination-log'] as const,
  zedInstanceStatus: (id: string) => ['spec-tasks', id, 'zed-instance'] as const,
  sessionHistory: (sessionId: string) => ['work-sessions', sessionId, 'history'] as const,
  cloneGroups: (taskId: string) => ['spec-tasks', taskId, 'clone-groups'] as const,
  cloneGroupProgress: (groupId: string) => ['clone-groups', groupId, 'progress'] as const,
  reposWithoutProjects: (orgId?: string) => ['repositories', 'without-projects', orgId] as const,
};

// Custom hooks for SpecTask operations
export function useSpecTask(taskId: string, options?: { enabled?: boolean; refetchInterval?: number | false }) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.specTask(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksDetail(taskId);
      return response.data;
    },
    enabled: options?.enabled !== false && !!taskId,
    refetchInterval: options?.refetchInterval !== undefined ? options.refetchInterval : 2000,
  });
}

// Hook to fetch task checklist progress from tasks.md in helix-specs branch
export function useTaskProgress(taskId: string, options?: { enabled?: boolean; refetchInterval?: number }) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.taskProgress(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksProgressDetail(taskId);
      return response.data;
    },
    enabled: options?.enabled !== false && !!taskId,
    refetchInterval: options?.refetchInterval ?? 10000, // Refresh every 10 seconds by default
  });
}

export function useZedInstanceStatus(taskId: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.zedInstanceStatus(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksZedInstanceDetail(taskId);
      return response.data;
    },
    enabled: !!taskId,
    refetchInterval: 10000, // Refresh every 10 seconds
  });
}

export function useSpecTaskUsage(taskId: string, options?: { 
  from?: string; 
  to?: string; 
  aggregationLevel?: 'hourly' | 'daily' | '5min';
  enabled?: boolean;
  refetchInterval?: number | false;
}) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.specTaskUsage(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksUsageDetail(taskId, {
        from: options?.from,
        to: options?.to,
        aggregation_level: options?.aggregationLevel,
      });
      return response.data;
    },
    enabled: options?.enabled !== false && !!taskId,
    refetchInterval: options?.refetchInterval,
  });
}

export function useUpdateSpecTask() {
  const api = useApi();
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async ({ taskId, updates }: { 
      taskId: string; 
      updates: TypesSpecTaskUpdateRequest;
    }) => {
      const response = await api.getApiClient().v1SpecTasksUpdate(taskId, updates);
      return response.data;
    },
    onSuccess: (_, { taskId }) => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.specTask(taskId) });
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.specTasks });
    },
  });
}

export function useApproveSpecTask() {
  const api = useApi();
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async (taskId: string) => {
      const response = await api.getApiClient().v1SpecTasksApproveSpecsCreate(taskId, { 
        approved: true,
        comments: 'Approved via UI'
      });
      return response.data;
    },
    onSuccess: (_, taskId) => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.specTask(taskId) });
    },
  });
}

export function useSendZedEvent() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (event: any) => { // TypesZedInstanceEvent
      const response = await api.getApiClient().v1ZedEventsCreate(event);
      return response.data;
    },
    onSuccess: (_, event) => {
      // Invalidate coordination events for the affected SpecTask
      if (event.spec_task_id) {
        queryClient.invalidateQueries({
          queryKey: QUERY_KEYS.coordinationLog(event.spec_task_id)
        });
      }
    },
  });
}

// Clone-related hooks
export function useCloneGroups(taskId: string) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.cloneGroups(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksCloneGroupsDetail(taskId);
      return response.data;
    },
    enabled: !!taskId,
  });
}

export function useCloneGroupProgress(groupId: string) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.cloneGroupProgress(groupId),
    queryFn: async () => {
      const response = await api.getApiClient().v1CloneGroupsProgressDetail(groupId);
      return response.data;
    },
    enabled: !!groupId,
    refetchInterval: 5000, // Refresh every 5 seconds during cloning
  });
}

export function useReposWithoutProjects(orgId?: string) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.reposWithoutProjects(orgId),
    queryFn: async () => {
      const response = await api.getApiClient().v1RepositoriesWithoutProjectsList({ organization_id: orgId });
      return response.data;
    },
  });
}

export function useCloneTask() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ taskId, request }: {
      taskId: string;
      request: {
        target_project_ids?: string[];
        create_projects?: { repo_id: string; name?: string }[];
        auto_start?: boolean;
      }
    }) => {
      const response = await api.getApiClient().v1SpecTasksCloneCreate(taskId, request);
      return response.data;
    },
    onSuccess: (data, { taskId }) => {
      // Invalidate clone groups for the source task
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.cloneGroups(taskId) });
      // Invalidate spec tasks list to show new cloned tasks
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.specTasks });
    },
  });
}

// Helper functions
export function getSessionStatusColor(status: string): 'success' | 'primary' | 'error' | 'warning' | 'default' {
  switch (status) {
    case 'active':
      return 'success';
    case 'completed':
      return 'primary';
    case 'failed':
    case 'cancelled':
      return 'error';
    case 'blocked':
      return 'warning';
    case 'pending':
    default:
      return 'default';
  }
}

export function getSpecTaskStatusColor(status: string): 'success' | 'primary' | 'error' | 'warning' | 'default' {
  switch (status) {
    case 'active':
    case 'implementing':
      return 'success';
    case 'completed':
      return 'primary';
    case 'failed':
    case 'cancelled':
      return 'error';
    case 'blocked':
    case 'pending_approval':
      return 'warning';
    case 'draft':
    case 'planning':
    default:
      return 'default';
  }
}

export function formatTimestamp(timestamp: string | undefined): string {
  if (!timestamp) return 'N/A';
  return new Date(timestamp).toLocaleString();
}

// Default export for the service
const specTaskService = {
  // Query functions
  useSpecTask,
  useSpecTaskUsage,
  useTaskProgress,  
  useZedInstanceStatus,
  useCloneGroups,
  useCloneGroupProgress,
  useReposWithoutProjects,

  // Mutation functions  
  useUpdateSpecTask,
  useApproveSpecTask,
  useSendZedEvent,
  useCloneTask,  

  // Helper functions
  getSessionStatusColor,
  getSpecTaskStatusColor,
  formatTimestamp,

  // Query keys for external use
  QUERY_KEYS,
};

export default specTaskService;