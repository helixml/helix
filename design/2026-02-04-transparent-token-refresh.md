# Transparent Token Refresh in API Middleware

**Date:** 2026-02-04
**Status:** In Progress
**Author:** Claude (with Luke)

## Problem

When using Google OIDC authentication, access tokens expire after approximately 1 hour. The current architecture has a fragile token refresh flow:

1. Frontend stores access token in axios headers (from `/api/v1/auth/user` response)
2. When API returns 401, frontend interceptor calls `/api/v1/auth/refresh`
3. Refresh endpoint updates cookies but returns 204 No Content (no token in body)
4. Frontend then calls `/api/v1/auth/user` again to get the new token
5. Frontend updates axios headers with new token

**Failure modes:**
- Race conditions: Multiple concurrent requests can all get 401, causing multiple refresh attempts
- Stale axios state: If refresh succeeds but frontend doesn't update axios headers, subsequent requests fail
- React Query retries: Failed queries may not retry with new token
- Page refresh: If token expires between page loads, user sees errors before interceptor can refresh

**Symptoms observed:**
- License popup appearing despite valid license in database
- Organizations not loading ("not found" errors)
- Projects not loading
- Intermittent data visibility with expired tokens

## Solution: Backend-Transparent Token Refresh

Move token refresh responsibility to the API middleware (`extractMiddleware`). When token validation fails:

1. Check for refresh_token cookie
2. Attempt to refresh using OIDC client
3. Update cookies with new tokens
4. Set `X-Token-Refreshed: <new_token>` response header
5. Continue processing request with refreshed user context

The frontend watches for `X-Token-Refreshed` header and updates its in-memory token.

### Benefits

1. **Atomic refresh**: Token refresh happens in the same request, no race conditions
2. **Transparent to frontend**: Requests succeed without explicit retry logic
3. **Cookies always current**: Backend owns cookie state, frontend just reads headers
4. **Works on page refresh**: First request after page load can refresh inline

## Implementation

### API Changes (`api/pkg/server/auth_middleware.go`)

```go
func (auth *authMiddleware) extractMiddleware(next http.Handler) http.Handler {
    f := func(w http.ResponseWriter, r *http.Request) {
        user, err := auth.getUserFromToken(r.Context(), getRequestToken(r))
        if err != nil {
            // Check for provider not ready (return 503)
            if errors.Is(err, authpkg.ErrProviderNotReady) {
                http.Error(w, "Authentication service temporarily unavailable", http.StatusServiceUnavailable)
                return
            }

            // Check for stale Helix JWT (clear cookies, return 401)
            if errors.Is(err, ErrHelixTokenWithOIDC) && auth.serverCfg != nil {
                NewCookieManager(auth.serverCfg).DeleteAllCookies(w)
                http.Error(w, err.Error(), http.StatusUnauthorized)
                return
            }

            // NEW: Attempt transparent refresh using refresh_token cookie
            if auth.oidcClient != nil && auth.serverCfg != nil {
                cm := NewCookieManager(auth.serverCfg)
                refreshToken, refreshErr := cm.Get(r, refreshTokenCookie)
                if refreshErr == nil && refreshToken != "" && !looksLikeHelixJWT(refreshToken) {
                    newToken, refreshErr := auth.oidcClient.RefreshAccessToken(r.Context(), refreshToken)
                    if refreshErr == nil && newToken.AccessToken != "" {
                        // Update cookies
                        cm.Set(w, accessTokenCookie, newToken.AccessToken)
                        if newToken.RefreshToken != "" {
                            cm.Set(w, refreshTokenCookie, newToken.RefreshToken)
                        }

                        // Set header for frontend to update its in-memory token
                        w.Header().Set("X-Token-Refreshed", newToken.AccessToken)

                        // Retry auth with new token
                        user, err = auth.getUserFromToken(r.Context(), newToken.AccessToken)
                        if err == nil {
                            log.Info().Str("path", r.URL.Path).Msg("Token refreshed transparently in middleware")
                            // Continue to request processing below
                        }
                    }
                }
            }

            // If still no valid user after refresh attempt, return 401
            if err != nil {
                http.Error(w, err.Error(), http.StatusUnauthorized)
                return
            }
        }

        // ... rest of existing logic
    }
}
```

### Frontend Changes (`frontend/src/hooks/useApi.ts`)

The frontend uses interceptors to watch for `X-Token-Refreshed` header on ALL responses (both success and error):

```typescript
// Helper function to handle X-Token-Refreshed header from backend
// This is called for both successful and error responses
const handleTokenRefreshHeader = (headers: Record<string, string> | undefined) => {
  if (!headers) return
  const newToken = headers['x-token-refreshed']
  if (newToken) {
    console.log('[API] Token refreshed transparently by backend, updating all token locations')

    // Update axios defaults (for raw axios calls)
    axios.defaults.headers.common = getTokenHeaders(newToken)

    // Update OpenAPI client security data
    apiClientSingleton.setSecurityData({ token: newToken })

    // Also update the client instance headers directly
    apiClientSingleton.instance.defaults.headers.common['Authorization'] = `Bearer ${newToken}`

    // Update localStorage for direct fetch() calls
    localStorage.setItem('token', newToken)

    // Dispatch event so account.tsx can update React state
    window.dispatchEvent(new CustomEvent(TOKEN_REFRESHED_EVENT, { detail: { token: newToken } }))
  }
}

// Interceptor on API client instance
apiClientSingleton.instance.interceptors.response.use(
  (response) => {
    handleTokenRefreshHeader(response.headers)
    return response
  },
  (error) => {
    // Also check error responses - token may be refreshed but request fails for other reasons
    handleTokenRefreshHeader(error.response?.headers)
    return Promise.reject(error)
  }
)

// Also add interceptor to global axios instance for raw axios.get/post calls
axios.interceptors.response.use(
  (response) => {
    handleTokenRefreshHeader(response.headers)
    return response
  },
  (error) => {
    handleTokenRefreshHeader(error.response?.headers)
    return Promise.reject(error)
  }
)
```

Key improvements:
1. **Error response handling**: Token may be refreshed but request can still fail (e.g., user not authorized for resource). We now capture the token from error responses too.
2. **Global axios interceptor**: Raw `axios.get/post` calls (used in some legacy code paths) also trigger the interceptor.
3. **Multiple update locations**: Token is updated in axios defaults, API client security data, localStorage, and React state (via custom event).

### WebSockets and Cookies

WebSocket connections use cookies for authentication (browsers cannot set custom headers on WS connections). The token refresh updates cookies via `Set-Cookie` headers, so subsequent WebSocket connections will use the refreshed token automatically.

Note: **Never put tokens in WebSocket URLs** - this is a security risk as URLs get logged.

## Testing Plan

1. Wait for token to expire (or simulate with short-lived token)
2. Refresh page (without logging out)
3. Verify:
   - No 401 errors in console
   - License loads correctly
   - Organizations load correctly
   - Projects load correctly
   - `X-Token-Refreshed` header visible in network tab
   - WebSocket connections authenticate via refreshed cookie

## Remaining Investigation

If issues persist after page refresh:
1. Check browser Network tab for `/api/v1/auth/authenticated` response
2. Check if `Set-Cookie` headers are being sent on refresh
3. Check browser Console for `[API] Token refreshed transparently` log
4. Verify refresh_token cookie exists in Application > Cookies

## Rollback

If issues arise, revert the middleware changes. The existing frontend interceptor logic remains as a fallback for edge cases.
