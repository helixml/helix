# Moonlight Web Configuration

This directory contains configuration for the moonlight-web streaming service.

## Files

- **config.json** - moonlight-web server settings (WebRTC, credentials)
- **data.json** - Hosts and pairing data (Wolf pre-configured)

## Production Setup

### 1. Set Secure Pairing PIN

The internal pairing PIN is used to automatically pair moonlight-web with Wolf on startup.

**Generate a secure random PIN**:
```bash
# Linux/Mac
echo $((RANDOM % 10000)) | awk '{printf "%04d\n", $1}'

# Or use openssl
openssl rand -hex 2 | awk '{print substr($0,1,4)}'
```

**Add to .env file**:
```bash
MOONLIGHT_INTERNAL_PAIRING_PIN=7382  # Your generated PIN
```

**Why this is secure**:
- PIN is only used for internal docker network communication
- moonlight-web and Wolf are both trusted services
- PIN is not exposed externally
- After pairing, certificates provide strong encryption
- Even if PIN is compromised, attacker needs docker network access

### 2. Change moonlight-web Credentials

**Edit config.json**:
```json
{
  "credentials": "YOUR_SECURE_PASSWORD_HERE"
}
```

This password protects the moonlight-web web interface.

**Important**: In production, use a strong password and configure HTTPS:

```json
{
  "credentials": "your-strong-password",
  "certificate": {
    "private_key_pem": "./key.pem",
    "certificate_pem": "./cert.pem"
  }
}
```

Generate certificates:
```bash
cd moonlight-web-config
python ~/pm/moonlight-web-stream/moonlight-web/web-server/generate_certificate.py
```

### 3. Configure TURN Server (Optional)

For NAT traversal when users are behind restrictive firewalls:

**Edit config.json**:
```json
{
  "webrtc_ice_servers": [
    {
      "urls": ["stun:stun.l.google.com:19302"]
    },
    {
      "urls": [
        "turn:your-turn-server.com:3478?transport=udp",
        "turn:your-turn-server.com:3478?transport=tcp"
      ],
      "username": "turn-user",
      "credential": "turn-password"
    }
  ]
}
```

**Deploy coturn TURN server**:
```yaml
# docker-compose.prod.yaml
coturn:
  image: coturn/coturn:latest
  ports:
    - "3478:3478/udp"
    - "3478:3478/tcp"
  environment:
    - REALM=your-domain.com
    - TURNSERVER_ENABLED=1
```

## Development Setup

For development:

### Initial Pairing (One-Time)

Pairing must be completed once manually:

1. **Start the stack**: `docker compose -f docker-compose.dev.yaml up -d`
2. **Get the PIN from Wolf logs**:
   ```bash
   docker compose -f docker-compose.dev.yaml logs moonlight-web | grep Pin
   # Look for: {"Pin":"1234"}
   ```
3. **Open moonlight-web UI**: http://localhost:8081
4. **Login**: Username and password both `helix` (from config.json)
5. **Click on Wolf host** → Enter the PIN when prompted
6. **Pairing complete!** Certificates saved to `data.json`

### Auto-Pairing (Future Restarts)

After initial manual pairing:
- Certificates persist in `data.json`
- moonlight-web automatically reconnects using saved certificates
- No PIN needed for subsequent connections
- Certificates remain valid unless Wolf is reset

## How Pairing Works

```
┌─────────────────────────────────────────────────────────┐
│ 1. Helix API Starts                                     │
│    ├─ Waits for moonlight-web ready                     │
│    └─ Checks if Wolf is already paired                  │
└─────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│ 2. If Not Paired: Trigger Pairing                       │
│    ├─ moonlight-web calls Wolf /pair endpoint           │
│    ├─ Wolf generates pending pair request               │
│    └─ Returns pair_secret to moonlight-web              │
└─────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│ 3. Helix Completes Pairing                              │
│    ├─ Gets pending request from Wolf API                │
│    ├─ Extracts pair_secret                              │
│    ├─ Calls Wolf /api/v1/pair/client with:              │
│    │   • pair_secret                                     │
│    │   • PIN (from env or generated)                     │
│    └─ Wolf validates and generates certificates         │
└─────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│ 4. Certificates Saved                                   │
│    ├─ Wolf sends certificates to moonlight-web          │
│    ├─ moonlight-web saves to data.json                  │
│    └─ Future connections use certificates (no re-pair)  │
└─────────────────────────────────────────────────────────┘
```

## Troubleshooting

### Pairing Fails

**Check logs**:
```bash
docker compose -f docker-compose.dev.yaml logs api | grep moonlight
docker compose -f docker-compose.dev.yaml logs moonlight-web | grep pair
docker compose -f docker-compose.dev.yaml logs wolf | grep pair
```

**Manual pairing** (if auto-pairing fails):
1. Open http://localhost:8080/moonlight/
2. Click on Wolf host
3. Enter PIN when prompted (check logs for generated PIN)
4. Pairing saved to data.json automatically

### Reset Pairing

**To re-pair** (if certificates become invalid):
```bash
# Stop services
docker compose -f docker-compose.dev.yaml down moonlight-web

# Clear pairing data
cat > moonlight-web-config/data.json << 'EOF'
{
  "hosts": [{
    "address": "wolf",
    "http_port": 47989,
    "client_private_key": null,
    "client_certificate": null,
    "server_certificate": null
  }]
}
EOF

# Restart and auto-pairing will run again
docker compose -f docker-compose.dev.yaml up -d moonlight-web
```

## Security Notes

### Why Env Var for PIN is Safe

1. **Localhost-Only Communication**:
   - moonlight-web and Wolf communicate over docker internal network
   - Not exposed to internet
   - Even if PIN is leaked, attacker needs docker network access

2. **Post-Pairing Security**:
   - PIN only used during initial pairing
   - After pairing, mutual TLS with certificates provides strong auth
   - Certificates are unique per deployment

3. **Production Best Practices**:
   - Use secrets management (Kubernetes Secrets, AWS Secrets Manager)
   - Rotate PIN periodically (requires re-pairing)
   - Monitor pairing attempts in audit logs

### Production Checklist

- [ ] Generate secure random PIN (not "0000")
- [ ] Store PIN in secrets manager (not plain .env file)
- [ ] Change moonlight-web credentials from "helix"
- [ ] Enable HTTPS for moonlight-web
- [ ] Configure TURN server for NAT traversal
- [ ] Set up certificate rotation policy
- [ ] Enable audit logging
- [ ] Monitor pairing attempts

---

**Last Updated**: 2025-10-08
**Author**: Claude Code
