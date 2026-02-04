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

Add response interceptor to watch for `X-Token-Refreshed` header:

```typescript
apiClientSingleton.instance.interceptors.response.use(
  (response) => {
    // Check if token was refreshed by backend
    const newToken = response.headers['x-token-refreshed'];
    if (newToken) {
      console.log('[AUTH] Token refreshed by backend, updating axios headers');
      axios.defaults.headers.common = getTokenHeaders(newToken);
    }
    return response;
  },
  // ... existing error handling
);
```

### WebSocket Handlers

WebSocket endpoints don't go through `extractMiddleware`, so they need their own token refresh logic. Updated:

- `websocket_server_user.go`: Added token refresh logic for user websocket connections

```go
user, err := apiServer.authMiddleware.getUserFromToken(r.Context(), getRequestToken(r))

// If auth failed (error or no user), attempt transparent token refresh
needsRefresh := err != nil || user == nil || !hasUser(user)
if needsRefresh && apiServer.authMiddleware.oidcClient != nil && apiServer.Cfg != nil {
    cm := NewCookieManager(apiServer.Cfg)
    refreshToken, refreshErr := cm.Get(r, refreshTokenCookie)
    if refreshErr == nil && refreshToken != "" && !looksLikeHelixJWT(refreshToken) {
        newToken, refreshErr := apiServer.authMiddleware.oidcClient.RefreshAccessToken(r.Context(), refreshToken)
        if refreshErr == nil && newToken.AccessToken != "" {
            // Update cookies with new tokens
            cm.Set(w, accessTokenCookie, newToken.AccessToken)
            if newToken.RefreshToken != "" {
                cm.Set(w, refreshTokenCookie, newToken.RefreshToken)
            }
            // Set header for frontend to update its in-memory token
            w.Header().Set("X-Token-Refreshed", newToken.AccessToken)
            // Retry auth with the new token
            user, err = apiServer.authMiddleware.getUserFromToken(r.Context(), newToken.AccessToken)
        }
    }
}
```

Note: `websocket_server_runner.go` doesn't need this - it uses static runner tokens, not OIDC.

## Testing Plan

1. Wait for token to expire (or simulate with short-lived token)
2. Refresh page (without logging out)
3. Verify:
   - No 401 errors in console
   - License loads correctly
   - Organizations load correctly
   - Projects load correctly
   - WebSocket connections establish successfully
   - `X-Token-Refreshed` header visible in network tab

## Rollback

If issues arise, revert the middleware changes. The existing frontend interceptor logic remains as a fallback for edge cases.
