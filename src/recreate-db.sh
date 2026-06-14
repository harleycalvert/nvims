#!/usr/bin/env bash
# Drops and recreates the nvims-sms database, applies the schema, and seeds data.
set -euo pipefail

ENV_FILE="$HOME/nvims/nvims.env"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: $ENV_FILE not found" >&2
  exit 1
fi
set -a; source "$ENV_FILE"; set +a

DB="nvims"
DB_USER="nvims"
DB_PASS="${DB_PASS:?DB_PASS not set in nvims.env}"
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

echo "==> Restarting Go server..."
fuser -k 8080/tcp 2>/dev/null || true
sleep 1
cd "$HOME/nvims-sms/src"
nohup go run ./cmd/server >> /tmp/nvims-server.log 2>&1 &
echo "==> Server restarted (PID $!, log: /tmp/nvims-server.log)"

echo "==> Done."
