# HostNotPaired Debugging Guide for code.helix.ml

## Commands to Run on Production

### 1. Check if moonlight-web successfully paired

```bash
# Check init script logs (most recent pairing attempt)
docker compose logs moonlight-web --tail 100 | grep -E "Auto-pairing|Wolf is ready|paired"

# Check pairing response file (if exists)
docker compose exec moonlight-web cat /tmp/pair-response.log 2>/dev/null

# Check data.json for "paired" section
docker compose exec moonlight-web cat /app/server/data.json | jq '.hosts[0].paired'
# Should show pairing cert, not null
```

### 2. Check Wolf's view of pairings

```bash
# Check if Wolf has any paired clients
docker compose exec api curl -s --unix-socket /var/run/wolf/wolf.sock \
  'http://localhost/api/v1/pairings' 2>/dev/null | jq '.'

# Check Wolf config/state directory
docker compose exec wolf ls -la /etc/wolf/
docker compose exec wolf cat /etc/wolf/config.toml 2>/dev/null
```

### 3. Check moonlight-web environment

```bash
# Verify MOONLIGHT_INTERNAL_PAIRING_PIN is set
docker compose exec moonlight-web env | grep MOONLIGHT_INTERNAL_PAIRING_PIN

# Check if Wolf environment has matching PIN
docker compose exec wolf env | grep MOONLIGHT_INTERNAL_PAIRING_PIN
```

### 4. Check actual streaming attempt logs

```bash
# When user gets HostNotPaired, capture these logs:
docker compose logs moonlight-web --tail 50 --follow

# In another terminal, try to stream and watch logs
# Look for: "tried to connect to a not paired host"
```

### 5. Check if there are multiple Wolf hosts in data.json

```bash
# moonlight-web might be trying to connect to wrong host
docker compose exec moonlight-web cat /app/server/data.json | jq '.hosts[] | {address, unique_id, paired: (.paired != null)}'
```

## Common Causes & Fixes

### Cause 1: Auto-pairing never happened

**Symptoms:**
- Init script shows "⚠️  Auto-pairing may have failed"
- data.json has no "paired" section
- /tmp/pair-response.log doesn't contain "Paired"

**Fix:**
```bash
# Restart moonlight-web to retry auto-pairing
docker compose restart moonlight-web

# Or manually trigger pairing via API
docker compose exec moonlight-web curl -X POST http://localhost:8080/api/pair \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer helix" \
  -d '{"host_id":0}'
```

### Cause 2: Wolf restarted and lost pairing

**Symptoms:**
- data.json HAS "paired" section
- Wolf doesn't recognize the pairing
- Wolf logs show pairing requests from unknown client

**Fix:**
```bash
# Check if Wolf persists state
ls -la ./wolf/

# If no state files, Wolf isn't persisting pairings!
# Need to ensure HOST_APPS_STATE_FOLDER is writable
```

### Cause 3: Wrong host ID

**Symptoms:**
- Frontend is connecting to wrong host ID
- moonlight-web has multiple hosts configured

**Check:**
```bash
# What host ID is frontend using?
# Check browser console for stream.html?hostId=X&appId=Y

# What hosts does moonlight-web know about?
docker compose exec moonlight-web cat /app/server/data.json | jq '.hosts'
```

### Cause 4: Environment variable mismatch

**Symptoms:**
- MOONLIGHT_INTERNAL_PAIRING_PIN not set or different between containers

**Fix:**
```bash
# Check .env file or docker-compose.yaml
grep MOONLIGHT_INTERNAL_PAIRING_PIN .env docker-compose.yaml

# Should be same value for both wolf and moonlight-web services
```

## Collecting Debug Info

Run this complete diagnostic:

```bash
echo "=== Moon light-web pairing status ==="
docker compose exec moonlight-web cat /app/server/data.json | jq '.'

echo -e "\n=== Moonlight-web init logs ==="
docker compose logs moonlight-web --tail 200 | grep -E "Auto-pairing|Wolf|Paired|ready"

echo -e "\n=== Wolf pairing PIN ==="
docker compose exec wolf env | grep MOONLIGHT_INTERNAL_PAIRING_PIN

echo -e "\n=== moonlight-web pairing PIN ==="
docker compose exec moonlight-web env | grep MOONLIGHT_INTERNAL_PAIRING_PIN

echo -e "\n=== Actual streaming error ==="
# User needs to trigger streaming and capture this
docker compose logs moonlight-web --tail 20 --follow
```
