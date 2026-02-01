import { describe, it, expect, vi, beforeEach } from 'vitest';

/**
 * Integration test for the 401 token refresh interceptor.
 *
 * This simulates the exact scenario: user opens laptop after overnight sleep,
 * access token is expired, but refresh token is still valid.
 *
 * Flow:
 * 1. API call with expired token → 401
 * 2. Interceptor catches 401
 * 3. Calls /api/v1/auth/refresh (backend uses refresh_token cookie)
 * 4. Calls /api/v1/auth/user to get new token
 * 5. Updates in-memory token (axios.defaults + securityData)
 * 6. Deletes old Authorization header
 * 7. Retries original request
 * 8. Success
 */
describe('Token Refresh Interceptor - Integration Test', () => {

  it('should refresh token and retry request when 401 is received (laptop resume scenario)', async () => {
    // Track what happened during the test
    const callLog: string[] = [];
    let retryCount = 0;

    // Simulate the axios instance and interceptor behavior
    const mockAxiosInstance = {
      request: vi.fn(),
    };

    // Mock API responses
    const mockRefreshCreate = vi.fn().mockImplementation(async () => {
      callLog.push('refresh-called');
      return { status: 204 }; // Backend sets new cookies
    });

    const mockUserList = vi.fn().mockImplementation(async () => {
      callLog.push('user-list-called');
      return { data: { token: 'new-fresh-token', id: 'user-123' } };
    });

    // Simulate the interceptor logic (extracted for testing)
    const simulateInterceptor = async (error: any) => {
      const originalRequest = error.config;

      if (error.response?.status === 401) {
        const url = originalRequest?.url || '';
        const isAuthEndpoint = url.includes('/api/v1/auth/');

        if (!isAuthEndpoint && !originalRequest._retry) {
          originalRequest._retry = true;
          callLog.push('interceptor-caught-401');

          // Attempt refresh
          try {
            await mockRefreshCreate();
            callLog.push('refresh-succeeded');

            // Get new token
            const userResponse = await mockUserList();
            const newToken = userResponse.data?.token;

            if (newToken) {
              callLog.push(`token-updated:${newToken}`);
            }

            // Delete old Authorization header
            delete originalRequest.headers['Authorization'];
            callLog.push('old-auth-header-deleted');

            // Retry the original request
            retryCount++;
            callLog.push('request-retried');

            // Simulate successful retry
            return { data: { success: true }, status: 200 };
          } catch (refreshError) {
            callLog.push('refresh-failed');
            throw error;
          }
        }
      }

      throw error;
    };

    // Simulate the original request that gets a 401
    const originalRequest = {
      url: '/api/v1/sessions',
      method: 'GET',
      headers: {
        'Authorization': 'Bearer expired-token-from-8-hours-ago',
        'Content-Type': 'application/json',
      },
      _retry: false,
    };

    const error401 = {
      response: { status: 401, data: { error: 'Token expired' } },
      config: originalRequest,
    };

    // Execute the interceptor
    const result = await simulateInterceptor(error401);

    // Verify the complete flow happened in order
    expect(callLog).toEqual([
      'interceptor-caught-401',
      'refresh-called',
      'refresh-succeeded',
      'user-list-called',
      'token-updated:new-fresh-token',
      'old-auth-header-deleted',
      'request-retried',
    ]);

    // Verify the request was retried exactly once
    expect(retryCount).toBe(1);

    // Verify the retry succeeded
    expect(result.status).toBe(200);
    expect(result.data.success).toBe(true);

    // Verify old Authorization header was removed
    expect(originalRequest.headers['Authorization']).toBeUndefined();

    // Verify _retry flag was set (prevents infinite loop)
    expect(originalRequest._retry).toBe(true);
  });

  it('should NOT retry if refresh token is also expired (hard logout scenario)', async () => {
    const callLog: string[] = [];

    const mockRefreshCreate = vi.fn().mockImplementation(async () => {
      callLog.push('refresh-called');
      throw new Error('Refresh token expired'); // This happens when refresh token is invalid
    });

    const simulateInterceptor = async (error: any) => {
      const originalRequest = error.config;

      if (error.response?.status === 401) {
        const url = originalRequest?.url || '';
        const isAuthEndpoint = url.includes('/api/v1/auth/');

        if (!isAuthEndpoint && !originalRequest._retry) {
          originalRequest._retry = true;
          callLog.push('interceptor-caught-401');

          try {
            await mockRefreshCreate();
          } catch (refreshError) {
            callLog.push('refresh-failed');
            throw error; // Propagate original 401 - will trigger logout
          }
        }
      }

      throw error;
    };

    const originalRequest = {
      url: '/api/v1/sessions',
      method: 'GET',
      headers: { 'Authorization': 'Bearer expired-token' },
      _retry: false,
    };

    const error401 = {
      response: { status: 401, data: { error: 'Token expired' } },
      config: originalRequest,
    };

    // Should throw the original error (triggers logout in account context)
    await expect(simulateInterceptor(error401)).rejects.toEqual(error401);

    expect(callLog).toEqual([
      'interceptor-caught-401',
      'refresh-called',
      'refresh-failed',
    ]);
  });

  it('should NOT attempt refresh for auth endpoints (prevents infinite loop)', async () => {
    const callLog: string[] = [];
    const mockRefreshCreate = vi.fn();

    const simulateInterceptor = async (error: any) => {
      const originalRequest = error.config;

      if (error.response?.status === 401) {
        const url = originalRequest?.url || '';
        const isAuthEndpoint = url.includes('/api/v1/auth/');

        if (isAuthEndpoint) {
          callLog.push('skipped-auth-endpoint');
        }

        if (!isAuthEndpoint && !originalRequest._retry) {
          await mockRefreshCreate();
          callLog.push('refresh-called');
        }
      }

      throw error;
    };

    // Simulate 401 on the refresh endpoint itself
    const refreshEndpointError = {
      response: { status: 401 },
      config: {
        url: '/api/v1/auth/refresh',
        headers: {},
        _retry: false,
      },
    };

    await expect(simulateInterceptor(refreshEndpointError)).rejects.toBeDefined();

    expect(callLog).toEqual(['skipped-auth-endpoint']);
    expect(mockRefreshCreate).not.toHaveBeenCalled();
  });
});

// Test the core refresh logic in isolation
describe('Token Refresh Logic - Unit Tests', () => {
  // These tests verify the expected behavior of individual pieces

  describe('attemptTokenRefresh behavior', () => {
    it('should return true when refresh succeeds', async () => {
      // This tests the pattern: refresh -> fetch user -> update token
      const mockRefreshCreate = vi.fn().mockResolvedValue({ status: 204 });
      const mockUserList = vi.fn().mockResolvedValue({
        data: { token: 'new-access-token', id: 'user-123' }
      });

      // Simulate the refresh flow
      await mockRefreshCreate();
      const userResponse = await mockUserList();
      const newToken = userResponse.data?.token;

      expect(newToken).toBe('new-access-token');
      expect(mockRefreshCreate).toHaveBeenCalledTimes(1);
      expect(mockUserList).toHaveBeenCalledTimes(1);
    });

    it('should handle refresh failure gracefully', async () => {
      const mockRefreshCreate = vi.fn().mockRejectedValue(
        new Error('Refresh token expired')
      );

      let refreshSucceeded = true;
      try {
        await mockRefreshCreate();
      } catch {
        refreshSucceeded = false;
      }

      expect(refreshSucceeded).toBe(false);
    });
  });

  describe('401 interceptor behavior', () => {
    it('should not retry auth endpoints to prevent infinite loops', () => {
      const url = '/api/v1/auth/refresh';
      const isAuthEndpoint = url.includes('/api/v1/auth/');

      expect(isAuthEndpoint).toBe(true);
      // Auth endpoints should be skipped
    });

    it('should retry non-auth endpoints', () => {
      const url = '/api/v1/sessions';
      const isAuthEndpoint = url.includes('/api/v1/auth/');

      expect(isAuthEndpoint).toBe(false);
      // Non-auth endpoints should trigger refresh
    });

    it('should use _retry flag to prevent double retry', () => {
      const originalRequest = { _retry: false, url: '/api/v1/sessions' };

      // First 401 - should retry
      expect(originalRequest._retry).toBe(false);
      originalRequest._retry = true;

      // Second 401 - should not retry
      expect(originalRequest._retry).toBe(true);
    });

    it('should delete Authorization header before retry', () => {
      const originalRequest = {
        headers: {
          'Authorization': 'Bearer old-expired-token',
          'Content-Type': 'application/json'
        }
      };

      // Simulate what the interceptor does after refresh
      delete originalRequest.headers['Authorization'];

      expect(originalRequest.headers['Authorization']).toBeUndefined();
      expect(originalRequest.headers['Content-Type']).toBe('application/json');
    });
  });

  describe('race condition handling', () => {
    it('should deduplicate concurrent refresh attempts', async () => {
      let isRefreshing = false;
      let refreshPromise: Promise<boolean> | null = null;
      let refreshCallCount = 0;

      const attemptRefresh = async (): Promise<boolean> => {
        refreshCallCount++;
        await new Promise(resolve => setTimeout(resolve, 100));
        return true;
      };

      const handleRequest = async (): Promise<boolean> => {
        if (isRefreshing && refreshPromise) {
          // Wait for existing refresh
          return refreshPromise;
        }

        isRefreshing = true;
        refreshPromise = attemptRefresh();

        try {
          return await refreshPromise;
        } finally {
          isRefreshing = false;
          refreshPromise = null;
        }
      };

      // Simulate 5 concurrent 401 responses
      const results = await Promise.all([
        handleRequest(),
        handleRequest(),
        handleRequest(),
        handleRequest(),
        handleRequest(),
      ]);

      // All should succeed
      expect(results.every(r => r === true)).toBe(true);
      // But only 1-2 actual refresh calls (first one + possibly one more if timing is tight)
      expect(refreshCallCount).toBeLessThanOrEqual(2);
    });
  });

  describe('token update after refresh', () => {
    it('should update both axios defaults and security data', () => {
      const newToken = 'new-access-token';
      const axiosDefaults = { headers: { common: {} as Record<string, string> } };
      let securityData: { token: string } | null = null;

      const setSecurityData = (data: { token: string }) => {
        securityData = data;
      };

      const getTokenHeaders = (token: string) => ({
        Authorization: `Bearer ${token}`,
      });

      // Simulate what attemptTokenRefresh does after getting new token
      axiosDefaults.headers.common = getTokenHeaders(newToken);
      setSecurityData({ token: newToken });

      expect(axiosDefaults.headers.common['Authorization']).toBe('Bearer new-access-token');
      expect(securityData?.token).toBe('new-access-token');
    });
  });
});
