import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Api, TypesSpecTaskUpdateRequest } from '../api/api';
import useApi from '../hooks/useApi';

// Re-export generated types for convenience
export type {
  TypesSpecTask as SpecTask,
  TypesSpecTaskWorkSession as WorkSession,
  TypesSpecTaskZedThread as ZedThread,
  TypesSpecTaskMultiSessionOverviewResponse as MultiSessionOverview,
  TypesZedInstanceStatus,
  TypesSpecTaskImplementationSessionsCreateRequest as ImplementationSessionsCreateRequest,
  TypesSpecTaskImplementationTaskListResponse as ImplementationTaskListResponse,
  TypesSpecTaskMultiSessionOverviewResponse as MultiSessionOverviewResponse,
  TypesSpecTaskUpdateRequest as SpecTaskUpdateRequest,
  TypesZedInstanceEvent as ZedInstanceEvent
} from '../api/api';

// Query keys
const QUERY_KEYS = {
  specTasks: ['spec-tasks'] as const,
  specTask: (id: string) => ['spec-tasks', id] as const,
  taskProgress: (id: string) => ['spec-tasks', id, 'progress'] as const,
  multiSessionOverview: (id: string) => ['spec-tasks', id, 'multi-session-overview'] as const,
  workSessions: (id: string) => ['spec-tasks', id, 'work-sessions'] as const,
  implementationTasks: (id: string) => ['spec-tasks', id, 'implementation-tasks'] as const,
  coordinationLog: (id: string) => ['spec-tasks', id, 'coordination-log'] as const,
  zedInstanceStatus: (id: string) => ['spec-tasks', id, 'zed-instance'] as const,
  sessionHistory: (sessionId: string) => ['work-sessions', sessionId, 'history'] as const,
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

export function useMultiSessionOverview(taskId: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.multiSessionOverview(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksMultiSessionOverviewDetail(taskId);
      return response.data;
    },
    enabled: !!taskId,
  });
}

export function useSpecTaskWorkSessions(taskId: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.workSessions(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksWorkSessionsDetail(taskId);
      return response.data;
    },
    enabled: !!taskId,
  });
}

export function useImplementationTasks(taskId: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.implementationTasks(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksImplementationTasksDetail(taskId);
      return response.data;
    },
    enabled: !!taskId,
  });
}

export function useCoordinationEvents(taskId: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.coordinationLog(taskId),
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecTasksCoordinationLogDetail(taskId);
      return response.data;
    },
    enabled: !!taskId,
    refetchInterval: 5000, // Refresh every 5 seconds for real-time updates
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

export function useSessionHistory(sessionId: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.sessionHistory(sessionId),
    queryFn: async () => {
      const response = await api.getApiClient().v1WorkSessionsHistoryDetail(sessionId);
      return response.data;
    },
    enabled: !!sessionId,
  });
}

// Mutation hooks
export function useCreateImplementationSessions() {
  const api = useApi();
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async ({ taskId, request }: { 
      taskId: string; 
      request: any; // TypesSpecTaskImplementationSessionsCreateRequest
    }) => {
      const response = await api.getApiClient().v1SpecTasksImplementationSessionsCreate(taskId, request);
      return response.data;
    },
    onSuccess: (_, { taskId }) => {
      // Invalidate related queries
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.specTask(taskId) });
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.multiSessionOverview(taskId) });
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.workSessions(taskId) });
    },
  });
}

export function useUpdateSpecTaskStatus() {
  const api = useApi();
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async ({ taskId, status }: { taskId: string; status: string }) => {
      const response = await api.getApiClient().v1SpecTasksUpdate(taskId, { status });
      return response.data;
    },
    onSuccess: (_, { taskId }) => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.specTask(taskId) });
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.multiSessionOverview(taskId) });
    },
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
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.multiSessionOverview(taskId) });
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

export function useRecordSessionHistory() {
  const api = useApi();
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async ({ sessionId, entry }: { 
      sessionId: string; 
      entry: { content: string; timestamp: string; type: string; }
    }) => {
      const response = await api.getApiClient().v1WorkSessionsRecordHistoryCreate(sessionId, entry);
      return response.data;
    },
    onSuccess: (_, { sessionId }) => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.sessionHistory(sessionId) });
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

// Real-time updates hook
export function useSpecTaskRealTimeUpdates(taskId: string) {
  const queryClient = useQueryClient();
  
  // This would typically use WebSocket or Server-Sent Events
  // For now, we'll use polling via the existing queries
  const multiSessionQuery = useMultiSessionOverview(taskId);
  const coordinationQuery = useCoordinationEvents(taskId);
  const zedStatusQuery = useZedInstanceStatus(taskId);
  
  return {
    multiSession: multiSessionQuery.data,
    coordination: coordinationQuery.data,
    zedStatus: zedStatusQuery.data,
    isLoading: multiSessionQuery.isLoading || coordinationQuery.isLoading || zedStatusQuery.isLoading,
    error: multiSessionQuery.error || coordinationQuery.error || zedStatusQuery.error,
  };
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
  useTaskProgress,
  useMultiSessionOverview,
  useSpecTaskWorkSessions,
  useImplementationTasks,
  useCoordinationEvents,
  useZedInstanceStatus,
  useSessionHistory,
  
  // Mutation functions
  useCreateImplementationSessions,
  useUpdateSpecTaskStatus,
  useUpdateSpecTask,
  useApproveSpecTask,
  useRecordSessionHistory,
  useSendZedEvent,
  
  // Real-time updates
  useSpecTaskRealTimeUpdates,
  
  // Helper functions
  getSessionStatusColor,
  getSpecTaskStatusColor,
  formatTimestamp,
  
  // Query keys for external use
  QUERY_KEYS,
};

export default specTaskService;