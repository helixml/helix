#!/bin/bash

# Guacamole Database Auto-Initialization Script
# Uses the official Guacamole initialization method

set -e

echo "🚀 Starting Guacamole database auto-initialization..."

# Wait for PostgreSQL to be ready
echo "⏳ Waiting for PostgreSQL to be ready..."
until PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c '\q' 2>/dev/null; do
  echo "   PostgreSQL is unavailable - sleeping"
  sleep 2
done

echo "✅ PostgreSQL is ready!"

# Check if guacamole_db exists, create if not
echo "🗄️  Checking if guacamole_db exists..."
PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tc "SELECT 1 FROM pg_database WHERE datname = 'guacamole_db'" | grep -q 1 || {
    echo "   Creating guacamole_db database..."
    PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "CREATE DATABASE guacamole_db"
}

# Check if Guacamole tables already exist
echo "🔍 Checking if Guacamole tables exist..."
TABLE_COUNT=$(PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name LIKE 'guacamole_%';" 2>/dev/null || echo "0")

if [ "$TABLE_COUNT" -gt "0" ]; then
    echo "✅ Guacamole tables already exist (found $TABLE_COUNT tables)"
    echo "🎉 Database initialization complete!"
else
    echo "📋 Initializing Guacamole database schema..."
    
    # Generate the official Guacamole PostgreSQL schema
    echo "   Generating schema using official Guacamole initdb..."
    docker run --rm --network helix_default \
        guacamole/guacamole:1.5.5 \
        /opt/guacamole/bin/initdb.sh --postgresql > /tmp/guacamole-schema.sql
    
    echo "   Applying schema to database..."
    PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" -f /tmp/guacamole-schema.sql
    
    echo "   Creating default admin user: ${GUACAMOLE_USERNAME:-guacadmin}"
    # The schema includes the default guacadmin user, but let's ensure it has the right password
    SALT=$(openssl rand -hex 32)
    PASSWORD_HASH=$(echo -n "${GUACAMOLE_PASSWORD:-guacadmin}${SALT}" | openssl sha256 | cut -d' ' -f2)
    
    # Update the default admin user password if using custom credentials
    if [ "${GUACAMOLE_PASSWORD:-guacadmin}" != "guacadmin" ]; then
        PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" << EOF
UPDATE guacamole_user 
SET password_hash = decode('$PASSWORD_HASH', 'hex'),
    password_salt = decode('$SALT', 'hex')
WHERE entity_id = (SELECT entity_id FROM guacamole_entity WHERE name = 'guacadmin' AND type = 'USER');
EOF
        echo "   ✅ Updated admin password"
    fi
    
    echo "🎉 Guacamole database initialization completed successfully!"
    echo "   👤 Admin user: ${GUACAMOLE_USERNAME:-guacadmin}"
    echo "   🔐 Password: [CONFIGURED]"
fi

echo "✅ Guacamole database auto-initialization finished!"