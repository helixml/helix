# BFF Authentication Rewrite

**Date:** 2026-02-05
**Status:** In Progress
**Author:** Claude (with Luke)

## Problem

The current frontend authentication system is a hybrid approach that's the worst of both worlds:

1. **Frontend manages tokens in 5+ different locations:**
   - `axios.defaults.headers.common` (global axios)
   - `apiClientSingleton.setSecurityData()` (generated API client)
   - `localStorage.setItem('token', ...)` (persistence)
   - React state via `account.token`
   - Custom event dispatch (`TOKEN_REFRESHED_EVENT`)

2. **Frontend deals with OAuth/OIDC complexity:**
   - Understands refresh tokens exist
   - Has to capture `X-Token-Refreshed` headers
   - Multiple interceptors on axios instances
   - Race conditions when token expires

3. **Two different auth systems with different behaviors:**
   - Regular Helix Auth: Long-lived JWTs (7 days default)
   - OIDC Auth: Short-lived access tokens (~1 hour) + refresh tokens

## Solution: Backend-For-Frontend (BFF) Pattern

### Core Principle

The frontend should only care about ONE thing: **an HTTP-only session cookie**.

- Frontend initiates login → gets redirected to auth flow
- On successful auth → backend sets HTTP-only session cookie
- All API requests automatically include the cookie (same-origin)
- Session expires naturally after 30 days
- **No tokens in JavaScript memory, ever**

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         FRONTEND                                 │
│  - No tokens in memory                                          │
│  - No Authorization headers                                     │
│  - No localStorage token storage                                │
│  - Just calls APIs, cookies sent automatically                  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTP requests (cookies sent automatically)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      HELIX API (BFF)                            │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Session Management Layer                    │   │
│  │  - Validates session cookie (helix_session)              │   │
│  │  - Extracts user from session                            │   │
│  │  - Handles OIDC token refresh transparently              │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                   │
│          ┌───────────────────┼───────────────────┐              │
│          ▼                   ▼                   ▼              │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────────┐    │
│   │   Regular   │    │    OIDC     │    │   API Key       │    │
│   │   Helix     │    │   (Google)  │    │   (unchanged)   │    │
│   │   Auth      │    │             │    │                 │    │
│   └─────────────┘    └─────────────┘    └─────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

### Session Table Schema

We'll follow the same pattern as `OAuthConnection` (see `api/pkg/types/oauth.go`), which already has:
- Token storage (access_token, refresh_token, expires_at)
- Background refresh via `RefreshExpiredTokens()` in the OAuth manager
- Database-backed persistence

```go
// UserSession represents an authenticated user session
// Similar pattern to OAuthConnection but for user auth
type UserSession struct {
    ID        string         `json:"id" gorm:"primaryKey;type:uuid"`
    CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
    UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
    DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

    // User who owns this session
    UserID string `json:"user_id" gorm:"not null;index"`

    // Auth provider used: "regular" or "oidc"
    AuthProvider string `json:"auth_provider" gorm:"not null;type:text"`

    // Session expiry (30 days from creation)
    ExpiresAt time.Time `json:"expires_at" gorm:"not null;index"`

    // For OIDC sessions: store the refresh token so backend can refresh access tokens
    // Access token is short-lived and fetched on-demand when needed
    OIDCRefreshToken string    `json:"-" gorm:"type:text"`  // Never expose to frontend
    OIDCAccessToken  string    `json:"-" gorm:"type:text"`  // Cache for backend use
    OIDCTokenExpiry  time.Time `json:"-"`                    // When access token expires

    // Optional metadata for security/audit
    UserAgent string `json:"user_agent,omitempty" gorm:"type:text"`
    IPAddress string `json:"ip_address,omitempty" gorm:"type:varchar(45)"`
    LastUsedAt time.Time `json:"last_used_at"`
}
```

### Code Reuse from OAuth Manager

The `api/pkg/oauth/manager.go` has patterns we can reuse:

1. **Background refresh job** (`RefreshExpiredTokens`):
   ```go
   // Already runs every minute, refreshes tokens approaching expiry
   err := m.RefreshExpiredTokens(ctx, 5*time.Minute)
   ```

2. **Token refresh on access** (`RefreshTokenIfNeeded`):
   ```go
   // Called when getting a connection, refreshes if needed
   if err := provider.RefreshTokenIfNeeded(ctx, connection); err != nil {
       return nil, fmt.Errorf("failed to refresh token: %w", err)
   }
   ```

For user sessions, we'll create a `SessionManager` that follows the same pattern:
- Background goroutine to refresh OIDC tokens before they expire
- `RefreshSessionIfNeeded()` called when validating session
- Reuse the existing `auth/oidc.go` OIDC client for token refresh

### Cookie Design

**Single cookie: `helix_session`**
- Value: Session ID (UUID)
- HttpOnly: true (JavaScript cannot access)
- Secure: true (HTTPS only in production)
- SameSite: Lax (CSRF protection)
- Path: /
- MaxAge: 30 days (2592000 seconds)

### Auth Flow

#### Regular Helix Auth (Email/Password)

```
1. POST /api/v1/auth/login { email, password }
2. Backend validates credentials
3. Backend creates session in database
4. Backend sets helix_session cookie
5. Returns { authenticated: true, user: { ... } }
```

#### OIDC Auth (Google)

```
1. POST /api/v1/auth/login → returns { redirect_url: "/api/v1/auth/oidc" }
2. Frontend redirects to OIDC provider
3. User authenticates with Google
4. Callback: /api/v1/auth/oidc/callback?code=...
5. Backend exchanges code for tokens
6. Backend creates session with OIDC tokens stored
7. Backend sets helix_session cookie
8. Redirects to frontend (original URL or /)
```

### Session Validation Middleware

```go
func (s *HelixAPIServer) sessionMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        sessionID, err := r.Cookie("helix_session")
        if err != nil || sessionID.Value == "" {
            // No session - continue without user (public endpoint)
            // or return 401 for protected endpoints
            next.ServeHTTP(w, r)
            return
        }

        session, err := s.store.GetSession(r.Context(), sessionID.Value)
        if err != nil || session.IsExpired() {
            // Invalid/expired session - clear cookie
            clearSessionCookie(w)
            next.ServeHTTP(w, r)
            return
        }

        // For OIDC sessions, check if access token needs refresh
        if session.AuthProvider == "oidc" && session.OIDCTokenNeedsRefresh() {
            newToken, err := s.oidcClient.RefreshAccessToken(r.Context(), session.OIDCRefreshToken)
            if err != nil {
                // Refresh failed - session is invalid
                s.store.DeleteSession(r.Context(), session.ID)
                clearSessionCookie(w)
                next.ServeHTTP(w, r)
                return
            }
            session.UpdateOIDCTokens(newToken)
            s.store.UpdateSession(r.Context(), session)
        }

        // Update last_used_at periodically (not every request)
        session.TouchIfNeeded(s.store)

        // Get user from database and add to context
        user, err := s.store.GetUser(r.Context(), session.UserID)
        if err != nil {
            clearSessionCookie(w)
            next.ServeHTTP(w, r)
            return
        }

        ctx := setRequestUser(r.Context(), *user)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### API Changes

#### New Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/auth/session` | GET | Get current session info |
| `/api/v1/auth/logout` | POST | Delete session, clear cookie |

#### Modified Endpoints

| Endpoint | Change |
|----------|--------|
| `/api/v1/auth/login` | Create session, set cookie |
| `/api/v1/auth/oidc/callback` | Create session, set cookie |
| `/api/v1/auth/authenticated` | Check session cookie instead of access_token |
| `/api/v1/auth/user` | Get user from session, no token in response |

#### Removed/Deprecated

| Endpoint | Reason |
|----------|--------|
| `/api/v1/auth/refresh` | No longer needed - backend handles transparently |
| `X-Token-Refreshed` header | No longer needed |

### Frontend Changes

#### Files to Modify

1. **`frontend/src/hooks/useApi.ts`**
   - Remove `setToken()` function
   - Remove `handleTokenRefreshHeader()`
   - Remove all token-related interceptors
   - Remove `localStorage.setItem('token', ...)`
   - Remove `securityWorker` (no Authorization header needed)

2. **`frontend/src/contexts/account.tsx`**
   - Remove `token` from context
   - Remove `tokenUrlEscaped`
   - Remove `TOKEN_REFRESHED_EVENT` handling
   - Simplify `initialize()` - just call `/api/v1/auth/session`
   - Simplify `onLogout()` - just call `/api/v1/auth/logout`

3. **`frontend/src/hooks/useWebsocket.ts`**
   - Remove token dependency (cookies sent automatically with WS)

4. **`frontend/src/contexts/streaming.tsx`**
   - Remove Authorization header from EventSource
   - Cookies are sent automatically with EventSource

5. **`frontend/src/hooks/useKnowledge.ts`**
   - Remove Authorization header from fetch calls

6. **`frontend/src/components/auth/TokenExpiryCounter.tsx`**
   - Remove entirely (no token expiry visible to frontend)

### Migration Path

1. **Add session table migration**
2. **Add session middleware** (new code path)
3. **Update auth endpoints** to create sessions
4. **Update frontend** to stop using tokens
5. **Remove old token code** from frontend
6. **Clean up deprecated endpoints**

### Backward Compatibility

During migration:
- Both old (cookie-based token) and new (session-based) auth work
- API key auth remains unchanged
- Runner token auth remains unchanged

### Security Considerations

1. **CSRF Protection**
   - SameSite=Lax on cookies
   - State parameter in OIDC flow (already implemented)

2. **Session Hijacking**
   - Secure cookies (HTTPS only)
   - HttpOnly (no JS access)
   - Session tied to IP/user-agent (optional)

3. **Session Fixation**
   - New session ID generated on each login

4. **Token Storage**
   - OIDC tokens encrypted at rest in database (recommended)
   - Only refresh token stored long-term; access token cached briefly

### Testing Plan

1. **Regular Auth Flow**
   - Login with email/password
   - Verify session cookie set
   - Verify API calls work without Authorization header
   - Logout and verify session deleted

2. **OIDC Auth Flow**
   - Login with Google
   - Verify session cookie set
   - Wait for OIDC token to expire (or simulate)
   - Verify backend refreshes transparently
   - Logout and verify session deleted

3. **Page Refresh**
   - Login, refresh page
   - Verify still authenticated
   - Verify no 401 errors

4. **Cross-tab**
   - Login in one tab
   - Open new tab
   - Verify authenticated in new tab

5. **Session Expiry**
   - Create session
   - Wait for expiry (or simulate)
   - Verify redirect to login

## Frontend API Interaction Audit

All places in the frontend that interact with the API, and how they handle auth:

### 1. Generated API Client (`apiClient.xxx()`)
**Files:** All service files (`projectService.ts`, `sessionService.ts`, etc.)
**Current auth:** `securityWorker` adds Authorization header from in-memory token
**BFF change:** Remove `securityWorker` - cookies sent automatically

### 2. Raw axios via useApi hook (`api.get()`, `api.post()`)
**Files:** `account.tsx` (`loadStatus`), various legacy code
**Current auth:** `axios.defaults.headers.common['Authorization']` set by `setToken()`
**BFF change:** Remove `setToken()` - cookies sent automatically

### 3. Direct fetch() calls
**Files:** `useKnowledge.ts`, `filestoreService.ts`, `account.tsx` (login/logout)
**Current auth:** Manual `Authorization: Bearer ${account.token}` header
**BFF change:** Add `credentials: 'same-origin'` (default for same-origin, but explicit is clearer)

### 4. EventSource (Server-Sent Events)
**Files:** `streaming.tsx`
**Current auth:** Cannot set custom headers on EventSource; currently passes token in URL or relies on cookies
**BFF change:** Cookies sent automatically by browser for same-origin

### 5. WebSocket connections
**Files:** `useWebsocket.ts`, `DesignReviewContent.tsx`, `streaming.tsx`
**Current auth:** Browsers send cookies automatically with WebSocket connections
**BFF change:** No change needed - already works via cookies

### 6. helix-stream library
**Files:** `lib/helix-stream/api.ts`
**Purpose:** Moonlight streaming host control (not Helix API auth)
**Current auth:** Separate credential system (`sessionStorage.mlCredentials`)
**BFF change:** Not affected - this is for external streaming hosts

### Summary of Changes Needed

| API Method | Current Token Source | BFF Change |
|------------|---------------------|------------|
| Generated client | `securityWorker` | Remove `securityWorker` |
| Raw axios | `axios.defaults.headers.common` | Remove header management |
| fetch() | Manual header | Remove Authorization header |
| EventSource | Cookies (already) | No change |
| WebSocket | Cookies (already) | No change |

## Best Practices Applied (from industry research)

Based on [Auth0](https://auth0.com/blog/the-backend-for-frontend-pattern-bff/), [FusionAuth](https://fusionauth.io/blog/backend-for-frontend), and [Duende BFF Framework](https://docs.duendesoftware.com/bff/):

1. **No tokens in JavaScript, ever** - Session ID only, HttpOnly cookie
2. **Server-side token storage** - OIDC tokens stored in database, not in cookies
3. **Transparent refresh** - Backend handles OIDC token refresh; frontend never knows
4. **CSRF protection** - SameSite=Lax cookies (may add custom header for extra safety)
5. **Secure cookies** - HttpOnly + Secure + SameSite

## Implementation Order

1. Add session table and store methods
2. Add session middleware (in parallel with existing auth)
3. Update login endpoints to create sessions
4. Update frontend to use session-based auth
5. Remove old token code
6. Clean up and test

## Answers to Key Questions

**Should frontend have NO refresh logic?**
Yes. The frontend should be completely unaware of token refresh. The backend handles all of this:
- Regular auth: Long-lived session (30 days), no refresh needed
- OIDC auth: Session (30 days), backend refreshes OIDC tokens transparently

**Should we rip out all token storage locations?**
Yes. The frontend currently stores tokens in 5+ locations. After BFF:
- No localStorage token
- No axios.defaults.headers.common['Authorization']
- No apiClientSingleton.setSecurityData()
- No account.token React state
- No TOKEN_REFRESHED_EVENT

The only "state" is the HttpOnly cookie, which JavaScript cannot access.

**Do cookies work with WebSockets?**
Yes. Browsers automatically send cookies with WebSocket connections to the same origin. This is why the current system already relies on cookies for WebSocket auth.

## Sources

- [Auth0: The Backend for Frontend Pattern (BFF)](https://auth0.com/blog/the-backend-for-frontend-pattern-bff/)
- [FusionAuth: A Guide to Backend-for-Frontend Auth](https://fusionauth.io/blog/backend-for-frontend)
- [Duende: BFF Security Framework](https://docs.duendesoftware.com/bff/)
- [Medium: Secure Your Tokens the Right Way: BFF + Redis Explained](https://dev.to/sovannaro/the-backend-for-frontend-bff-pattern-secure-auth-done-right-fm7)
