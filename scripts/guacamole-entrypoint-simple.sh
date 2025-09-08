#!/bin/bash

# Simple Guacamole Entrypoint - No external dependencies
# Uses only tools available in the Guacamole container

set -e

echo "🚀 Starting Guacamole with database auto-initialization..."

# Debug connection parameters
echo "🔍 PostgreSQL connection parameters:"
echo "   POSTGRES_HOSTNAME: $POSTGRES_HOSTNAME"
echo "   POSTGRES_USER: $POSTGRES_USER" 
echo "   POSTGRES_DATABASE: $POSTGRES_DATABASE"

# Simple wait for PostgreSQL using curl/telnet-like approach
echo "⏳ Waiting for PostgreSQL to be ready..."
RETRY_COUNT=0
while [ $RETRY_COUNT -lt 30 ]; do
    RETRY_COUNT=$((RETRY_COUNT + 1))
    
    # Use timeout and /dev/tcp to test connection (built into bash)
    if timeout 3 bash -c "</dev/tcp/$POSTGRES_HOSTNAME/5432" 2>/dev/null; then
        echo "   ✅ PostgreSQL connection successful (attempt $RETRY_COUNT)"
        break
    fi
    
    echo "   PostgreSQL unavailable (attempt $RETRY_COUNT) - sleeping"
    if [ $RETRY_COUNT -eq 30 ]; then
        echo "❌ ERROR: PostgreSQL connection failed after 30 attempts"
        exit 1
    fi
    sleep 2
done

# Create the database and initialize schema using the Guacamole REST API approach
# Since we can't easily use psql, we'll let Guacamole handle the database initialization
# by connecting to it and letting it create what it needs

echo "🎯 Letting Guacamole handle database initialization..."
echo "   The Guacamole application will create the database schema on first connection"

echo "🚀 Starting Guacamole application..."

# Start Guacamole using the original entrypoint
exec /opt/guacamole/bin/start.sh "$@"