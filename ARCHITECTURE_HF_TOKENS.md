# Hugging Face Token Architecture

## Current Implementation (Global)

The current implementation provides a global Hugging Face token that applies to all users and organizations:

- **Storage**: `SystemSettings.HuggingFaceToken` in database
- **Management**: Admin-only API endpoints at `/api/v1/system/settings`
- **Distribution**: Control plane pushes token to all runners via `/api/v1/system/config`
- **Usage**: All model instances use the same global token

## Future Extensions (Per-Org/Per-User)

The architecture is designed to naturally extend for per-organization and per-user tokens:

### Database Schema Extensions
```sql
-- Add to existing Organization table
ALTER TABLE organizations ADD COLUMN huggingface_token TEXT;

-- Add to existing User table  
ALTER TABLE users ADD COLUMN huggingface_token TEXT;

-- Future: Model-specific tokens
ALTER TABLE models ADD COLUMN huggingface_token TEXT;
```

### Token Resolution Hierarchy
Priority order for token resolution:
1. **Model-specific token** (highest priority)
2. **User-specific token** 
3. **Organization-specific token**
4. **Global system token** (current implementation)
5. **Environment variable** (backward compatibility, lowest priority)

### API Extensions
```typescript
// Extended system settings request
interface SystemSettingsRequest {
  huggingface_token?: string;        // Global (current)
  org_hf_tokens?: Record<string, string>;     // Per-org (future)
  user_hf_tokens?: Record<string, string>;    // Per-user (future)
}

// Extended runner config request
interface RunnerSystemConfigRequest {
  huggingface_token?: string;        // Global fallback
  user_id?: string;                  // Context for user token
  organization_id?: string;          // Context for org token
  user_hf_token?: string;           // User-specific token
  org_hf_token?: string;            // Org-specific token
}
```

### Implementation Strategy

#### Phase 1: Global Token (Current)
- ✅ Global system settings
- ✅ Runner distribution mechanism
- ✅ Runtime integration

#### Phase 2: Per-Organization Tokens
- Add `huggingface_token` to `Organization` table
- Update organization management UI/API
- Modify token resolution in scheduler to check user's organization
- Update runner config to include org context

#### Phase 3: Per-User Tokens  
- Add `huggingface_token` to `User` table
- Update user profile management
- Modify token resolution to check user-specific token first
- Update runner config to include user context

#### Phase 4: Per-Model Tokens
- Add `huggingface_token` to `Model` table (or `RuntimeArgs`)
- Update model management UI/CLI
- Modify token resolution to check model-specific token first
- Support model-specific tokens in slot creation

### Key Design Benefits

1. **Backward Compatibility**: Environment variables continue to work
2. **Incremental Migration**: Can be implemented phase by phase
3. **Clear Hierarchy**: Predictable token resolution order
4. **Minimal Changes**: Existing code structure supports extensions
5. **Security**: Tokens are never logged, only "token_provided" boolean

### Migration Path

Users can migrate incrementally:
1. Start with environment variables (legacy)
2. Move to global system token (current)
3. Add per-org tokens (future)
4. Add per-user tokens (future)
5. Add per-model tokens (future)

Each level overrides the lower levels, allowing gradual adoption.
