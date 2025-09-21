# 🚀 Zed-Helix WebSocket Sync

**Real-time bidirectional sync between Zed editor and Helix AI sessions**

## 🎯 How it works

- **Zed** maintains chat threads (source of truth for conversation content)
- **Helix** can inject new messages into Zed threads via WebSocket commands  
- **Environment variables** configure the connection (perfect for containers!)

```
Helix Session "hi" → WebSocket → Zed Chat Thread
```

## 🏃‍♂️ Quick Start

```bash
# 1. Start Helix dev environment
./stack start

# 2. Build Zed with WebSocket sync
./stack build-zed

# 3. Test the integration
./test-zed-helix-integration.sh
```

## 🔧 Configuration

Zed reads these environment variables:

- `ZED_EXTERNAL_SYNC_ENABLED=true` - Enable WebSocket sync
- `ZED_HELIX_URL=localhost:8080` - Helix server URL  
- `ZED_HELIX_TOKEN=your-token` - Authentication token
- `ZED_HELIX_TLS=false` - Use TLS (true/false)

## 🧪 What the test does

1. **Loads runner token** from `.env` file
2. **Starts Zed** with WebSocket sync enabled
3. **Creates Helix session** and sends "hi" message
4. **Monitors WebSocket** for message injection commands
5. **Verifies** bidirectional sync is working

## 🐳 Container Ready

Environment variable configuration makes this perfect for Docker deployments - just set the env vars in your container!

---
*Built for seamless AI-assisted coding workflows* ✨


