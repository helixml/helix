#!/bin/bash
# Generate Moonlight pairing certificates for internal Wolf <> moonlight-web communication
# These are stored in git and used for development/testing only
# Production deployments should use proper certificate management

set -e

CERT_DIR="$(dirname "$0")/../moonlight-web-config/certs"
mkdir -p "$CERT_DIR"

echo "🔐 Generating Moonlight pairing certificates..."

# Generate client private key and certificate (moonlight-web client)
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$CERT_DIR/client-key.pem" \
  -out "$CERT_DIR/client-cert.pem" \
  -days 3650 \
  -subj "/CN=helix-moonlight-client/O=Helix/C=US"

# Generate server certificate (Wolf server)
# Note: In real Moonlight pairing, the server cert is fetched from Wolf
# For our internal setup, we generate a matching pair
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$CERT_DIR/server-key.pem" \
  -out "$CERT_DIR/server-cert.pem" \
  -days 3650 \
  -subj "/CN=wolf-moonlight-server/O=Helix/C=US"

echo "✅ Certificates generated in $CERT_DIR"
echo ""
echo "These certificates bypass Moonlight pairing for internal Wolf communication."
echo "DO NOT use these certificates in production - they are for development only."
echo ""
echo "Next steps:"
echo "1. Update moonlight-web-config/data.json with certificate paths"
echo "2. Configure Wolf to accept these certificates (if needed)"
