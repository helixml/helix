# Zed Agent & Personal Development Environment Roadmap

## Current Status
- ✅ Wolf integration working with complete GStreamer pipelines
- ✅ Personal Dev environment creation working
- ✅ Apps appear in Moonlight client
- ✅ Upstream Wolf image compatible (no custom modifications needed)

## Outstanding Issues

### 1. Wolf Session Auto-Creation Problem
**Issue**: Something weird happens when we start the app in Wolf without the user actually loading it via the Moonlight client.

**Current Workaround**: Disabled automatic session creation in `wolf_executor.go`. Apps are created in Wolf but sessions are not auto-started.

**Required Solution**:
- Investigate why auto-creating Wolf sessions causes issues
- Determine if this is a Wolf limitation or our implementation
- May need to implement session creation on-demand when Moonlight client connects
- Consider if this affects streaming latency or user experience

**Priority**: Medium - affects user experience but has workaround

### 2. Authentication Integration
**Issue**: Personal Dev environment API requires proper authentication tokens, making testing difficult.

**Solution Needed**:
- Implement proper token-based testing mechanisms
- Document authentication flow for Personal Dev environments
- Ensure API works with frontend authentication

**Priority**: Low - mainly affects development workflow

### 3. Zed Agent Multi-Session Support
**Status**: Partially implemented but not tested

**Requirements**:
- Multiple Zed threads within single Personal Dev environment
- Proper thread lifecycle management
- Resource isolation between threads

**Priority**: Future enhancement

### 4. Persistent Workspace Enhancements
**Current**: Basic workspace directory creation with startup scripts

**Enhancements Needed**:
- Pre-installed development tools (beyond JupyterLab/OnlyOffice)
- Language-specific environments (Node.js, Python, Go, etc.)
- Git integration and SSH key management
- VS Code server integration alongside Zed

**Priority**: Enhancement

## Technical Debt

### Wolf Integration
- ✅ Removed dependency on custom Wolf modifications
- ✅ Using upstream `ghcr.io/games-on-whales/wolf:stable`
- ✅ Complete GStreamer pipeline configuration implemented

### Code Quality
- Add proper error handling for Wolf API failures
- Implement retry logic for Wolf session operations
- Add metrics/monitoring for Personal Dev environment usage
- Clean up temporary session creation disable

## Testing Strategy

### Manual Testing Checklist
1. ✅ Personal Dev environment creation via API
2. ✅ Apps appear in Moonlight client
3. 🔄 Moonlight client can connect and stream (needs verification)
4. ⏳ Workspace persistence across restarts
5. ⏳ Multiple environments per user
6. ⏳ Resource cleanup on environment deletion

### Automated Testing Needs
- Integration tests for Wolf API interactions
- End-to-end streaming tests with Moonlight
- Resource leak detection for long-running environments

## Documentation Needed
- Personal Dev environment user guide
- API documentation for Personal Dev endpoints
- Wolf integration architecture overview
- Troubleshooting guide for streaming issues

---

**Last Updated**: 2025-09-27
**Status**: Active development, main functionality working