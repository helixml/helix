import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import useApi from "../hooks/useApi";

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

export interface AdminOrgsListQuery {
    page?: number;
    per_page?: number;
    query?: string;
}

export const adminOrgsQueryKey = (query?: AdminOrgsListQuery) =>
    query ? ["admin-orgs", query] : ["admin-orgs"];

export function useListAdminOrgs(query?: AdminOrgsListQuery) {
    const api = useApi();
    const apiClient = api.getApiClient();

    return useQuery({
        queryKey: adminOrgsQueryKey(query),
        queryFn: async () => {
            const response = await apiClient.v1AdminOrgsList(query);
            return response.data;
        },
        placeholderData: (previousData) => previousData,
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

export interface SetOrgPlanInput {
    orgId: string;
    plan: string; // "pro" | "free" | "" (clear → derive from Stripe)
}

export interface ActivateTrialInput {
    userId: string;
    // org_id set → activate directly on that owned org's wallet (org screen).
    // Omitted → stash the intent on the user, applied when they create their
    // first org (user screen onboarding flow).
    orgId?: string;
    days?: number;
    credits?: number;
    // plan "pro" grants a PAID plan via a PlanOverride (no Stripe subscription)
    // — for customers who paid out-of-band. "" (default) uses the Stripe trial.
    plan?: string;
}

/**
 * Hook to activate a trial for a user (cloud edition, admin only).
 * With org_id: Stripe trial subscription on that org's wallet immediately.
 * Without: intent stashed on the user, consumed on their first owned org.
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
                org_id: input.orgId,
                plan: input.plan ?? "",
            });
            return response.data;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["users"] });
            queryClient.invalidateQueries({ queryKey: adminOrgsQueryKey() });
        },
    });
}

export interface RevokeTrialInput {
    userId: string;
    // org_id set → cancel that org's trialing subscription (org screen).
    // Omitted → clear any stashed trial intent on the user (user screen).
    orgId?: string;
}

/**
 * Hook to revoke a trial (cloud edition, admin only). With org_id: cancels
 * that org's trialing Stripe subscription and mirrors the cancelled wallet
 * state immediately. Without: clears any stashed trial intent on the user.
 * Paid subscriptions are never cancelled.
 */
export function useAdminRevokeTrial() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (input: RevokeTrialInput) => {
            const response = await apiClient.v1AdminUsersTrialActivateDelete(input.userId, {
                org_id: input.orgId,
            });
            return response.data;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["users"] });
            queryClient.invalidateQueries({ queryKey: adminOrgsQueryKey() });
        },
    });
}

export interface GrantCreditsInput {
    userId: string;
    // org_id set → top up that org's wallet (org screen). Omitted → stash the
    // grant on the user for their first owned org (user screen onboarding flow).
    orgId?: string;
    credits: number;
}

/**
 * Hook to grant credits (cloud edition, admin only). With org_id: tops up
 * that org's wallet regardless of subscription state. Without: grant stashed
 * on the user, applied to their first owned org.
 */
export function useAdminGrantCredits() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (input: GrantCreditsInput) => {
            const response = await apiClient.v1AdminUsersCreditsCreate(input.userId, {
                credits: input.credits,
                org_id: input.orgId,
            });
            return response.data;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["users"] });
            queryClient.invalidateQueries({ queryKey: adminOrgsQueryKey() });
        },
    });
}

export interface OwnedOrgSummary {
    id: string;
    name: string;
    display_name?: string;
}

/**
 * Hook to fetch the organisations a target user owns (cloud edition, admin
 * only). Used by the user-level trial/credit dialogs to route between the
 * stash flow (no orgs yet) and a pointer to the org screen (already owns).
 */
export function useAdminUserOwnedOrgs(userId: string | undefined, enabled: boolean) {
    const api = useApi();
    const apiClient = api.getApiClient();

    return useQuery({
        queryKey: ["admin-user-owned-orgs", userId],
        enabled: !!userId && enabled,
        queryFn: async () => {
            const response = await apiClient.v1AdminUsersOwnedOrgsDetail(userId!);
            return (response.data as OwnedOrgSummary[]) ?? [];
        },
    });
}


/**
 * Hook to set an organization's plan override (cloud edition, admin only).
 * plan "pro"/"free" forces the quota tier independent of Stripe (for customers
 * who paid out-of-band); "" clears the override. Never reverted by a Stripe
 * webhook.
 */
export function useAdminSetOrgPlan() {
    const api = useApi();
    const apiClient = api.getApiClient();
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (input: SetOrgPlanInput) => {
            const response = await apiClient.v1AdminOrgsPlanCreate(input.orgId, { plan: input.plan });
            return response.data;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: adminOrgsQueryKey() });
        },
    });
}
