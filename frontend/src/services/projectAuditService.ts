import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import { TypesProjectAuditLogResponse, TypesAuditEventType } from '../api/api'

export interface AuditLogFilters {
  eventType?: TypesAuditEventType
  userId?: string
  specTaskId?: string
  startDate?: string
  endDate?: string
  search?: string
  limit?: number
  offset?: number
}

// Query key for audit logs
export const projectAuditLogsQueryKey = (projectId: string, filters?: AuditLogFilters) =>
  ['project-audit-logs', projectId, filters] as const

// Hook to fetch project audit logs
export function useProjectAuditLogs(projectId: string, filters?: AuditLogFilters) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery<TypesProjectAuditLogResponse>({
    queryKey: projectAuditLogsQueryKey(projectId, filters),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsAuditLogsDetail(projectId, {
        event_type: filters?.eventType,
        user_id: filters?.userId,
        spec_task_id: filters?.specTaskId,
        start_date: filters?.startDate,
        end_date: filters?.endDate,
        search: filters?.search,
        limit: filters?.limit,
        offset: filters?.offset,
      })
      return response.data
    },
    enabled: !!projectId,
  })
}

// Format event type for display
export function formatEventType(eventType: string): string {
  const labels: Record<string, string> = {
    task_created: 'Task Created',
    task_cloned: 'Task Cloned',
    task_approved: 'Spec Approved',
    task_completed: 'Task Completed',
    task_archived: 'Task Archived',
    agent_prompt: 'Prompt Sent',
    user_message: 'User Message',
    agent_started: 'Agent Started',
    spec_generated: 'Spec Generated',
    spec_updated: 'Spec Updated',
    review_comment: 'Review Comment',
    review_comment_reply: 'Comment Reply',
    pr_created: 'PR Created',
    pr_merged: 'PR Merged',
    git_push: 'Git Push',
  }
  return labels[eventType] || eventType
}

// Get color for event type
export function getEventTypeColor(eventType: string): 'default' | 'primary' | 'secondary' | 'success' | 'warning' | 'error' | 'info' {
  const colors: Record<string, 'default' | 'primary' | 'secondary' | 'success' | 'warning' | 'error' | 'info'> = {
    task_created: 'primary',
    task_cloned: 'secondary',
    task_approved: 'success',
    task_completed: 'success',
    task_archived: 'default',
    agent_prompt: 'info',
    user_message: 'info',
    agent_started: 'primary',
    spec_generated: 'secondary',
    spec_updated: 'secondary',
    review_comment: 'warning',
    review_comment_reply: 'warning',
    pr_created: 'success',
    pr_merged: 'success',
    git_push: 'default',
  }
  return colors[eventType] || 'default'
}
