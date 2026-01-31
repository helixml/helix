# Security Dashboard Integration Guide

## Quick Integration Steps

### 1. Register API Routes

**File**: `api/pkg/server/server.go`

Find the `registerRoutes()` function (around line 440) and add after other route registrations:

```go
// Around line 850, after other subRouter registrations, add:

// Security telemetry monitoring routes
s.registerSecurityRoutes(subRouter)
```

### 2. Add Frontend Route

**File**: `frontend/src/App.tsx` (or wherever routes are defined)

```tsx
import TelemetrySecurityDashboard from './components/security/TelemetrySecurityDashboard'

// Add to your routes:
<Route path="/security/telemetry" element={<TelemetrySecurityDashboard />} />
```

### 3. Add Navigation Link

Find your navigation sidebar component and add:

```tsx
<ListItemButton onClick={() => navigate('/security/telemetry')}>
  <ListItemIcon>
    <SecurityIcon />
  </ListItemIcon>
  <ListItemText primary="AI Agent Security" />
</ListItemButton>
```

### 4. Rebuild and Deploy

```bash
cd ~/pm/helix
./stack build-sandbox
docker compose up -d sandbox
```

---

## Testing the Implementation

### 1. Verify Firewall Rules
```bash
docker exec -it <sandbox-container> bash
sudo iptables -L TELEMETRY_BLOCK -n -v

# Should show rules like:
# pkts bytes target     prot opt in     out     source   destination
#    0     0 LOG        all  --  *      *       0.0.0.0/0  gb4w8c3ygj-default-sea.rum.aliyuncs.com  /* QWEN_CODE:ALIBABA_RUM */
#    0     0 REJECT     all  --  *      *       0.0.0.0/0  gb4w8c3ygj-default-sea.rum.aliyuncs.com  /* QWEN_CODE:ALIBABA_RUM */
```

### 2. Test API Endpoints
```bash
# Get auth token first
TOKEN="your-helix-api-token"

# Test telemetry status endpoint
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/security/telemetry-status | jq .

# Test logs endpoint
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/security/telemetry-logs?limit=20" | jq .
```

### 3. Access Dashboard
1. Open Helix UI in browser
2. Navigate to `/security/telemetry`
3. Verify dashboard loads
4. Check all 4 agents show "SECURED" status
5. Verify firewall status shows "ACTIVE"
6. Confirm total blocked packets = 0 (no attempts)

---

## Expected Results

### Healthy System
```
Firewall Status: ✅ ACTIVE
Total Blocked Packets: 0
Agent Configurations: 4/4 SECURED

Security Recommendations:
✅ All AI agents properly configured with telemetry disabled
✅ Firewall active and blocking phone-home attempts
```

### System with Phone-Home Attempts
```
Firewall Status: ✅ ACTIVE
Total Blocked Packets: 127
Agent Configurations: 4/4 SECURED

Security Recommendations:
⚠️ Detected 127 telemetry attempts blocked by firewall - investigate agent configuration

Phone-Home Attempts:
- QWEN_CODE → ALIBABA_RUM: 127 packets (CRITICAL)
```

**Action Required**: Investigate why agent is attempting to phone home despite config.

---

## Rollback Plan

If integration causes issues:

### 1. Disable API Routes
Comment out the `s.registerSecurityRoutes(subRouter)` call

### 2. Remove Frontend Route
Comment out the security dashboard route

### 3. Firewall Remains Active
The telemetry firewall will continue blocking even if dashboard is disabled - this is by design for safety.

### 4. Manual Firewall Disable (if needed)
```bash
# Inside sandbox container
sudo iptables -F TELEMETRY_BLOCK
sudo iptables -X TELEMETRY_BLOCK
```

---

## Notes

- **Performance Impact**: Minimal - iptables pattern matching is very efficient
- **Storage Impact**: ~10KB for logs and counters
- **API Impact**: New endpoints don't affect existing routes
- **Frontend Impact**: New component is isolated, doesn't modify existing pages

---

**Document Version**: 1.0
**Integration Time**: ~15 minutes
**Testing Time**: ~10 minutes
