#!/bin/bash

# Guacamole Entrypoint with Auto Database Initialization
# This script initializes the database if needed, then starts Guacamole

set -e

echo "🚀 Starting Guacamole with auto database initialization..."

# Install required tools if needed
if ! command -v psql &> /dev/null; then
    echo "📦 Installing PostgreSQL client..."
    echo "   Fixing apt directories..."
    mkdir -p /var/lib/apt/lists/partial
    chmod 755 /var/lib/apt/lists/partial
    echo "   Running apt-get update..."
    apt-get update -q
    echo "   Installing postgresql-client..."
    apt-get install -y -q postgresql-client netcat-openbsd
    echo "   ✅ Installation complete"
else
    echo "✅ PostgreSQL client already available"
fi

# Verify tools are available
echo "🔧 Verifying tools:"
echo "   psql: $(which psql 2>/dev/null || echo 'NOT FOUND')"
echo "   nc: $(which nc 2>/dev/null || echo 'NOT FOUND')"

# Debug connection parameters
echo "🔍 PostgreSQL connection parameters:"
echo "   POSTGRES_HOSTNAME: $POSTGRES_HOSTNAME"
echo "   POSTGRES_USER: $POSTGRES_USER"
echo "   POSTGRES_DATABASE: $POSTGRES_DATABASE"
echo "   POSTGRES_PASSWORD: [${#POSTGRES_PASSWORD} chars]"

# Test network connectivity first
echo "🌐 Testing network connectivity..."
if command -v nc &> /dev/null; then
    if nc -z "$POSTGRES_HOSTNAME" 5432 -w 3; then
        echo "   ✅ Network connection to $POSTGRES_HOSTNAME:5432 successful"
    else
        echo "   ❌ Network connection to $POSTGRES_HOSTNAME:5432 failed"
        echo "   🔍 Trying to resolve hostname..."
        nslookup "$POSTGRES_HOSTNAME" || echo "   ❌ DNS resolution failed"
        exit 1
    fi
else
    echo "   ⚠️  nc not available, skipping network test"
fi

# Wait for PostgreSQL to be ready (connect to main postgres DB first)
echo "⏳ Waiting for PostgreSQL to be ready..."
RETRY_COUNT=0
until PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOSTNAME" -U "$POSTGRES_USER" -d "postgres" -c '\q' 2>/dev/null; do
  RETRY_COUNT=$((RETRY_COUNT + 1))
  echo "   PostgreSQL is unavailable (attempt $RETRY_COUNT) - sleeping"
  if [ $RETRY_COUNT -eq 30 ]; then
    echo "❌ ERROR: PostgreSQL connection failed after 30 attempts"
    echo "🔍 Debug info:"
    echo "   Trying to connect to: $POSTGRES_HOSTNAME:5432"
    echo "   Database: postgres (main DB)"
    echo "   User: $POSTGRES_USER"
    
    # Try to see if the host is reachable
    if command -v nc &> /dev/null; then
      echo "   Testing network connectivity..."
      nc -z "$POSTGRES_HOSTNAME" 5432 && echo "   ✅ Port 5432 is open" || echo "   ❌ Port 5432 is not reachable"
    fi
    
    exit 1
  fi
  sleep 2
done

echo "✅ PostgreSQL is ready!"

# Check if guacamole_db exists, create if not
echo "🗄️  Checking if $POSTGRES_DATABASE exists..."
DB_EXISTS=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOSTNAME" -U "$POSTGRES_USER" -d "postgres" -tc "SELECT 1 FROM pg_database WHERE datname = '$POSTGRES_DATABASE'" 2>/dev/null | grep -q 1 && echo "1" || echo "0")

if [ "$DB_EXISTS" = "0" ]; then
    echo "   Creating $POSTGRES_DATABASE database..."
    PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOSTNAME" -U "$POSTGRES_USER" -d "postgres" -c "CREATE DATABASE $POSTGRES_DATABASE"
    echo "   ✅ Database created"
else
    echo "   ✅ Database already exists"
fi

# Check if Guacamole tables already exist
echo "🔍 Checking if Guacamole database is initialized..."
TABLE_COUNT=$(PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOSTNAME" -U "$POSTGRES_USER" -d "$POSTGRES_DATABASE" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name LIKE 'guacamole_%';" 2>/dev/null || echo "0")

if [ "$TABLE_COUNT" -gt "0" ]; then
    echo "✅ Guacamole database already initialized (found $TABLE_COUNT tables)"
else
    echo "📋 Initializing Guacamole database schema..."
    
    # Generate and apply the official Guacamole PostgreSQL schema
    echo "   Generating official schema..."
    /opt/guacamole/bin/initdb.sh --postgresql > /tmp/guacamole-schema.sql
    
    echo "   Applying schema to database..."
    PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOSTNAME" -U "$POSTGRES_USER" -d "$POSTGRES_DATABASE" -f /tmp/guacamole-schema.sql
    
    # Update admin user password if using custom credentials
    if [ "${GUACAMOLE_PASSWORD:-guacadmin}" != "guacadmin" ]; then
        echo "   🔐 Setting custom admin password..."
        SALT=$(openssl rand -hex 32)
        PASSWORD_HASH=$(echo -n "${GUACAMOLE_PASSWORD}${SALT}" | openssl sha256 | cut -d' ' -f2)
        
        PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOSTNAME" -U "$POSTGRES_USER" -d "$POSTGRES_DATABASE" << EOF
UPDATE guacamole_user 
SET password_hash = decode('$PASSWORD_HASH', 'hex'),
    password_salt = decode('$SALT', 'hex')
WHERE entity_id = (SELECT entity_id FROM guacamole_entity WHERE name = 'guacadmin' AND type = 'USER');
EOF
        echo "   ✅ Admin password updated"
    fi
    
    echo "🎉 Guacamole database initialized successfully!"
    echo "   👤 Admin user: ${GUACAMOLE_USERNAME:-guacadmin}"
    echo "   🔐 Password: [CONFIGURED]"
fi

echo "🚀 Starting Guacamole application..."

# Start Guacamole using the original entrypoint
exec /opt/guacamole/bin/start.sh "$@"