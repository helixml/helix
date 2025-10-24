import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import useApi from "../hooks/useApi";

export const dashboardQueryKey = () => ["dashboard"];

export function useGetDashboardData() {
    const api = useApi();
    const apiClient = api.getApiClient();

    return useQuery({
        queryKey: dashboardQueryKey(),
        queryFn: async () => {
            const result = await apiClient.v1DashboardList();
            return result.data;
        },
        enabled: true,
        staleTime: 1000, // 1 second - matches backend update intervals
        refetchInterval: 1000, // Refetch every 1 second - matches backend runner cache and reconcile intervals
    });
}

export function useDeleteSlot() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (slotId: string) => {
            const response = await apiClient.v1SlotsDelete(slotId);
            return response.data;
        },
        onSuccess: () => {
            // Invalidate dashboard data to refresh the UI
            queryClient.invalidateQueries({ queryKey: dashboardQueryKey() });
        },
    });
}

/**
 * User list query parameters interface
 * Supports pagination and filtering options
 */
export interface UserListQuery {
    /** Page number (default: 1) */
    page?: number;
    /** Number of users per page (max: 200, default: 50) */
    per_page?: number;
    /** Filter by email domain (e.g., 'hotmail.com') or exact email */
    email?: string;
    /** Filter by username (partial match) */
    username?: string;
    /** Filter by admin status */
    admin?: boolean;
    /** Filter by user type */
    type?: string;
    /** Filter by token type */
    token_type?: string;
}

/**
 * Query key factory for users list with parameters
 * @param query - Optional query parameters for filtering and pagination
 * @returns Query key array for React Query caching
 */
export function usersQueryKey(query?: UserListQuery) {
    return ["users", query];
}

/**
 * Hook to fetch users list with pagination and search support
 * @param query - Optional query parameters for filtering and pagination
 * @returns React Query result with paginated users data
 * 
 * @example
 * // Basic usage - get first page with default settings
 * const { data, isLoading, error } = useListUsers();
 * 
 * @example
 * // With pagination
 * const { data, isLoading, error } = useListUsers({ page: 2, per_page: 25 });
 * 
 * @example
 * // With search filters
 * const { data, isLoading, error } = useListUsers({ 
 *   username: 'john', 
 *   admin: true,
 *   page: 1,
 *   per_page: 50 
 * });
 */
export function useListUsers(query?: UserListQuery) {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useQuery({
        queryKey: usersQueryKey(query),
        queryFn: async () => {
            const response = await apiClient.v1UsersList(query);
            return response.data;
        },
        placeholderData: (previousData) => previousData, // Keep previous data while fetching new page
    });
}