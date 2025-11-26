import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const WOLF_HEALTH_QUERY_KEY = (sandboxInstanceId: string) => ['wolf-health', sandboxInstanceId];
export const WOLF_KEYBOARD_STATE_QUERY_KEY = (sandboxInstanceId: string) => ['wolf-keyboard-state', sandboxInstanceId];

/**
 * useWolfHealth - Get Wolf system health including thread heartbeat status
 * Returns thread heartbeat information and deadlock detection status
 */
export function useWolfHealth(options: {
  sandboxInstanceId: string;
  enabled?: boolean;
  refetchInterval?: number | false;
}) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: WOLF_HEALTH_QUERY_KEY(options.sandboxInstanceId),
    queryFn: async () => {
      if (!options.sandboxInstanceId) return null
      const result = await apiClient.v1WolfHealthList({ wolf_instance_id: options.sandboxInstanceId })
      // The generated client returns Axios response, need to extract .data
      return result.data
    },
    // Poll every 5 seconds for live monitoring
    // React Query waits for request to complete before starting interval timer
    // So if pipeline test times out (6s), actual cadence is 11s (no pileup)
    refetchInterval: options?.refetchInterval ?? 5000,
    enabled: (options?.enabled ?? true) && !!options.sandboxInstanceId,
    // Don't retry on error - if Wolf is down, retrying won't help
    retry: false,
    // Keep data fresh - pipeline health check is fast (~1-100ms normally, 6s max if deadlocked)
    staleTime: 1000,
  })
}

// Types for keyboard state (matching Go types)
export interface KeyboardModifierState {
  shift: boolean;
  ctrl: boolean;
  alt: boolean;
  meta: boolean;
}

export interface SessionKeyboardState {
  session_id: string;
  timestamp_ms: number;
  pressed_keys: number[];
  pressed_key_names: string[];
  modifier_state: KeyboardModifierState;
  device_name: string;
}

export interface KeyboardStateResponse {
  sessions: SessionKeyboardState[];
}

export interface KeyboardResetResponse {
  session_id: string;
  released_keys: number[];
  released_key_names: string[];
  success: boolean;
}

/**
 * useWolfKeyboardState - Get Wolf keyboard state for all sessions
 * Returns currently pressed keys and modifier state for debugging stuck keys
 */
export function useWolfKeyboardState(options: {
  sandboxInstanceId: string;
  enabled?: boolean;
  refetchInterval?: number | false;
}) {
  const api = useApi()

  return useQuery({
    queryKey: WOLF_KEYBOARD_STATE_QUERY_KEY(options.sandboxInstanceId),
    queryFn: async (): Promise<KeyboardStateResponse | null> => {
      if (!options.sandboxInstanceId) return null
      // Use fetch directly since generated client might not have this endpoint yet
      const response = await fetch(
        `/api/v1/wolf/keyboard-state?wolf_instance_id=${encodeURIComponent(options.sandboxInstanceId)}`,
        {
          headers: {
            'Authorization': `Bearer ${api.getToken()}`,
          },
        }
      )
      if (!response.ok) {
        throw new Error(`Failed to fetch keyboard state: ${response.statusText}`)
      }
      return response.json()
    },
    // Poll every 500ms for responsive visualization
    refetchInterval: options?.refetchInterval ?? 500,
    enabled: (options?.enabled ?? true) && !!options.sandboxInstanceId,
    retry: false,
    staleTime: 100,
  })
}

/**
 * useResetWolfKeyboardState - Reset keyboard state for a session (release stuck keys)
 */
export function useResetWolfKeyboardState(sandboxInstanceId: string) {
  const api = useApi()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (sessionId: string): Promise<KeyboardResetResponse> => {
      const response = await fetch(
        `/api/v1/wolf/keyboard-state/reset?wolf_instance_id=${encodeURIComponent(sandboxInstanceId)}&session_id=${encodeURIComponent(sessionId)}`,
        {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${api.getToken()}`,
          },
        }
      )
      if (!response.ok) {
        throw new Error(`Failed to reset keyboard state: ${response.statusText}`)
      }
      return response.json()
    },
    onSuccess: () => {
      // Invalidate keyboard state to refresh after reset
      queryClient.invalidateQueries({ queryKey: WOLF_KEYBOARD_STATE_QUERY_KEY(sandboxInstanceId) })
    },
  })
}
