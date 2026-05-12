import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import useApi from "../hooks/useApi";
import { DashboardData } from "../types/dashboard";

export const dashboardQueryKey = () => ["dashboard"];

export function useGetDashboardData() {
    return useQuery({
        queryKey: dashboardQueryKey(),
        queryFn: async (): Promise<DashboardData> => ({ runners: [] }),
        enabled: true,
        staleTime: Infinity,
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
    /** Free-text search across email, username, and full_name (ILIKE). */
    query?: string;
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
    /** Filter by waitlist status (true = only waitlisted, false = only active) */
    waitlisted?: boolean;
    /** Comma-separated list of extras to include (e.g. "trial") */
    include?: string;
}

/**
 * Query key factory for users list with parameters
 * @param query - Optional query parameters for filtering and pagination
 * @returns Query key array for React Query caching
 */
export function usersQueryKey(query?: UserListQuery) {
    return ["users", query];
}

export const adminOrgsQueryKey = () => ["admin-orgs"];

export function useListAdminOrgs() {
    const api = useApi();
    const apiClient = api.getApiClient();

    return useQuery({
        queryKey: adminOrgsQueryKey(),
        queryFn: async () => {
            const response = await apiClient.v1AdminOrgsList();
            return response.data;
        },
    });
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

/**
 * Hook to load per-user admin stats (projects, spec tasks, model usage, last active)
 */
export function useUserStats(userId: string | null | undefined) {
    const api = useApi();
    const apiClient = api.getApiClient();

    return useQuery({
        queryKey: ["user-stats", userId],
        queryFn: async () => {
            const response = await apiClient.v1UsersStatsDetail(userId as string);
            return response.data;
        },
        enabled: Boolean(userId),
    });
}

/**
 * Hook to create a new user (Admin only)
 * @returns React Query mutation for creating a user
 * 
 * @example
 * const createUser = useCreateUser();
 * 
 * // Create a regular user
 * createUser.mutate({
 *   email: 'user@example.com',
 *   password: 'securepassword',
 *   full_name: 'John Doe',
 *   admin: false
 * });
 * 
 * @example
 * // Create an admin user
 * createUser.mutate({
 *   email: 'admin@example.com',
 *   password: 'securepassword',
 *   full_name: 'Admin User',
 *   admin: true
 * });
 */
export function useCreateUser() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (data: {
            email: string;
            password: string;
            full_name?: string;
            admin?: boolean;
        }) => {
            const response = await apiClient.v1UsersCreate(data);
            return response.data;
        },
        onSuccess: () => {
            // Invalidate users list to refresh the UI
            queryClient.invalidateQueries({ queryKey: ["users"] });
        },
    });
}

/**
 * Hook to reset a user's password (Admin only)
 * @returns React Query mutation for resetting a user's password
 *
 * @example
 * const resetPassword = useAdminResetPassword();
 *
 * resetPassword.mutate({
 *   userId: 'user-123',
 *   newPassword: 'newSecurePassword'
 * });
 */
export function useAdminResetPassword() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (data: {
            userId: string;
            newPassword: string;
        }) => {
            const response = await apiClient.v1AdminUsersPasswordUpdate(data.userId, {
                new_password: data.newPassword,
            });
            return response.data;
        },
        onSuccess: () => {
            // Invalidate users list to refresh the UI
            queryClient.invalidateQueries({ queryKey: ["users"] });
        },
    });
}

/**
 * Hook to approve a waitlisted user (Admin only)
 * @returns React Query mutation for approving a user
 *
 * @example
 * const approveUser = useAdminApproveUser();
 *
 * approveUser.mutate('user-123');
 */
export function useAdminApproveUser() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (userId: string) => {
            const response = await apiClient.v1AdminUsersApproveCreate(userId);
            return response.data;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["users"] });
        },
    });
}

/**
 * Hook to delete a user (Admin only)
 * @returns React Query mutation for deleting a user
 *
 * @example
 * const deleteUser = useAdminDeleteUser();
 *
 * deleteUser.mutate('user-123');
 */
export function useAdminDeleteUser() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (userId: string) => {
            const response = await apiClient.v1AdminUsersDelete(userId);
            return response.data;
        },
        onSuccess: () => {
            // Invalidate users list to refresh the UI
            queryClient.invalidateQueries({ queryKey: ["users"] });
        },
    });
}

export interface ActivateTrialInput {
    userId: string;
    days?: number;
    credits?: number;
}

/**
 * Hook to activate a trial on a user (cloud edition, admin only).
 * If the user has no orgs yet, the intent is stashed and applied when they
 * create their first org. Otherwise the Stripe trial subscription is created
 * on the user's oldest owned org wallet immediately.
 */
export function useAdminActivateTrial() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (input: ActivateTrialInput) => {
            const response = await apiClient.v1AdminUsersTrialActivateCreate(input.userId, {
                days: input.days ?? 0,
                credits: input.credits ?? 0,
            });
            return response.data;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["users"] });
        },
    });
}

/**
 * Hook to revoke a trial on a user (cloud edition, admin only).
 * Clears any stashed intent and cancels the Stripe subscription if currently
 * trialing. Paid subscriptions are never cancelled.
 */
export function useAdminRevokeTrial() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (userId: string) => {
            const response = await apiClient.v1AdminUsersTrialActivateDelete(userId);
            return response.data;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["users"] });
        },
    });
}
