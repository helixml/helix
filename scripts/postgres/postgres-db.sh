#!/bin/bash

set -e
set -u

function create_user_and_database() {
	local database=$1
	echo "  Creating database '$database'"
	psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
	    CREATE DATABASE $database;
EOSQL
}

if [ -n "$POSTGRES_DATABASES" ]; then
	echo "Database creation requested: $POSTGRES_DATABASES"
	for db in $(echo $POSTGRES_DATABASES | tr ',' ' '); do
		create_user_and_database $db
	done
	echo "databases created"
fi