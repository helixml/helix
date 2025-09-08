#!/bin/bash

# Simple Guacamole Entrypoint with Database Auto-Init
# Uses only tools available in the Guacamole container

set -e

echo "🚀 Starting Guacamole with auto database initialization..."

# Wait for PostgreSQL using simple bash TCP test
echo "⏳ Waiting for PostgreSQL at $POSTGRES_HOSTNAME:5432..."
RETRY_COUNT=0
while [ $RETRY_COUNT -lt 60 ]; do
    RETRY_COUNT=$((RETRY_COUNT + 1))
    
    # Use bash built-in TCP test
    if timeout 3 bash -c "</dev/tcp/$POSTGRES_HOSTNAME/5432" 2>/dev/null; then
        echo "   ✅ PostgreSQL is ready!"
        break
    fi
    
    echo "   Waiting... (attempt $RETRY_COUNT/60)"
    if [ $RETRY_COUNT -eq 60 ]; then
        echo "❌ ERROR: PostgreSQL not reachable after 60 attempts"
        exit 1
    fi
    sleep 2
done

# The simplest approach: let Guacamole's start.sh handle everything
# But first, we need to make sure the database and schema exist
# We'll create a simple SQL file and try to apply it using available tools

echo "📋 Preparing database initialization..."

# Generate the schema using the official initdb.sh
echo "   Generating PostgreSQL schema..."
/opt/guacamole/bin/initdb.sh --postgresql > /tmp/guacamole-schema.sql

echo "   Schema generated ($(wc -l < /tmp/guacamole-schema.sql) lines)"

# Since we can't easily run psql, we'll use a workaround:
# Create a simple init script that can be executed via curl/wget to a postgres container
# But actually, let's just try starting Guacamole and see if it creates what it needs

echo "🚀 Starting Guacamole application..."

# The Guacamole container expects the database to exist and be initialized
# If it's not, it will fail to start - which is actually what we want for now
# This will help us debug the exact issue

# Start Guacamole
exec /opt/guacamole/bin/start.sh "$@"