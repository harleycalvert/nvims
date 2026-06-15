# NVIMS Installation Guide

NVIMS is an AVETMISS-compliant student management system built with Go and PostgreSQL. This guide walks through a fresh installation on a Linux server (Ubuntu/Debian examples shown).

---

## Prerequisites

| Dependency | Minimum version |
|---|---|
| Go | 1.22+ |
| PostgreSQL | 16+ |
| MinIO (or any S3-compatible store) | latest stable |
| Linux server | Ubuntu 22.04 / Debian 12 recommended |

Install Go from [https://go.dev/dl/](https://go.dev/dl/) and PostgreSQL from your distribution's package manager:

```bash
sudo apt install postgresql-16
```

---

## 1. Database setup

### Create database and user

```sql
sudo -u postgres psql <<'EOF'
CREATE USER nvims_user WITH PASSWORD 'changeme';
CREATE DATABASE nvims OWNER nvims_user;
EOF
```

### Apply the schema

The schema file is at `src/nvims.sql` in the repository root.

```bash
psql -U nvims_user -d nvims -f src/nvims.sql
```

The schema is versioned (current: v0.35). Re-running it on an existing database will fail on duplicate objects — see [Upgrading](#7-upgrading) for migration guidance.

---

## 2. Object storage (MinIO)

NVIMS stores uploaded documents in an S3-compatible bucket. MinIO is the recommended self-hosted option; AWS S3 or any compatible service also works.

### Install MinIO

```bash
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x minio
sudo mv minio /usr/local/bin/minio
```

### Create a systemd service for MinIO

```ini
# /etc/systemd/system/minio.service
[Unit]
Description=MinIO object storage
After=network.target

[Service]
User=minio-user
Environment=MINIO_ROOT_USER=minioadmin
Environment=MINIO_ROOT_PASSWORD=changeme
ExecStart=/usr/local/bin/minio server /var/lib/minio/data --console-address :9001
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd -r -s /sbin/nologin minio-user
sudo mkdir -p /var/lib/minio/data
sudo chown -R minio-user: /var/lib/minio

sudo systemctl daemon-reload
sudo systemctl enable minio
sudo systemctl start minio
```

### Create the bucket

Using the MinIO client (`mc`):

```bash
mc alias set local http://localhost:9000 <MINIO_ROOT_USER> <MINIO_ROOT_PASSWORD>
mc mb local/nvims-docs
```

The bucket name (`nvims-docs`) is the default used by the server. It can be overridden with the `MINIO_BUCKET` environment variable.

---

## 3. Environment configuration

The server reads all configuration from environment variables. The recommended pattern is a dedicated env file stored **outside the repository directory** so it is never accidentally committed.

### Required environment variables

| Variable | Description | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string (required) | none — server exits if unset |
| `MINIO_ENDPOINT` | MinIO host and port | `localhost:9000` |
| `MINIO_ROOT_USER` | MinIO access key | none |
| `MINIO_ROOT_PASSWORD` | MinIO secret key | none |
| `MINIO_BUCKET` | Bucket name for document storage | `nvims-docs` |

### Example env file

Create `/home/<user>/nvims/nvims.env` (or any path outside the repo):

```env
DATABASE_URL=postgres://nvims_user:changeme@localhost:5432/nvims
MINIO_ENDPOINT=localhost:9000
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=changeme
MINIO_BUCKET=nvims-docs
```

Replace all placeholder values with your actual credentials.

**Security note:** Keep this file outside the repository directory and restrict its permissions:

```bash
chmod 600 /home/<user>/nvims/nvims.env
```

---

## 4. Build

From the repository root:

```bash
cd src && go build -o ../nvims-sms ./cmd/server
```

This produces the `nvims-sms` binary at the repository root. Dependencies are fetched automatically by the Go toolchain on first build.

---

## 5. Run

### Manual (for testing)

Source the env file and run the binary. The server's `WorkingDirectory` must be `src/` because it serves static files relative to that path.

```bash
export $(grep -v '^#' /home/<user>/nvims/nvims.env | xargs)
cd /home/<user>/nvims-sms/src
../nvims-sms
```

The server listens on port **8080** and logs to stdout.

### Systemd service

Create the unit file at `/etc/systemd/system/nvims.service`:

```ini
[Unit]
Description=NVIMS
After=network.target postgresql@16-main.service minio.service
Requires=postgresql@16-main.service

[Service]
User=<your-user>
WorkingDirectory=/home/<your-user>/nvims-sms/src
EnvironmentFile=/home/<your-user>/nvims/nvims.env
ExecStart=/home/<your-user>/nvims-sms/nvims-sms
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable nvims
sudo systemctl start nvims
```

Check the service status:

```bash
sudo systemctl status nvims
sudo journalctl -u nvims -f
```

---

## 6. First login / initial setup

After the server is running, navigate to `http://<server-ip>:8080/login`.

A user account must exist in the database before anyone can log in. Create the first administrator directly in the database:

1. Insert a person record and assign the appropriate role via the `people`, `staff`, and `user_accounts` tables (refer to `src/nvims.sql` for the schema).
2. Once the first admin account is active, additional users can be created through the web interface at `/admin/people/new`.

---

## 7. Upgrading

### Application binary

Build a new binary and restart the service:

```bash
cd /home/<user>/nvims-sms/src
go build -o ../nvims-sms ./cmd/server
sudo systemctl restart nvims
```

### Schema migrations

The schema file `src/nvims.sql` represents the full target schema. For minor upgrades, apply only the relevant `ALTER TABLE` / `CREATE TABLE` statements from the changelog at the top of `nvims.sql` rather than re-running the whole file.

For a full reinstall (non-production / fresh environment only):

```bash
sudo -u postgres psql -c "DROP DATABASE nvims;"
sudo -u postgres psql -c "CREATE DATABASE nvims OWNER nvims_user;"
psql -U nvims_user -d nvims -f src/nvims.sql
```
