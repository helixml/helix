# Architecture Comparison: Zed Settings Sync with MCP Integration

**Status**: Design Analysis
**Created**: 2025-10-08
**Context**: Addressing settings conflict resolution and K8s deployment concerns

## Table of Contents
1. [Problem Statement](#problem-statement)
2. [Key Constraints](#key-constraints)
3. [Architecture Options](#architecture-options)
4. [Detailed Comparison](#detailed-comparison)
5. [Recommendation](#recommendation)
6. [Implementation Details](#implementation-details)

---

## 1. Problem Statement

### The Settings Conflict Problem

**Scenario**:
- Helix wants to sync MCP servers from app config → Zed settings.json
- User can modify settings (including MCPs) via Zed UI
- **Who wins?** Two-way sync creates conflict potential

**Example Conflict**:
```
T0: Helix writes settings.json with MCP server "helix-rag"
T1: User adds MCP server "local-tools" via Zed UI → modifies settings.json
T2: Helix detects app config change, regenerates settings.json
T3: User's "local-tools" is now deleted! 😱
```

### The K8s Deployment Problem

**Current Architecture Issues**:
- Bind mounts tie containers to specific host filesystems
- `/opt/helix/wolf/zed-config/{instance_id}` assumes shared filesystem
- In K8s: API pod on Node A, Wolf container on Node B → **bind mount fails**
- Need network-based solution, not filesystem-based

**K8s Reality**:
```
┌─────────────┐         ┌─────────────┐
│   Node A    │         │   Node B    │
│             │         │             │
│  Helix API  │         │  Wolf Pod   │
│  (writes    │    X    │  (needs     │
│   config)   │         │   config)   │
└─────────────┘         └─────────────┘
     ↓                       ↓
Different filesystems - bind mount impossible
```

---

## 2. Key Constraints

### Must-Have Requirements

1. **Bidirectional Sync**: Both Helix and Zed can modify settings
2. **Conflict Resolution**: Deterministic merge strategy
3. **K8s Compatible**: No shared filesystem assumptions
4. **Minimal Zed Changes**: Reuse existing Zed patterns
5. **Fast Updates**: < 100ms for settings propagation
6. **Reliable**: Settings sync failures don't break Zed

### Nice-to-Have

1. **Hot Reload**: Settings update without container restart
2. **Audit Trail**: Track who changed what
3. **Rollback**: Revert to previous settings
4. **Validation**: Prevent invalid configurations

---

## 3. Architecture Options

### Option A: Settings Sync Daemon (Sidecar Pattern)

**Pattern**: Copy screenshot server architecture - Go daemon in container

```
┌────────────────────────────────────────────────┐
│         Wolf Container (K8s Pod)               │
│                                                 │
│  ┌──────────────────────────────────────────┐ │
│  │  settings-sync-daemon                     │ │
│  │  - Listens on :9877                       │ │
│  │  - Manages settings.json                  │ │
│  │  - Watches for Zed changes (inotify)     │ │
│  │  - Polls Helix API for updates            │ │
│  └──────────────────────────────────────────┘ │
│           ↓                    ↑               │
│  ┌──────────────────────────────────────────┐ │
│  │  /home/retro/.config/zed/settings.json   │ │
│  └──────────────────────────────────────────┘ │
│           ↓                                    │
│  ┌──────────────────────────────────────────┐ │
│  │  Zed Editor                               │ │
│  │  - Reads settings.json on startup        │ │
│  │  - Writes settings.json on UI changes    │ │
│  │  - Reloads on file change notification   │ │
│  └──────────────────────────────────────────┘ │
└────────────────────────────────────────────────┘
                    ↕ HTTP (K8s Service DNS)
┌────────────────────────────────────────────────┐
│         Helix API (Different K8s Node)         │
│                                                 │
│  GET /api/v1/apps/{id}/zed-settings            │
│  - Returns merged MCP config                   │
│  - Includes Helix-managed + user additions     │
│                                                 │
│  POST /api/v1/apps/{id}/zed-settings           │
│  - Receives user changes from daemon           │
│  - Merges with app config                      │
└────────────────────────────────────────────────┘
```

**Pros**:
- ✅ K8s compatible (no filesystem dependencies)
- ✅ Familiar pattern (screenshot server)
- ✅ Fast local file access in container
- ✅ Can watch Zed changes via inotify
- ✅ Simple HTTP API for Helix communication

**Cons**:
- ❌ Requires new daemon binary
- ❌ File watching complexity (inotify)
- ❌ Race conditions possible (file writes)

---

### Option B: Helix API as Settings Server (Pull Pattern)

**Pattern**: Zed pulls settings from Helix on startup/reload

```
┌────────────────────────────────────────────────┐
│         Wolf Container (K8s Pod)               │
│                                                 │
│  ┌──────────────────────────────────────────┐ │
│  │  Zed Editor                               │ │
│  │                                            │ │
│  │  On startup:                              │ │
│  │  1. GET helix-api:8080/settings.json     │ │
│  │  2. Merge with local settings.json       │ │
│  │  3. Write merged result                   │ │
│  │                                            │ │
│  │  On UI change:                            │ │
│  │  1. Write local settings.json             │ │
│  │  2. POST helix-api:8080/settings/update  │ │
│  └──────────────────────────────────────────┘ │
└────────────────────────────────────────────────┘
                    ↕ HTTP
┌────────────────────────────────────────────────┐
│         Helix API                              │
│                                                 │
│  GET /zed-settings/{instance_id}               │
│  - Returns: {helix_managed: {...}, schema}    │
│                                                 │
│  POST /zed-settings/{instance_id}/user         │
│  - Accepts: {user_additions: {...}}           │
│  - Stores in database                          │
└────────────────────────────────────────────────┘
```

**Pros**:
- ✅ K8s compatible (HTTP only)
- ✅ No daemon needed
- ✅ Clear ownership model
- ✅ Zed pulls settings = Zed controls timing

**Cons**:
- ❌ **Requires Zed code changes** (custom settings provider)
- ❌ No automatic sync (Zed restart needed)
- ❌ More complex Zed integration

---

### Option C: Helix CLI as Settings Proxy (Hybrid)

**Pattern**: helix-cli runs in container, acts as settings middleware

```
┌────────────────────────────────────────────────┐
│         Wolf Container                         │
│                                                 │
│  ┌──────────────────────────────────────────┐ │
│  │  helix-cli settings-daemon                │ │
│  │  - Exposes settings.json via HTTP         │ │
│  │  - Caches Helix settings locally          │ │
│  │  - Merges with user overrides             │ │
│  │  - Syncs changes back to API              │ │
│  └──────────────────────────────────────────┘ │
│           ↓ (generates)                        │
│  ┌──────────────────────────────────────────┐ │
│  │  /tmp/zed-settings.json (tmpfs)           │ │
│  └──────────────────────────────────────────┘ │
│           ↓ (bind mount)                       │
│  ┌──────────────────────────────────────────┐ │
│  │  Zed sees as: ~/.config/zed/settings.json│ │
│  └──────────────────────────────────────────┘ │
└────────────────────────────────────────────────┘
```

**Pros**:
- ✅ K8s compatible
- ✅ Reuses helix-cli (already in containers)
- ✅ tmpfs avoids disk I/O
- ✅ CLI can handle all merge logic

**Cons**:
- ❌ Still has bind mount (but to tmpfs in same container)
- ❌ Read-only mount prevents Zed UI changes
- ❌ Doesn't solve bidirectional sync

---

### Option D: Settings ConfigMap with Init Container (K8s Native)

**Pattern**: Use K8s ConfigMap + init container for settings

```
┌────────────────────────────────────────────────┐
│         K8s ConfigMap                          │
│  zed-settings-{instance-id}:                   │
│    settings.json: |                            │
│      {context_servers: {...}}                  │
└────────────────────────────────────────────────┘
                    ↓ (mounted as volume)
┌────────────────────────────────────────────────┐
│         Wolf Pod                               │
│                                                 │
│  Init Container: settings-merger               │
│  1. Read ConfigMap → /config/helix.json        │
│  2. Read user settings → /config/user.json     │
│  3. Merge → /config/merged/settings.json       │
│                                                 │
│  Main Container: Zed                           │
│  - Reads /config/merged/settings.json          │
│  - Writes changes to /config/user.json         │
│  - Triggers ConfigMap update via API           │
└────────────────────────────────────────────────┘
```

**Pros**:
- ✅ K8s native (ConfigMaps designed for this)
- ✅ No bind mounts across nodes
- ✅ Version control (ConfigMap revisions)
- ✅ Can use K8s watch for updates

**Cons**:
- ❌ **Only works in K8s** (not docker-compose dev)
- ❌ ConfigMap size limits (1MB)
- ❌ Init container restart required for updates
- ❌ Overly complex for simple settings sync

---

### Option E: WebSocket Settings Stream (Real-time Sync)

**Pattern**: Bidirectional WebSocket for real-time settings sync

```
┌────────────────────────────────────────────────┐
│         Wolf Container                         │
│                                                 │
│  ┌──────────────────────────────────────────┐ │
│  │  settings-sync-client (Go daemon)         │ │
│  │  - WebSocket to Helix API                 │ │
│  │  - Receives: helix_settings updates       │ │
│  │  - Sends: user_settings changes           │ │
│  │  - Writes merged settings.json            │ │
│  │  - Notifies Zed on changes (SIGHUP)       │ │
│  └──────────────────────────────────────────┘ │
│           ↓                                    │
│  ┌──────────────────────────────────────────┐ │
│  │  Zed Editor                               │ │
│  │  - Reloads settings on SIGHUP             │ │
│  │  - Writes settings.json on UI changes     │ │
│  └──────────────────────────────────────────┘ │
└────────────────────────────────────────────────┘
           ↕ WebSocket (wss://api:8080/ws/settings)
┌────────────────────────────────────────────────┐
│         Helix API                              │
│                                                 │
│  WebSocket Handler:                            │
│  - Broadcasts app config changes              │
│  - Receives user setting updates              │
│  - Stores user overrides in DB                │
└────────────────────────────────────────────────┘
```

**Pros**:
- ✅ Real-time bidirectional sync
- ✅ K8s compatible (WebSocket = HTTP upgrade)
- ✅ Instant updates (no polling)
- ✅ Connection status visible (connected/disconnected)

**Cons**:
- ❌ More complex (WebSocket management)
- ❌ Reconnection logic needed
- ❌ State synchronization on reconnect
- ❌ Overkill for settings sync?

---

## 4. Detailed Comparison

### 4.1 Conflict Resolution Strategies

All options need a merge strategy. Here's the recommended approach:

**Three-Way Merge Model**:
```
Base Settings (Helix-managed, read-only):
{
  "context_servers": {
    "helix-rag": {...},      // From app.config.helix RAG
    "helix-api": {...}       // From app.config.helix APIs
  }
}

User Overrides (user-managed, writable):
{
  "context_servers": {
    "local-tools": {...},    // User added via Zed UI
    "helix-rag": {           // User modified Helix tool
      "enabled": false       // Override: disable this tool
    }
  },
  "theme": "dark",           // Non-MCP settings
  "vim_mode": true
}

Merged Result (presented to Zed):
{
  "context_servers": {
    "helix-rag": {           // Merged: Helix base + user override
      ...helix_config,
      "enabled": false       // User's override wins
    },
    "helix-api": {...},      // Helix-managed, no override
    "local-tools": {...}     // User-added, preserved
  },
  "theme": "dark",
  "vim_mode": true
}
```

**Merge Rules**:
1. **Helix namespace** (`helix-*`): Helix controls structure, user can disable/override
2. **User namespace** (any other name): User has full control
3. **Conflicts**: User settings override Helix for same key
4. **Deletions**: User can't delete Helix tools, only disable

**Storage**:
```go
// In database
type ZedSettingsOverride struct {
    InstanceID   string
    UserID       string
    Overrides    map[string]interface{} // User's custom settings
    UpdatedAt    time.Time
}

// In API
func GetMergedSettings(instanceID string) (*ZedSettings, error) {
    helix := GenerateHelixManagedSettings(app)
    user := GetUserOverrides(instanceID)
    return MergeSettings(helix, user), nil
}
```

### 4.2 Performance Comparison

| Option | Startup Time | Update Latency | Memory Overhead | Network Calls |
|--------|--------------|----------------|-----------------|---------------|
| **A: Daemon** | +50ms (daemon start) | 10-50ms (inotify) | +10MB (Go process) | Poll every 30s |
| **B: Pull** | +100ms (HTTP fetch) | N/A (restart only) | 0 (no daemon) | On startup only |
| **C: CLI Proxy** | +30ms (CLI start) | 50-100ms (file regen) | +8MB (helix-cli) | Poll every 30s |
| **D: ConfigMap** | +200ms (init container) | N/A (pod restart) | 0 | On pod start |
| **E: WebSocket** | +100ms (WS connect) | 5-20ms (real-time) | +12MB (WS client) | Persistent connection |

### 4.3 K8s Compatibility Matrix

| Option | Docker Compose | K8s Single Node | K8s Multi-Node | Complexity |
|--------|----------------|-----------------|----------------|------------|
| **A: Daemon** | ✅ Perfect | ✅ Works | ✅ Works | Medium |
| **B: Pull** | ✅ Perfect | ✅ Works | ✅ Works | Low (but needs Zed changes) |
| **C: CLI Proxy** | ✅ Perfect | ✅ Works | ⚠️ Tmpfs bind mount | Medium |
| **D: ConfigMap** | ❌ K8s only | ✅ Works | ✅ Works | High |
| **E: WebSocket** | ✅ Perfect | ✅ Works | ✅ Works | High |

### 4.4 Failure Mode Analysis

**What happens when...**

| Scenario | Option A (Daemon) | Option B (Pull) | Option E (WebSocket) |
|----------|-------------------|-----------------|----------------------|
| **API unreachable** | Uses cached settings | Zed fails to start | Reconnects, uses cache |
| **Daemon crashes** | Settings frozen until restart | N/A | Settings frozen until restart |
| **Network partition** | Stale settings until reconnect | Zed uses last-known | Auto-reconnect, eventual consistency |
| **Concurrent writes** | Last write wins (file lock) | Merge on next pull | Real-time conflict resolution |
| **Invalid settings** | Daemon validates, rejects | API validates, returns 400 | WS rejects invalid update |

---

## 5. Recommendation

### **Winner: Option A - Settings Sync Daemon** 🏆

**Why this is the best choice**:

1. ✅ **Proven Pattern**: Screenshot server shows it works
2. ✅ **K8s Ready**: HTTP communication, no filesystem coupling
3. ✅ **Bidirectional Sync**: Handles both Helix → Zed and Zed → Helix
4. ✅ **No Zed Changes**: Zed just reads/writes settings.json as normal
5. ✅ **Fast & Reliable**: Local file access, inotify for instant detection
6. ✅ **Simple Architecture**: Single daemon, clear responsibilities

**Why not the others**:
- **Option B**: Requires Zed modifications (harder to maintain)
- **Option C**: Read-only settings.json = no Zed UI customization
- **Option D**: K8s-only, too complex, not for dev environment
- **Option E**: Overkill complexity for settings sync

### Refined Daemon Architecture

```
┌──────────────────────────────────────────────────────────┐
│  settings-sync-daemon (Go binary in container)           │
│                                                           │
│  Components:                                              │
│  1. File Watcher (inotify)                               │
│     - Watches: ~/.config/zed/settings.json               │
│     - Debounce: 500ms (avoid rapid fire)                 │
│     - On change: Extract user overrides → POST to API    │
│                                                           │
│  2. HTTP Client (to Helix API)                           │
│     - Poll: Every 30s for app config changes             │
│     - Endpoint: GET /api/v1/sessions/{id}/zed-config     │
│     - On change: Merge with user overrides → Write file  │
│                                                           │
│  3. Merge Engine                                          │
│     - Strategy: Helix base + User overrides              │
│     - Namespaces: helix-* vs user-*                      │
│     - Validation: JSON schema check before write         │
│                                                           │
│  4. HTTP Server (for API communication)                  │
│     - GET /health → readiness probe                      │
│     - GET /settings → current merged settings            │
│     - POST /reload → force refresh from API              │
│                                                           │
│  Startup Flow:                                            │
│  1. Fetch Helix config from API                          │
│  2. Load user overrides from settings.json (if exists)   │
│  3. Merge and write settings.json                        │
│  4. Start file watcher                                    │
│  5. Start polling loop                                    │
└──────────────────────────────────────────────────────────┘
```

---

## 6. Implementation Details

### 6.1 Daemon Binary Structure

**Location**: `/home/luke/pm/helix/api/cmd/settings-sync-daemon/main.go`

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/helixml/helix/api/pkg/client"
)

const (
    SettingsPath = "/home/retro/.config/zed/settings.json"
    PollInterval = 30 * time.Second
    DebounceTime = 500 * time.Millisecond
)

type SettingsDaemon struct {
    helixClient  *client.Client
    sessionID    string
    watcher      *fsnotify.Watcher
    lastModified time.Time

    // Current state
    helixSettings map[string]interface{}
    userOverrides map[string]interface{}
}

func main() {
    // Environment variables
    helixURL := os.Getenv("HELIX_API_URL")      // "api:8080"
    helixToken := os.Getenv("HELIX_API_TOKEN")  // Runner token
    sessionID := os.Getenv("HELIX_SESSION_ID")  // Session ID
    port := os.Getenv("SETTINGS_SYNC_PORT")     // "9877"

    if port == "" {
        port = "9877"
    }

    // Create Helix API client
    helixClient, err := client.NewClient(context.Background(), &client.ClientOptions{
        URL:   helixURL,
        Token: helixToken,
    })
    if err != nil {
        log.Fatalf("Failed to create Helix client: %v", err)
    }

    daemon := &SettingsDaemon{
        helixClient: helixClient,
        sessionID:   sessionID,
    }

    // Initial sync from Helix → local
    if err := daemon.syncFromHelix(); err != nil {
        log.Printf("Warning: Initial sync failed: %v", err)
    }

    // Start file watcher for Zed changes
    if err := daemon.startWatcher(); err != nil {
        log.Fatalf("Failed to start file watcher: %v", err)
    }

    // Start polling loop for Helix changes
    go daemon.pollHelixChanges()

    // HTTP server for health checks and manual triggers
    http.HandleFunc("/health", daemon.healthCheck)
    http.HandleFunc("/settings", daemon.getSettings)
    http.HandleFunc("/reload", daemon.forceReload)

    log.Printf("Settings sync daemon listening on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}

// syncFromHelix fetches Helix-managed settings and merges with user overrides
func (d *SettingsDaemon) syncFromHelix() error {
    ctx := context.Background()

    // Fetch Helix-managed config
    resp, err := d.helixClient.GetZedConfig(ctx, d.sessionID)
    if err != nil {
        return fmt.Errorf("failed to fetch Helix config: %w", err)
    }

    d.helixSettings = resp.Settings

    // Load existing user settings (if file exists)
    if _, err := os.Stat(SettingsPath); err == nil {
        data, _ := os.ReadFile(SettingsPath)
        var current map[string]interface{}
        if json.Unmarshal(data, &current) == nil {
            d.userOverrides = extractUserOverrides(current, d.helixSettings)
        }
    }

    // Merge and write
    merged := mergeSettings(d.helixSettings, d.userOverrides)
    return d.writeSettings(merged)
}

// mergeSettings applies three-way merge: Helix base + User overrides
func mergeSettings(helix, user map[string]interface{}) map[string]interface{} {
    merged := make(map[string]interface{})

    // Copy Helix settings as base
    for k, v := range helix {
        merged[k] = v
    }

    // Apply user overrides (deep merge for context_servers)
    if userServers, ok := user["context_servers"].(map[string]interface{}); ok {
        if helixServers, ok := merged["context_servers"].(map[string]interface{}); ok {
            // Deep merge context_servers
            for name, config := range userServers {
                helixServers[name] = config // User override/addition wins
            }
        } else {
            merged["context_servers"] = userServers
        }
    }

    // Apply other user settings (non-context_servers)
    for k, v := range user {
        if k != "context_servers" {
            merged[k] = v
        }
    }

    return merged
}

// extractUserOverrides finds settings that differ from Helix base
func extractUserOverrides(current, helix map[string]interface{}) map[string]interface{} {
    overrides := make(map[string]interface{})

    // Extract user-added context_servers (not in helix namespace)
    if currentServers, ok := current["context_servers"].(map[string]interface{}); ok {
        helixServers, _ := helix["context_servers"].(map[string]interface{})
        userServers := make(map[string]interface{})

        for name, config := range currentServers {
            // If not in Helix config, or user modified it
            if helixConfig, inHelix := helixServers[name]; !inHelix {
                userServers[name] = config // User addition
            } else if !deepEqual(config, helixConfig) {
                userServers[name] = config // User modification
            }
        }

        if len(userServers) > 0 {
            overrides["context_servers"] = userServers
        }
    }

    // Extract other user settings (theme, vim_mode, etc.)
    for k, v := range current {
        if k == "context_servers" {
            continue
        }
        if helixVal, inHelix := helix[k]; !inHelix || !deepEqual(v, helixVal) {
            overrides[k] = v
        }
    }

    return overrides
}

// startWatcher monitors settings.json for Zed UI changes
func (d *SettingsDaemon) startWatcher() error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }
    d.watcher = watcher

    // Watch the settings file
    if err := watcher.Add(SettingsPath); err != nil {
        return err
    }

    go func() {
        var debounceTimer *time.Timer

        for {
            select {
            case event := <-watcher.Events:
                if event.Op&fsnotify.Write == fsnotify.Write {
                    // Debounce rapid writes
                    if debounceTimer != nil {
                        debounceTimer.Stop()
                    }
                    debounceTimer = time.AfterFunc(DebounceTime, func() {
                        d.onFileChanged()
                    })
                }
            case err := <-watcher.Errors:
                log.Printf("Watcher error: %v", err)
            }
        }
    }()

    return nil
}

// onFileChanged handles Zed UI modifications to settings.json
func (d *SettingsDaemon) onFileChanged() {
    // Prevent re-triggering on our own writes
    if time.Since(d.lastModified) < 1*time.Second {
        return
    }

    log.Println("Detected settings.json change from Zed UI")

    // Read current settings
    data, err := os.ReadFile(SettingsPath)
    if err != nil {
        log.Printf("Failed to read settings: %v", err)
        return
    }

    var current map[string]interface{}
    if err := json.Unmarshal(data, &current); err != nil {
        log.Printf("Failed to parse settings: %v", err)
        return
    }

    // Extract user overrides
    d.userOverrides = extractUserOverrides(current, d.helixSettings)

    // Send to Helix API for persistence
    if err := d.syncToHelix(); err != nil {
        log.Printf("Failed to sync to Helix: %v", err)
    }
}

// syncToHelix sends user overrides to Helix API
func (d *SettingsDaemon) syncToHelix() error {
    ctx := context.Background()
    return d.helixClient.UpdateZedUserSettings(ctx, d.sessionID, d.userOverrides)
}

// pollHelixChanges checks for app config updates from Helix
func (d *SettingsDaemon) pollHelixChanges() {
    ticker := time.NewTicker(PollInterval)
    defer ticker.Stop()

    for range ticker.C {
        if err := d.checkHelixUpdates(); err != nil {
            log.Printf("Poll error: %v", err)
        }
    }
}

func (d *SettingsDaemon) checkHelixUpdates() error {
    ctx := context.Background()

    resp, err := d.helixClient.GetZedConfig(ctx, d.sessionID)
    if err != nil {
        return err
    }

    // Check if Helix settings changed
    if !deepEqual(resp.Settings, d.helixSettings) {
        log.Println("Detected Helix config change, updating settings.json")
        d.helixSettings = resp.Settings

        // Merge with user overrides and write
        merged := mergeSettings(d.helixSettings, d.userOverrides)
        if err := d.writeSettings(merged); err != nil {
            return err
        }
    }

    return nil
}

// writeSettings atomically writes settings.json
func (d *SettingsDaemon) writeSettings(settings map[string]interface{}) error {
    // Ensure directory exists
    dir := filepath.Dir(SettingsPath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }

    // Marshal with indentation
    data, err := json.MarshalIndent(settings, "", "  ")
    if err != nil {
        return err
    }

    // Atomic write (write to temp file, then rename)
    tmpFile := SettingsPath + ".tmp"
    if err := os.WriteFile(tmpFile, data, 0644); err != nil {
        return err
    }

    if err := os.Rename(tmpFile, SettingsPath); err != nil {
        return err
    }

    d.lastModified = time.Now()
    log.Println("Updated settings.json")
    return nil
}

// HTTP handlers
func (d *SettingsDaemon) healthCheck(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func (d *SettingsDaemon) getSettings(w http.ResponseWriter, r *http.Request) {
    merged := mergeSettings(d.helixSettings, d.userOverrides)
    json.NewEncoder(w).Encode(merged)
}

func (d *SettingsDaemon) forceReload(w http.ResponseWriter, r *http.Request) {
    if err := d.syncFromHelix(); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Write([]byte("Reloaded"))
}

func deepEqual(a, b interface{}) bool {
    aJSON, _ := json.Marshal(a)
    bJSON, _ := json.Marshal(b)
    return string(aJSON) == string(bJSON)
}
```

### 6.2 Helix API Endpoints

**New endpoints needed**:

```go
// GET /api/v1/sessions/{id}/zed-config
func (s *HelixAPIServer) getZedConfig(res http.ResponseWriter, req *http.Request) (*ZedConfig, *system.HTTPError) {
    sessionID := mux.Vars(req)["id"]
    user := getRequestUser(req)

    // Get session to find app
    session, err := s.Store.GetSession(req.Context(), sessionID, user.ID)
    if err != nil {
        return nil, system.NewHTTPError404("session not found")
    }

    // Generate Helix-managed MCP config
    app, err := s.Store.GetApp(req.Context(), session.AppID)
    if err != nil {
        return nil, system.NewHTTPError500("failed to get app")
    }

    helixSettings := GenerateZedMCPConfig(app, user.ID, sessionID, token)

    return &ZedConfig{
        Settings:  helixSettings.ContextServers,
        Version:   app.Updated.Unix(),
        SessionID: sessionID,
    }, nil
}

// POST /api/v1/sessions/{id}/zed-config/user
func (s *HelixAPIServer) updateZedUserSettings(res http.ResponseWriter, req *http.Request) (*system.HTTPError) {
    sessionID := mux.Vars(req)["id"]
    user := getRequestUser(req)

    var userSettings map[string]interface{}
    if err := json.NewDecoder(req.Body).Decode(&userSettings); err != nil {
        return system.NewHTTPError400("invalid request body")
    }

    // Store user overrides in database
    override := &types.ZedSettingsOverride{
        SessionID: sessionID,
        UserID:    user.ID,
        Overrides: userSettings,
        UpdatedAt: time.Now(),
    }

    if err := s.Store.UpsertZedSettingsOverride(req.Context(), override); err != nil {
        return system.NewHTTPError500(fmt.Sprintf("failed to save settings: %v", err))
    }

    return nil
}
```

### 6.3 Database Schema

```sql
CREATE TABLE zed_settings_overrides (
    session_id VARCHAR PRIMARY KEY,
    user_id VARCHAR NOT NULL,
    overrides JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_zed_settings_user ON zed_settings_overrides(user_id);
```

### 6.4 Container Integration

**Dockerfile changes** (`Dockerfile.sway-helix`):

```dockerfile
# Build settings-sync-daemon
FROM golang:1.24 AS build
WORKDIR /app
COPY api ./api
RUN CGO_ENABLED=0 go build -o /settings-sync-daemon ./cmd/settings-sync-daemon

# Runtime
FROM ghcr.io/games-on-whales/base-app:edge
COPY --from=build /settings-sync-daemon /usr/local/bin/settings-sync-daemon
```

**Startup script** (`wolf/sway-config/startup-app.sh`):

```bash
# Start settings sync daemon (after Sway is ready)
echo "exec HELIX_API_URL=$HELIX_API_URL HELIX_API_TOKEN=$HELIX_API_TOKEN HELIX_SESSION_ID=$HELIX_SESSION_ID /usr/local/bin/settings-sync-daemon > /tmp/settings-sync.log 2>&1" >> $HOME/.config/sway/config
```

**Wolf executor** (`wolf_executor.go`):

```go
// In createSwayWolfApp()
env = append(env,
    fmt.Sprintf("HELIX_SESSION_ID=%s", config.SessionID),
    "SETTINGS_SYNC_PORT=9877",
)
```

---

## 7. Migration Path

### Phase 1: Daemon Foundation (Week 1)
- [ ] Build settings-sync-daemon binary
- [ ] Basic merge logic (Helix + user)
- [ ] HTTP endpoints (health, get, reload)
- [ ] Container integration

### Phase 2: File Watching (Week 2)
- [ ] inotify integration
- [ ] Debouncing logic
- [ ] User override extraction
- [ ] Sync to Helix API

### Phase 3: Polling & Updates (Week 3)
- [ ] Poll Helix for app config changes
- [ ] Update detection
- [ ] Atomic file writes
- [ ] Testing with real Zed

### Phase 4: Production Hardening (Week 4)
- [ ] Error handling & retries
- [ ] Graceful degradation
- [ ] Metrics & logging
- [ ] K8s deployment validation

---

## 8. Testing Strategy

### Unit Tests
```go
func TestMergeSettings(t *testing.T) {
    helix := map[string]interface{}{
        "context_servers": map[string]interface{}{
            "helix-rag": map[string]interface{}{"enabled": true},
        },
    }

    user := map[string]interface{}{
        "context_servers": map[string]interface{}{
            "helix-rag": map[string]interface{}{"enabled": false}, // Override
            "my-tool": map[string]interface{}{"url": "..."},       // Addition
        },
        "theme": "dark", // User preference
    }

    merged := mergeSettings(helix, user)

    // User override wins for helix-rag
    assert.False(t, merged["context_servers"].(map[string]interface{})["helix-rag"].(map[string]interface{})["enabled"].(bool))

    // User addition preserved
    assert.Contains(t, merged["context_servers"], "my-tool")

    // User preferences preserved
    assert.Equal(t, "dark", merged["theme"])
}
```

### Integration Tests
```go
func TestDaemonSync(t *testing.T) {
    // 1. Start mock Helix API
    api := startMockAPI(t)
    defer api.Stop()

    // 2. Start daemon
    daemon := startDaemon(t, api.URL())
    defer daemon.Stop()

    // 3. Simulate Zed UI change
    writeSettings(t, SettingsPath, map[string]interface{}{
        "theme": "dark",
    })

    // 4. Wait for sync
    time.Sleep(1 * time.Second)

    // 5. Verify API received update
    userSettings := api.GetUserSettings(t, sessionID)
    assert.Equal(t, "dark", userSettings["theme"])
}
```

---

## 9. Conclusion

**Recommendation: Implement Option A (Settings Sync Daemon)** with the screenshot server pattern as foundation.

**Key Benefits**:
- ✅ **K8s Ready**: No filesystem coupling, pure HTTP/network
- ✅ **Proven Architecture**: Screenshot server shows it works
- ✅ **Bidirectional Sync**: Handles both directions elegantly
- ✅ **No Zed Changes**: Works with Zed as-is
- ✅ **Clear Ownership**: Helix manages base, user manages overrides

**Next Steps**:
1. Review this design doc
2. Approve architecture approach
3. Begin Phase 1 implementation
4. Iterate based on testing feedback
