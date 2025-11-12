# Wolf & Moonlight-Web Pairing and Configuration

## Overview

Wolf (game streaming server) and moonlight-web (WebRTC bridge) use Moonlight protocol pairing with certificate-based authentication. When Wolf's configuration changes, pairing must be reset.

## Quick Reference

**Update Wolf encoder settings (e.g., gop-size, bitrate):**
```bash
# Stop Wolf, delete runtime config, restart both
docker compose -f docker-compose.dev.yaml down wolf
rm wolf/config.toml
docker compose -f docker-compose.dev.yaml up -d wolf
sleep 8
rm moonlight-web-config/data.json
docker compose -f docker-compose.dev.yaml restart moonlight-web
sleep 12

# Verify pairing succeeded
docker compose -f docker-compose.dev.yaml logs moonlight-web --tail 15 | grep "Auto-pairing"
```

**Expected output:**
```
✅ Auto-pairing with Wolf completed successfully
```

## Understanding the Pairing System

### Two-Level Pairing

1. **Server-level pairing** (moonlight-web ↔ Wolf):
   - Happens once at moonlight-web startup
   - Uses `MOONLIGHT_INTERNAL_PAIRING_PIN` environment variable (default: 1234)
   - Result stored in `moonlight-web-config/data.json`
   - Wolf auto-accepts pairing when PIN matches

2. **Per-session pairing** (browser client ↔ Wolf):
   - Happens for each browser WebRTC connection
   - Moonlight-web generates unique certificate per `client_unique_id`
   - Auto-paired using server-level credentials
   - Cached in moonlight-web's certificate store

### How Auto-Pairing Works

**moonlight-web side:**
```rust
// In /api/pair endpoint handler
if let Ok(env_pin) = std::env::var("MOONLIGHT_INTERNAL_PAIRING_PIN") {
    // Use PIN from environment variable
    let pin = parse_4_digit_pin(env_pin);
    host.pair(&client_auth, device_name, pin).await
}
```

**Wolf side:**
```cpp
// In PairSignal event handler
auto auto_pin_env = utils::get_env("MOONLIGHT_INTERNAL_PAIRING_PIN", "");
if (!auto_pin.empty() && is_local_client) {
    // Auto-fulfill PIN promise
    pair_sig->user_pin->set_value(auto_pin);
}
```

**Both read from the same environment variable** - no manual PIN entry needed!

## When to Re-Pair

### Always Re-Pair When:
- Wolf's `config.toml` is deleted/regenerated
- Wolf's SSL certificates change (`wolf/cert.pem`, `wolf/key.pem`)
- moonlight-web reports "Certificate verification failed"

### Re-Pairing Procedure

1. **Delete Wolf runtime config** (preserves encoder template):
   ```bash
   docker compose -f docker-compose.dev.yaml down wolf
   rm wolf/config.toml
   ```

2. **Start Wolf** (regenerates config from template):
   ```bash
   docker compose -f docker-compose.dev.yaml up -d wolf
   sleep 8  # Wait for Wolf to initialize
   ```

3. **Delete moonlight-web pairing data**:
   ```bash
   rm moonlight-web-config/data.json
   ```

4. **Restart moonlight-web** (triggers auto-pairing):
   ```bash
   docker compose -f docker-compose.dev.yaml restart moonlight-web
   sleep 12
   ```

5. **Verify pairing succeeded**:
   ```bash
   docker compose -f docker-compose.dev.yaml logs moonlight-web --tail 20 | grep -E "Auto-pairing|failed to pair"
   ```

### Success Indicators

**moonlight-web logs:**
```
[INFO] [Api]: Using internal pairing PIN from MOONLIGHT_INTERNAL_PAIRING_PIN
✅ Auto-pairing with Wolf completed successfully
```

**Wolf logs:**
```
INFO  | Auto-pairing client 172.19.0.14 with PIN from MOONLIGHT_INTERNAL_PAIRING_PIN
```

**No random PIN generation** - if you see this, pairing failed:
```
INFO  | Insert pin at http://172.19.0.50:47989/pin/#abc123
```

### Failure Indicators

**Certificate verification failed:**
```
[ERROR] [Stream]: failed to get app list from host: Api(InvalidXmlStatusCode { message: Some("The client is not authorized. Certificate verification failed.") })
```
→ Delete `data.json` and restart moonlight-web

**Incorrect PIN:**
```
[INFO] [Api]: failed to pair host wolf: Pair(IncorrectPin)
```
→ Check `MOONLIGHT_INTERNAL_PAIRING_PIN` is consistent in both containers:
```bash
docker compose -f docker-compose.dev.yaml exec wolf env | grep MOONLIGHT_INTERNAL_PAIRING_PIN
docker compose -f docker-compose.dev.yaml exec moonlight-web env | grep MOONLIGHT_INTERNAL_PAIRING_PIN
```

**Out of order pair request:**
```
[INFO] [Api]: failed to pair host wolf: Pair(Api(InvalidXmlStatusCode { message: Some("Out of order pair request (phase 1)") }))
```
→ Wolf has stale pairing state. Delete `wolf/config.toml` and restart both.

## Updating Wolf Encoder Settings

### Template vs Runtime Config

**Wolf uses two config files:**

1. **Template** (baked into image at build time):
   - Source: `wolf/config.toml.template`
   - Location in container: `/etc/wolf/cfg/config.toml.template`
   - Used to initialize runtime config if missing

2. **Runtime config** (persisted in volume):
   - Location: `wolf/config.toml` (mounted from host)
   - Container path: `/etc/wolf/cfg/config.toml`
   - Wolf only reads template if this file doesn't exist or is empty

**Init script logic:**
```bash
if [ ! -f "$CONFIG_FILE" ] || [ ! -s "$CONFIG_FILE" ]; then
    cp "$TEMPLATE_FILE" "$CONFIG_FILE"
    # ... (UUID generation, PIN injection)
fi
```

### Updating Encoder Settings

**Option 1: Update template and regenerate** (recommended for major changes):

1. Edit `wolf/config.toml.template` with new settings
2. Rebuild Wolf image: `./stack rebuild-wolf`
3. Delete runtime config: `rm wolf/config.toml`
4. Restart and re-pair (see procedure above)

**Option 2: Directly edit runtime config** (quick testing):

1. Stop Wolf: `docker compose -f docker-compose.dev.yaml down wolf`
2. Edit `wolf/config.toml` directly (e.g., `sed -i 's/gop-size=0/gop-size=15/g' wolf/config.toml`)
3. Start Wolf: `docker compose -f docker-compose.dev.yaml up -d wolf`
4. **No pairing needed** - certificates unchanged

## Common Encoder Settings

### GOP Size (Group of Pictures)

Controls I-frame frequency:

```toml
# All I-frames (original - causes quality issues)
gop-size=0

# I-frame every 15 frames (recommended - FEC compatible)
gop-size=15

# I-frame every 30 frames (too large for FEC @ 40 Mbps)
gop-size=30

# Encoder chooses automatically
gop-size=-1
```

**FEC Packet Limit:** 255 packets max per frame
- 255 packets × 1400 bytes/packet ≈ **350 KB frame size limit**
- Exceeding this disables FEC → juddering on packet loss

**Recommended for 4K60 @ 40 Mbps:** `gop-size=15`
- I-frame size: ~1.1 MB = 80 packets ✅
- P-frames: 5-20 KB each
- Balanced quality and FEC protection

### Bitrate

Browser-side setting in `frontend/src/components/external-agent/MoonlightStreamViewer.tsx`:

```typescript
settings.bitrate = 40000;  // 40 Mbps

// Historical values:
// - 80000: Original (excessive with P-frames, caused FEC warnings)
// - 40000: Optimal for gop-size=15 (current)
```

**Frontend auto-reloads** - no container restart needed!

## Troubleshooting

### Auto-Pairing Broken After Upstream Merge

**Symptom:** Pairing always fails with `IncorrectPin` after merging moonlight-web-stream upstream

**Root cause:** Upstream code generates random PINs, overwriting our `MOONLIGHT_INTERNAL_PAIRING_PIN` reader

**Fix:** Restore PIN-reading code in `moonlight-web/web-server/src/api/mod.rs`:

```rust
// BEFORE (upstream - broken):
let Ok(pin) = PairPin::generate() else { ... };

// AFTER (our fork - working):
let pin = if let Ok(env_pin) = std::env::var("MOONLIGHT_INTERNAL_PAIRING_PIN") {
    // Parse 4-digit PIN from env var
    if env_pin.len() == 4 && env_pin.chars().all(|c| c.is_ascii_digit()) {
        let digits: Vec<u8> = env_pin.chars()
            .map(|c| c.to_digit(10).unwrap() as u8)
            .collect();
        PairPin::from_array([digits[0], digits[1], digits[2], digits[3]]).unwrap()
    } else {
        PairPin::generate().unwrap()
    }
} else {
    PairPin::generate().unwrap()
};
```

Then rebuild: `./stack build-moonlight-web`

### NAL Stats Not Appearing in Logs

**Symptom:** Added `info!()` logging but nothing appears in `docker compose logs moonlight-web`

**Root causes:**

1. **RUST_LOG filter missing streamer module:**
   ```yaml
   # BEFORE (broken):
   RUST_LOG=moonlight_common=trace,moonlight_web=trace

   # AFTER (working):
   RUST_LOG=moonlight_common=trace,moonlight_web=trace,streamer::video=info
   ```

2. **IPC uses stdout, logs must use stderr:**
   ```rust
   // Use eprintln!() instead of info!() to avoid IPC conflicts
   eprintln!("[H264 Stats] I-frames: {}, P-frames: {}", i_count, p_count);
   ```

### FEC Warnings

**Symptom:**
```
WARN | [GSTREAMER] Size of frame too large, 285 packets is bigger than the max (255); skipping FEC
```

**Diagnosis:** I-frames exceed 255 packet limit (~350 KB max)

**Solutions:**
- Reduce GOP size (gop-size=30 → gop-size=15)
- Reduce bitrate (80 Mbps → 40 Mbps)
- Both together for best results

**Calculation:**
```
Max I-frame size with FEC: 255 packets × 1400 bytes ≈ 350 KB
Actual I-frame size = (Bitrate / FPS) × (GOP size) / GOP size
                    = (40 Mbps / 60 FPS) × 15 frames ÷ 15
                    ≈ 1.1 MB total budget ÷ 15 frames
                    ≈ 73 KB per frame average
But I-frames get most of the budget: ~1.1 MB - (14 × 10 KB) ≈ 1 MB
1 MB ÷ 1400 bytes ≈ 80 packets ✅
```

## Environment Variables

### Required for Auto-Pairing

Both Wolf and moonlight-web need:

```bash
MOONLIGHT_INTERNAL_PAIRING_PIN=1234  # Must match on both sides
```

Set in `.env` file (loaded by docker-compose).

**Important:** Container recreation needed for env var changes:
```bash
# restart alone doesn't reload env vars:
docker compose -f docker-compose.dev.yaml restart wolf  # ❌ Old env

# down + up forces recreation:
docker compose -f docker-compose.dev.yaml down wolf
docker compose -f docker-compose.dev.yaml up -d wolf   # ✅ New env
```

### Optional

```bash
MOONLIGHT_CREDENTIALS=helix          # Auth credentials for moonlight-web API
TURN_PASSWORD=helix-turn-secret      # TURN server for WebRTC NAT traversal
```

## Files Reference

### Wolf Configuration
- **Template:** `wolf/config.toml.template` (source of truth, edit this)
- **Runtime:** `wolf/config.toml` (auto-generated, gitignored)
- **Init script:** `wolf/init-wolf-config.sh` (copies template, sets UUID/PIN)
- **Certificates:** `wolf/cert.pem`, `wolf/key.pem` (auto-generated if missing)

### Moonlight-Web Configuration
- **Template:** `moonlight-web-config/config.json.template`
- **Runtime:** `moonlight-web-config/config.json` (TURN server, ICE servers)
- **Pairing data:** `moonlight-web-config/data.json` (Wolf host info, pairing status)
- **Init script:** `moonlight-web-config/init-moonlight-config.sh` (handles pairing)

### Frontend Settings
- **Stream config:** `frontend/src/components/external-agent/MoonlightStreamViewer.tsx`
  - Bitrate, resolution, FPS, queue sizes
  - **Hot-reloads** via Vite - no restart needed

## Common Workflows

### Change GOP Size

**Development (quick test):**
```bash
docker compose -f docker-compose.dev.yaml down wolf
sed -i 's/gop-size=0/gop-size=15/g' wolf/config.toml
docker compose -f docker-compose.dev.yaml up -d wolf
```

**Production (persistent):**
1. Edit `wolf/config.toml.template`
2. Update Wolf repo: `cd /home/luke/pm/wolf && sed -i 's/gop-size=0/gop-size=15/g' docker/config.toml.template src/moonlight-server/state/default/*.toml`
3. Rebuild Wolf: `./stack rebuild-wolf`
4. Delete runtime config and re-pair (see Quick Reference)

### Change Bitrate

**Frontend only** (hot-reloads):
```bash
# Edit frontend/src/components/external-agent/MoonlightStreamViewer.tsx
settings.bitrate = 40000;  // Change this value

# Vite auto-reloads, refresh browser tab
```

No container restart needed!

### Verify Configuration

**Check Wolf encoder settings:**
```bash
docker compose -f docker-compose.dev.yaml exec wolf grep "gop-size\|bitrate" /etc/wolf/cfg/config.toml | grep nvh264enc
```

**Check moonlight-web pairing status:**
```bash
cat moonlight-web-config/data.json | jq '.hosts[0].paired // "not_paired"'
```

**Check if NAL stats show correct frame distribution:**
```bash
docker compose -f docker-compose.dev.yaml logs moonlight-web --tail 100 | grep "H264 Stats"
```

Expected with gop-size=15:
```
[H264 Stats] I-frames: 60, P-frames: 840, ...  (ratio ~14:1)
```

## Debugging Pairing Failures

### Enable Detailed Logging

**Wolf:**
```bash
# Wolf logs to stdout by default
docker compose -f docker-compose.dev.yaml logs wolf --tail 50
```

**Moonlight-web:**
```bash
# Already configured in docker-compose.dev.yaml:
RUST_LOG=moonlight_common=trace,moonlight_web=trace,streamer::video=info,webrtc_sctp=warn
```

### Common Issues

**Issue: "IncorrectPin" even with matching env vars**

Check if upstream merge overwrote PIN-reading code:
```bash
cd /home/luke/pm/moonlight-web-stream
grep -A5 "PairPin::generate()" moonlight-web/web-server/src/api/mod.rs
```

Should see env var reading logic, not direct `generate()` call.

**Issue: "Out of order pair request"**

Wolf has stale pairing state from failed attempts. Full reset:
```bash
docker compose -f docker-compose.dev.yaml down wolf moonlight-web
rm wolf/config.toml moonlight-web-config/data.json
docker compose -f docker-compose.dev.yaml up -d wolf
sleep 8
docker compose -f docker-compose.dev.yaml up -d moonlight-web
sleep 12
```

**Issue: Wolf generates random PIN instead of using env var**

Verify Wolf has the auto-pairing code:
```bash
cd /home/luke/pm/wolf
grep -n "MOONLIGHT_INTERNAL_PAIRING_PIN" src/moonlight-server/rest/servers.cpp
```

Should find auto-fulfilling logic around line 76-85.

## Advanced: Manual Pairing (Fallback)

If auto-pairing fails completely, use manual pairing:

1. Start Wolf without `MOONLIGHT_INTERNAL_PAIRING_PIN` (generates random PIN)
2. Check Wolf logs for: `Insert pin at http://172.19.0.50:47989/pin/#SECRET`
3. Visit that URL in browser and enter the 4-digit PIN shown on screen
4. Trigger pairing from moonlight-web:
   ```bash
   docker compose -f docker-compose.dev.yaml exec moonlight-web bash -c \
     'exec 3<>/dev/tcp/localhost/8080 && \
      echo -ne "POST /api/pair HTTP/1.1\r\nHost: localhost:8080\r\nContent-Length: 13\r\n\r\n{\"host_id\":0}" >&3 && \
      cat <&3'
   ```

This is error-prone and slow. Fix auto-pairing instead!

## References

- **Wolf config docs:** `wolf/docker/config.toml.template` (has inline comments)
- **Pairing protocol:** Moonlight protocol Phase 1-5 (HTTP + HTTPS)
- **Install script:** `install.sh` lines 1744-1792 (auto-pairing reference implementation)
- **Design doc:** `design/2025-11-06-webrtc-video-quality-investigation.md` (GOP size tuning)
