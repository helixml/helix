#!/bin/bash

# Guacamole Database Initialization Script
# This script initializes the Guacamole database with configurable admin credentials

set -e

# Default values
DEFAULT_USERNAME="guacadmin"
DEFAULT_PASSWORD="guacadmin"

# Use environment variables or defaults
GUAC_USERNAME="${GUACAMOLE_USERNAME:-$DEFAULT_USERNAME}"
GUAC_PASSWORD="${GUACAMOLE_PASSWORD:-$DEFAULT_PASSWORD}"

echo "Initializing Guacamole database with admin user: $GUAC_USERNAME"

# Wait for PostgreSQL to be ready
until PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c '\q' 2>/dev/null; do
  echo "PostgreSQL is unavailable - sleeping"
  sleep 1
done

echo "PostgreSQL is up - executing database initialization"

# Check if guacamole_db exists, create if not
PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tc "SELECT 1 FROM pg_database WHERE datname = 'guacamole_db'" | grep -q 1 || \
PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "CREATE DATABASE guacamole_db"

# Check if tables already exist
TABLE_COUNT=$(PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name LIKE 'guacamole_%';" 2>/dev/null || echo "0")

if [ "$TABLE_COUNT" -gt "0" ]; then
    echo "Guacamole tables already exist, checking admin user..."
    
    # Check if admin user exists
    USER_EXISTS=$(PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" -t -c "SELECT COUNT(*) FROM guacamole_entity WHERE name = '$GUAC_USERNAME' AND type = 'USER';" 2>/dev/null || echo "0")
    
    if [ "$USER_EXISTS" -eq "0" ]; then
        echo "Creating admin user: $GUAC_USERNAME"
        # Generate password hash (SHA-256 with salt)
        SALT=$(openssl rand -hex 32)
        # Guacamole uses SHA-256(password + salt)
        PASSWORD_HASH=$(echo -n "${GUAC_PASSWORD}${SALT}" | openssl sha256 | cut -d' ' -f2)
        
        PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" << EOF
INSERT INTO guacamole_entity (name, type) VALUES ('$GUAC_USERNAME', 'USER');
INSERT INTO guacamole_user (entity_id, password_hash, password_salt)
SELECT 
    entity_id,
    decode('$PASSWORD_HASH', 'hex'),
    decode('$SALT', 'hex')
FROM guacamole_entity WHERE name = '$GUAC_USERNAME' AND type = 'USER';

-- Grant admin all system permissions
INSERT INTO guacamole_system_permission (entity_id, permission)
SELECT entity_id, permission
FROM (
    SELECT entity_id FROM guacamole_entity WHERE name = '$GUAC_USERNAME' AND type = 'USER'
) AS admin_user
CROSS JOIN (
    VALUES ('ADMINISTER'), ('CREATE_CONNECTION'), ('CREATE_CONNECTION_GROUP'), 
           ('CREATE_SHARING_PROFILE'), ('CREATE_USER'), ('CREATE_USER_GROUP')
) AS permissions(permission);

-- Grant admin all connection permissions
INSERT INTO guacamole_connection_permission (entity_id, connection_id, permission)
SELECT admin_user.entity_id, connections.connection_id, permissions.permission
FROM (
    SELECT entity_id FROM guacamole_entity WHERE name = '$GUAC_USERNAME' AND type = 'USER'
) AS admin_user
CROSS JOIN guacamole_connection AS connections
CROSS JOIN (
    VALUES ('READ'), ('UPDATE'), ('DELETE'), ('ADMINISTER')
) AS permissions(permission);
EOF
        echo "Admin user created successfully"
    else
        echo "Admin user already exists"
    fi
else
    echo "Initializing Guacamole database schema..."
    # Run the original SQL initialization script
    PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" -f /docker-entrypoint-initdb.d/guacamole-initdb.sql
    
    # Update the default admin user with custom credentials if different
    if [ "$GUAC_USERNAME" != "$DEFAULT_USERNAME" ] || [ "$GUAC_PASSWORD" != "$DEFAULT_PASSWORD" ]; then
        echo "Updating admin credentials..."
        
        # Generate new password hash
        SALT=$(openssl rand -hex 32)
        PASSWORD_HASH=$(echo -n "${GUAC_PASSWORD}${SALT}" | openssl sha256 | cut -d' ' -f2)
        
        PGPASSWORD=$POSTGRES_PASSWORD psql -h "postgres" -U "$POSTGRES_USER" -d "guacamole_db" << EOF
-- Update entity name if changed
UPDATE guacamole_entity SET name = '$GUAC_USERNAME' WHERE name = '$DEFAULT_USERNAME' AND type = 'USER';

-- Update password
UPDATE guacamole_user 
SET password_hash = decode('$PASSWORD_HASH', 'hex'),
    password_salt = decode('$SALT', 'hex')
WHERE entity_id = (SELECT entity_id FROM guacamole_entity WHERE name = '$GUAC_USERNAME' AND type = 'USER');
EOF
        echo "Admin credentials updated successfully"
    fi
fi

echo "Guacamole database initialization completed"