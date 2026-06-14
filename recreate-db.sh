#!/usr/bin/env bash
# Drops and recreates the nvims-sms database, applies the schema, and seeds data.
set -euo pipefail

DB="nvims"
DB_USER="nvims"
DB_PASS="jjnhbFC56RDWRTJHBjhb98uibe"
SQL="$HOME/nvims-sms/src/nvims-sms.sql"
SEED="$HOME/nvims/private_seed/private_seed.py"

echo "==> Dropping database '$DB' (if exists)..."
sudo -u postgres psql -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB' AND pid <> pg_backend_pid();"
sudo -u postgres psql -c "DROP DATABASE IF EXISTS \"$DB\";"

echo "==> Creating database '$DB'..."
sudo -u postgres psql -c "CREATE DATABASE \"$DB\";"

echo "==> Creating user '$DB_USER' (if not exists)..."
sudo -u postgres psql -c "DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '$DB_USER') THEN
    CREATE USER $DB_USER WITH PASSWORD '$DB_PASS';
  END IF;
END \$\$;"

echo "==> Granting privileges..."
sudo -u postgres psql -c "ALTER DATABASE \"$DB\" OWNER TO $DB_USER;"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE \"$DB\" TO $DB_USER;"

echo "==> Applying schema..."
sudo -u postgres psql -d "$DB" < "$SQL"

echo "==> Granting table/sequence/function privileges..."
sudo -u postgres psql -d "$DB" -c "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO $DB_USER;"
sudo -u postgres psql -d "$DB" -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO $DB_USER;"
sudo -u postgres psql -d "$DB" -c "GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO $DB_USER;"

echo "==> Seeding data..."
python3 "$SEED"

echo "==> Done."
