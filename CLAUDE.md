# Claude Development Notes

## Automated Build and Deploy System

### Single-Command Build & Deploy
The HyprMoon build system now features **automatic deployment**:

```bash
cd /path/to/hyprmoon
./build.sh
```

This single command will:
1. **Build** the HyprMoon package with your changes
2. **Auto-deploy** to the adjacent Helix directory (`../helix`)
3. **Update** Dockerfile.zed-agent-vnc with new version numbers
4. **Rebuild** the Docker container with new packages
5. **Restart** the container if it was running

### Project Structure (Flexible)
```
parent-dir/
├── hyprmoon/          # HyprMoon development repo
│   ├── build.sh       # Automated build & deploy script
│   └── ...
└── helix/             # Helix deployment repo (auto-detected)
    ├── Dockerfile.zed-agent-vnc  # Auto-updated with versions
    └── ...
```

### Version Management
- **IMPORTANT**: Always commit when bumping the version
- The build system automatically handles version progression and deployment
- Dockerfile version references are updated automatically
- No manual copying or container rebuilding required

### Build Features
- **Stale Build Detection**: Detects if changes aren't being compiled
- **Smart Cache Management**: Use `FORCE_CLEAN=1 ./build.sh` for clean builds
- **Progress Tracking**: Clear status updates throughout the process
- **Error Handling**: Detailed error reporting and rollback

## Recent Work (Certificate Fix - Step 8.9.10)
- **CRITICAL FIX**: Connected OpenSSL certificate generation to Wolf AppState
- Fixed empty `<plaincert/>` field in Moonlight pairing Phase 1 responses
- Added `loadCertificatesIntoAppState()` method for proper certificate loading
- Implemented auto-deploy system for seamless development workflow
- Fixed Moonlight pairing protocol integration with Wolf reference implementation