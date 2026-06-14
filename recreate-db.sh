#!/usr/bin/env bash
# Drop and recreate the nvims database, then apply the schema.
# Run from the project root: bash recreate-db.sh

set -euo pipefail

DBNAME="nvims"
DBUSER="nvims"
DBPASS="jjnhbFC56RDWRTJHBjhb98uibe"
DSN="postgresql://${DBUSER}:${DBPASS}@localhost:5432/${DBNAME}"
SCHEMA="src/nvims-sms.sql"

echo "==> Dropping database '${DBNAME}' (if it exists)..."
sudo -u postgres psql -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DBNAME}' AND pid <> pg_backend_pid();"
sudo -u postgres psql -c "DROP DATABASE IF EXISTS ${DBNAME};"

echo "==> Creating database '${DBNAME}'..."
sudo -u postgres psql -c "CREATE DATABASE ${DBNAME};"
sudo -u postgres psql -c "ALTER DATABASE ${DBNAME} OWNER TO ${DBUSER};"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE ${DBNAME} TO ${DBUSER};"

echo "==> Applying schema from ${SCHEMA}..."
psql "${DSN}" -f "${SCHEMA}"

echo "==> Granting table/sequence privileges..."
sudo -u postgres psql -d "${DBNAME}" -c "GRANT ALL PRIVILEGES ON ALL TABLES    IN SCHEMA public TO ${DBUSER};"
sudo -u postgres psql -d "${DBNAME}" -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ${DBUSER};"
sudo -u postgres psql -d "${DBNAME}" -c "GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO ${DBUSER};"

echo "==> Done. Run: cd src && go run ./cmd/server"
