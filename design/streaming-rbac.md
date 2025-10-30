# Streaming Access Control & RBAC Design

## Overview

This document describes the enterprise-ready Role-Based Access Control (RBAC) system for Moonlight streaming sessions in Helix, including secure session sharing with team members and managers.

## Security Requirements

1. **Authentication**: Only authenticated Helix users can access streaming
2. **Authorization**: Users can only access streams they own or have been granted access to
3. **Audit Trail**: All streaming access logged for compliance
4. **Session Isolation**: Streaming sessions isolated by Wolf lobby PINs
5. **Secure Sharing**: Fine-grained sharing with role-based permissions
6. **Enterprise Ready**: Integration with existing Helix RBAC (Keycloak, roles)

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                        Browser Client                           │
│                                                                 │
│  User requests streaming → Helix authenticates → Issues token  │
└─────────────────────────────┬───────────────────────────────────┘
                              │ JWT Token
                              ▼
┌────────────────────────────────────────────────────────────────┐
│                      Helix API Server                          │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │  Streaming Access Control Middleware                     │ │
│  │                                                           │ │
│  │  1. Verify JWT token                                     │ │
│  │  2. Extract user_id from token                           │ │
│  │  3. Check session/PDE ownership OR access grants         │ │
│  │  4. Generate time-limited streaming token                │ │
│  │  5. Log access for audit                                 │ │
│  └──────────────────────────────────────────────────────────┘ │
│                              │                                 │
│                              ▼                                 │
│  GET /api/v1/sessions/{id}/stream-token                       │
│      → Returns: {stream_token, wolf_lobby_id, credentials}    │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │  Moonlight Web Proxy with Token Validation               │ │
│  │                                                           │ │
│  │  /moonlight/* → moonlight-web:8080                        │ │
│  │    + Injects stream_token as auth header                 │ │
│  └──────────────────────────────────────────────────────────┘ │
└─────────────────────────────┬───────────────────────────────────┘
                              │ Token + Lobby Credentials
                              ▼
┌────────────────────────────────────────────────────────────────┐
│                    Moonlight Web Stream                         │
│                                                                │
│  Validates stream_token → Allows access to Wolf lobby         │
└─────────────────────────────┬───────────────────────────────────┘
                              │ With Lobby PIN
                              ▼
┌────────────────────────────────────────────────────────────────┐
│                         Wolf Server                            │
│                                                                │
│  Verifies lobby PIN → Grants streaming access                 │
└────────────────────────────────────────────────────────────────┘
```

## Database Schema

### New Table: streaming_access_grants

```sql
CREATE TABLE streaming_access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- What is being shared
    session_id TEXT,  -- Either session_id (for external agents)
    pde_id TEXT,      -- Or PDE ID (for personal dev environments)

    -- Who owns it
    owner_user_id TEXT NOT NULL REFERENCES users(id),

    -- Who can access it
    granted_user_id TEXT REFERENCES users(id),     -- Specific user
    granted_team_id TEXT,                           -- Or entire team
    granted_role TEXT,                              -- Or everyone with this role

    -- What they can do
    access_level TEXT NOT NULL,  -- 'view', 'control', 'admin'

    -- When
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,  -- Optional expiration

    -- Audit
    granted_by TEXT NOT NULL REFERENCES users(id),
    revoked_at TIMESTAMP WITH TIME ZONE,
    revoked_by TEXT REFERENCES users(id),

    -- Constraints
    CONSTRAINT check_target CHECK (
        (session_id IS NOT NULL AND pde_id IS NULL) OR
        (session_id IS NULL AND pde_id IS NOT NULL)
    ),
    CONSTRAINT check_grantee CHECK (
        (granted_user_id IS NOT NULL AND granted_team_id IS NULL AND granted_role IS NULL) OR
        (granted_user_id IS NULL AND granted_team_id IS NOT NULL AND granted_role IS NULL) OR
        (granted_user_id IS NULL AND granted_team_id IS NULL AND granted_role IS NOT NULL)
    )
);

CREATE INDEX idx_streaming_access_session ON streaming_access_grants(session_id) WHERE session_id IS NOT NULL;
CREATE INDEX idx_streaming_access_pde ON streaming_access_grants(pde_id) WHERE pde_id IS NOT NULL;
CREATE INDEX idx_streaming_access_user ON streaming_access_grants(granted_user_id) WHERE granted_user_id IS NOT NULL;
CREATE INDEX idx_streaming_access_team ON streaming_access_grants(granted_team_id) WHERE granted_team_id IS NOT NULL;
CREATE INDEX idx_streaming_access_role ON streaming_access_grants(granted_role) WHERE granted_role IS NOT NULL;
```

### New Table: streaming_access_audit_log

```sql
CREATE TABLE streaming_access_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- What was accessed
    session_id TEXT,
    pde_id TEXT,
    wolf_lobby_id TEXT,

    -- Who accessed it
    user_id TEXT NOT NULL REFERENCES users(id),
    access_level TEXT NOT NULL,

    -- How
    access_method TEXT NOT NULL,  -- 'owner', 'user_grant', 'team_grant', 'role_grant'
    grant_id UUID REFERENCES streaming_access_grants(id),

    -- When
    accessed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    session_duration_seconds INTEGER,

    -- Where from
    ip_address INET,
    user_agent TEXT,

    CONSTRAINT check_target CHECK (
        (session_id IS NOT NULL AND pde_id IS NULL) OR
        (session_id IS NULL AND pde_id IS NOT NULL)
    )
);

CREATE INDEX idx_streaming_audit_user ON streaming_access_audit_log(user_id);
CREATE INDEX idx_streaming_audit_session ON streaming_access_audit_log(session_id) WHERE session_id IS NOT NULL;
CREATE INDEX idx_streaming_audit_pde ON streaming_access_audit_log(pde_id) WHERE pde_id IS NOT NULL;
CREATE INDEX idx_streaming_audit_time ON streaming_access_audit_log(accessed_at);
```

## Access Levels

### View
- Can see video stream (read-only)
- Cannot control mouse/keyboard
- Cannot modify settings
- **Use case**: Manager observing agent work, team demo

### Control
- Can see video stream
- Can control mouse/keyboard/touch
- Can interact with applications
- Cannot modify streaming settings or terminate session
- **Use case**: Team member collaboration, pair programming

### Admin
- Full control including stream settings
- Can terminate streaming session
- Can grant access to others
- **Use case**: Session owner, DevOps admin

## RBAC Roles

### New Roles

```go
const (
    StreamingRoleViewer     = "streaming:viewer"      // Read-only access
    StreamingRoleController = "streaming:controller"  // Interactive access
    StreamingRoleAdmin      = "streaming:admin"       // Full control
)
```

### Role Mapping

| User Type | Default Role | Can Grant Access | Can Terminate |
|-----------|--------------|------------------|---------------|
| Owner | admin | ✅ | ✅ |
| Manager (via team) | control | ✅ | ❌ |
| Team Member (shared) | view or control | ❌ | ❌ |
| External User (role grant) | view | ❌ | ❌ |

## API Endpoints

### 1. Get Streaming Token

```
GET /api/v1/sessions/{id}/stream-token
GET /api/v1/personal-dev-environments/{id}/stream-token
```

**Purpose**: Generate time-limited streaming token with embedded access level

**Authorization**:
- User must be owner OR have access grant OR have appropriate role

**Response**:
```json
{
  "stream_token": "eyJhbGc...",  // JWT with 1-hour expiry
  "wolf_lobby_id": "lobby_123",
  "wolf_lobby_pin": "1234",
  "moonlight_host_id": 0,
  "moonlight_app_id": 1,
  "access_level": "control",
  "expires_at": "2025-10-08T10:40:00Z"
}
```

**JWT Claims**:
```json
{
  "sub": "usr_123",               // User ID
  "session_id": "ses_456",        // Session or PDE ID
  "wolf_lobby_id": "lobby_789",
  "access_level": "control",
  "granted_via": "owner",         // 'owner', 'user_grant', 'team_grant', 'role_grant'
  "exp": 1728384000               // Expiration timestamp
}
```

### 2. Grant Streaming Access

```
POST /api/v1/sessions/{id}/streaming-access
POST /api/v1/personal-dev-environments/{id}/streaming-access
```

**Purpose**: Share streaming access with other users/teams/roles

**Request**:
```json
{
  "granted_user_id": "usr_789",     // Optional: specific user
  "granted_team_id": "team_456",    // Optional: entire team
  "granted_role": "engineering",     // Optional: anyone with role
  "access_level": "control",         // Required: 'view', 'control', 'admin'
  "expires_at": "2025-10-15T00:00:00Z"  // Optional: expiration
}
```

**Authorization**:
- User must have 'admin' access level on the session/PDE

### 3. List Access Grants

```
GET /api/v1/sessions/{id}/streaming-access
GET /api/v1/personal-dev-environments/{id}/streaming-access
```

**Response**:
```json
{
  "grants": [
    {
      "id": "grant_123",
      "granted_user_id": "usr_789",
      "granted_user_name": "alice@company.com",
      "access_level": "control",
      "granted_at": "2025-10-08T09:00:00Z",
      "granted_by": "bob@company.com",
      "expires_at": null
    }
  ]
}
```

### 4. Revoke Access

```
DELETE /api/v1/sessions/{id}/streaming-access/{grant_id}
DELETE /api/v1/personal-dev-environments/{id}/streaming-access/{grant_id}
```

**Authorization**:
- User must be owner OR the user who granted access

### 5. Audit Log

```
GET /api/v1/streaming-access/audit
```

**Query Parameters**:
- `user_id`: Filter by user
- `session_id` / `pde_id`: Filter by session
- `from` / `to`: Time range
- `access_level`: Filter by level

**Response**:
```json
{
  "audit_entries": [
    {
      "id": "audit_123",
      "user_id": "usr_789",
      "user_email": "alice@company.com",
      "session_id": "ses_456",
      "access_level": "control",
      "access_method": "team_grant",
      "accessed_at": "2025-10-08T09:30:00Z",
      "session_duration_seconds": 1800,
      "ip_address": "192.168.1.100"
    }
  ]
}
```

**Authorization**:
- Admins: Can see all audit logs
- Managers: Can see team member logs
- Users: Can see only their own logs

## Implementation Flow

### 1. User Opens Streaming UI

```typescript
// In ScreenshotViewer or MoonlightWebPlayer
const { data: tokenInfo } = useQuery({
  queryKey: ['stream-token', sessionId],
  queryFn: async () => {
    const response = await apiClient.v1SessionsStreamToken(sessionId);
    return response.data;
  },
  // Re-fetch token 5 minutes before expiry
  staleTime: 55 * 60 * 1000,
});

if (!tokenInfo) {
  return <Alert>Requesting streaming access...</Alert>;
}

// Use token to access moonlight-web
const streamUrl = `/moonlight/stream.html?token=${tokenInfo.stream_token}`;
```

### 2. Helix API Validates Access

```go
// api/pkg/server/streaming_access.go

func (s *HelixAPIServer) getStreamingToken(w http.ResponseWriter, r *http.Request) (*types.StreamingTokenResponse, *system.HTTPError) {
    ctx := r.Context()
    sessionID := mux.Vars(r)["id"]
    user := getRequestUser(r)

    // Check if user has access
    accessLevel, grantedVia, err := s.checkStreamingAccess(ctx, user.ID, sessionID)
    if err != nil {
        return nil, system.NewHTTPError403("access denied")
    }

    // Get session/PDE details
    session, err := s.Store.GetSession(ctx, sessionID)
    if err != nil {
        return nil, system.NewHTTPError404("session not found")
    }

    // Generate streaming token (1 hour expiry)
    token, err := s.generateStreamingToken(user.ID, sessionID, session.WolfLobbyID, accessLevel, grantedVia)
    if err != nil {
        return nil, system.NewHTTPError500("failed to generate token")
    }

    // Log access
    s.logStreamingAccess(ctx, user.ID, sessionID, accessLevel, grantedVia, r.RemoteAddr, r.UserAgent())

    return &types.StreamingTokenResponse{
        StreamToken:     token,
        WolfLobbyID:     session.WolfLobbyID,
        WolfLobbyPIN:    session.WolfLobbyPIN,
        MoonlightHostID: 0, // Wolf is always host 0
        MoonlightAppID:  1, // TODO: Map lobby ID to app ID
        AccessLevel:     accessLevel,
        ExpiresAt:       time.Now().Add(1 * time.Hour),
    }, nil
}
```

### 3. Check Access Logic

```go
func (s *HelixAPIServer) checkStreamingAccess(ctx context.Context, userID, sessionID string) (accessLevel, grantedVia string, err error) {
    // Check 1: Is user the owner?
    session, err := s.Store.GetSession(ctx, sessionID)
    if err != nil {
        return "", "", err
    }

    if session.Owner == userID {
        return "admin", "owner", nil
    }

    // Check 2: Direct user grant
    grant, err := s.Store.GetStreamingAccessGrantByUser(ctx, sessionID, userID)
    if err == nil && grant.RevokedAt == nil {
        if grant.ExpiresAt != nil && grant.ExpiresAt.Before(time.Now()) {
            return "", "", errors.New("grant expired")
        }
        return grant.AccessLevel, "user_grant", nil
    }

    // Check 3: Team grant
    userTeams, err := s.Store.GetUserTeams(ctx, userID)
    if err == nil {
        for _, team := range userTeams {
            grant, err := s.Store.GetStreamingAccessGrantByTeam(ctx, sessionID, team.ID)
            if err == nil && grant.RevokedAt == nil {
                if grant.ExpiresAt == nil || grant.ExpiresAt.After(time.Now()) {
                    return grant.AccessLevel, "team_grant", nil
                }
            }
        }
    }

    // Check 4: Role grant (e.g., "manager" role gets view access to all team sessions)
    userRoles, err := s.Store.GetUserRoles(ctx, userID)
    if err == nil {
        for _, role := range userRoles {
            grant, err := s.Store.GetStreamingAccessGrantByRole(ctx, sessionID, role)
            if err == nil && grant.RevokedAt == nil {
                if grant.ExpiresAt == nil || grant.ExpiresAt.After(time.Now()) {
                    return grant.AccessLevel, "role_grant", nil
                }
            }
        }
    }

    return "", "", errors.New("no access")
}
```

### 4. Moonlight Web Integration

The streaming token is passed to moonlight-web which validates it before allowing access to the Wolf lobby.

**Modified moonlight_proxy.go**:
```go
func (apiServer *HelixAPIServer) proxyToMoonlightWeb(w http.ResponseWriter, r *http.Request) {
    // Extract stream token from query params
    streamToken := r.URL.Query().Get("token")

    // Validate token
    claims, err := apiServer.validateStreamingToken(streamToken)
    if err != nil {
        http.Error(w, "Invalid streaming token", http.StatusUnauthorized)
        return
    }

    // Add token claims as headers for moonlight-web
    r.Header.Set("X-Helix-User-ID", claims.UserID)
    r.Header.Set("X-Helix-Session-ID", claims.SessionID)
    r.Header.Set("X-Helix-Access-Level", claims.AccessLevel)

    // Proxy to moonlight-web
    target, _ := url.Parse("http://moonlight-web:8080")
    proxy := httputil.NewSingleHostReverseProxy(target)

    // Remove /moonlight prefix
    r.URL.Path = strings.TrimPrefix(r.URL.Path, "/moonlight")

    proxy.ServeHTTP(w, r)
}
```

## Access Level Enforcement

### View-Only Mode

For users with `view` access level:
- Video stream enabled
- Audio stream enabled (receive only)
- Mouse/keyboard input **disabled**
- Gamepad input **disabled**
- Implementation: moonlight-web modified to check `X-Helix-Access-Level` header

### Control Mode

For users with `control` access level:
- Full video/audio streaming
- Full input control (mouse, keyboard, gamepad, touch)
- Cannot modify stream settings
- Cannot terminate session

### Admin Mode

For users with `admin` access level (owner):
- All control mode permissions
- Can modify stream settings (bitrate, resolution, FPS)
- Can terminate streaming session
- Can grant/revoke access to others

## Session Sharing UI

### Share Button in ScreenshotViewer

```typescript
// In ScreenshotViewer.tsx
<IconButton onClick={() => setShareDialogOpen(true)} title="Share Session">
  <Share fontSize="small" />
</IconButton>

<StreamingSharingDialog
  open={shareDialogOpen}
  onClose={() => setShareDialogOpen(false)}
  sessionId={sessionId}
  isPersonalDevEnvironment={isPersonalDevEnvironment}
/>
```

### StreamingSharingDialog Component

```typescript
interface StreamingSharingDialogProps {
  sessionId: string;
  isPersonalDevEnvironment: boolean;
  open: boolean;
  onClose: () => void;
}

const StreamingSharingDialog: React.FC<StreamingSharingDialogProps> = ({
  sessionId,
  isPersonalDevEnvironment,
  open,
  onClose,
}) => {
  // Fetch current grants
  const { data: grants } = useQuery({
    queryKey: ['streaming-grants', sessionId],
    queryFn: () => apiClient.v1SessionsStreamingAccessList(sessionId),
  });

  // Grant access mutation
  const grantMutation = useMutation({
    mutationFn: (request: {
      granted_user_id?: string;
      granted_team_id?: string;
      granted_role?: string;
      access_level: 'view' | 'control' | 'admin';
      expires_at?: string;
    }) => apiClient.v1SessionsStreamingAccessCreate(sessionId, request),
    onSuccess: () => {
      queryClient.invalidateQueries(['streaming-grants', sessionId]);
    },
  });

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Share Streaming Session</DialogTitle>
      <DialogContent>
        {/* Current Grants List */}
        <Typography variant="h6">Current Access</Typography>
        <List>
          {grants?.map((grant) => (
            <ListItem key={grant.id}>
              <ListItemText
                primary={grant.granted_user_name || grant.granted_team_id || grant.granted_role}
                secondary={`${grant.access_level} access - granted ${formatDate(grant.granted_at)}`}
              />
              <ListItemSecondaryAction>
                <IconButton onClick={() => revokeMutation.mutate(grant.id)}>
                  <Delete />
                </IconButton>
              </ListItemSecondaryAction>
            </ListItem>
          ))}
        </List>

        {/* Grant New Access Form */}
        <Divider sx={{ my: 2 }} />
        <Typography variant="h6">Grant New Access</Typography>
        <Box component="form" onSubmit={handleGrantAccess}>
          <FormControl fullWidth sx={{ mb: 2 }}>
            <InputLabel>Share With</InputLabel>
            <Select value={grantType} onChange={(e) => setGrantType(e.target.value)}>
              <MenuItem value="user">Specific User</MenuItem>
              <MenuItem value="team">Entire Team</MenuItem>
              <MenuItem value="role">Role-Based</MenuItem>
            </Select>
          </FormControl>

          {grantType === 'user' && (
            <Autocomplete
              options={teamMembers}
              getOptionLabel={(user) => user.email}
              renderInput={(params) => <TextField {...params} label="User Email" />}
              onChange={(_, user) => setSelectedUser(user)}
            />
          )}

          {grantType === 'team' && (
            <Select fullWidth value={selectedTeam} onChange={(e) => setSelectedTeam(e.target.value)}>
              {teams.map((team) => (
                <MenuItem key={team.id} value={team.id}>
                  {team.name}
                </MenuItem>
              ))}
            </Select>
          )}

          {grantType === 'role' && (
            <TextField
              fullWidth
              label="Role Name"
              value={selectedRole}
              onChange={(e) => setSelectedRole(e.target.value)}
              helperText="E.g., 'manager', 'engineering', 'support'"
            />
          )}

          <FormControl fullWidth sx={{ my: 2 }}>
            <InputLabel>Access Level</InputLabel>
            <Select value={accessLevel} onChange={(e) => setAccessLevel(e.target.value)}>
              <MenuItem value="view">
                View Only (Read-only stream, no input)
              </MenuItem>
              <MenuItem value="control">
                Control (Full input, cannot modify settings)
              </MenuItem>
              <MenuItem value="admin">
                Admin (Full control, can grant access)
              </MenuItem>
            </Select>
          </FormControl>

          <TextField
            fullWidth
            type="datetime-local"
            label="Expires At (Optional)"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            helperText="Leave empty for permanent access"
          />

          <Button
            type="submit"
            variant="contained"
            fullWidth
            sx={{ mt: 2 }}
            disabled={grantMutation.isLoading}
          >
            Grant Access
          </Button>
        </Box>
      </DialogContent>
    </Dialog>
  );
};
```

## Security Considerations

### 1. Token Security

- **Short-lived**: Tokens expire after 1 hour (force re-authentication)
- **Signed**: JWT tokens signed with server secret
- **Non-transferable**: Tied to specific user ID and session ID
- **Audited**: All token issuance logged

### 2. Wolf Lobby PIN Protection

- Each Wolf lobby has unique 4-digit PIN
- PIN only shared via authenticated Helix API
- PIN changes on lobby recreation (prevents stale access)
- moonlight-web validates PIN before streaming

### 3. Network Security

- All streaming traffic over HTTPS/WSS in production
- WebRTC encrypted via DTLS-SRTP
- Reverse proxy ensures moonlight-web not directly accessible

### 4. Access Revocation

- Revoking access grant immediately prevents new tokens
- Existing tokens expire within 1 hour
- Emergency revocation: Change Wolf lobby PIN (requires lobby restart)

## Backward Compatibility

### Existing Sessions

Sessions without RBAC configured:
- Owner has `admin` access (backward compatible)
- No access grants by default
- Can add grants at any time

### Migration Path

1. **Phase 1**: Add RBAC tables (migrations)
2. **Phase 2**: Implement access checking (defaults to owner-only)
3. **Phase 3**: Add UI for sharing (opt-in)
4. **Phase 4**: Add audit logging (always enabled for new sessions)

## Production Deployment Checklist

- [ ] Database migrations applied
- [ ] Streaming token JWT secret configured
- [ ] Audit log retention policy defined
- [ ] Access grant expiration cleanup job scheduled
- [ ] Monitoring alerts for unauthorized access attempts
- [ ] Documentation for admins on managing access
- [ ] GDPR compliance review (audit log PII handling)

## Future Enhancements

1. **Temporary Access Links**: Generate shareable links with embedded tokens
2. **Recording Permissions**: Separate role for screen recording access
3. **Bandwidth Limits**: Enforce different bitrates based on user tier
4. **Concurrent Viewer Limit**: Limit simultaneous viewers per session
5. **Watermarking**: Add user identifier watermark for compliance

---

**Last Updated**: 2025-10-08
**Status**: Design Complete - Ready for Implementation
