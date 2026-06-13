#!/usr/bin/env python3
"""
private_seed.py — Seeds the nvims database with real/private institutional data.

This file is NOT committed to version control. It contains real people, programs,
locations, and other data specific to your institution.

Usage:
    pip install psycopg2-binary bcrypt
    python synthetic_data/private_seed.py [--dsn postgresql://localhost/nvims] [--clean]

Options:
    --dsn    PostgreSQL connection string (default: see DEFAULT_DSN)
    --clean  TRUNCATE all seeded tables and restart sequences before inserting
"""

import argparse
import sys

import bcrypt
import psycopg2
import psycopg2.extras

DEFAULT_DSN = "postgresql://nvims:jjnhbFC56RDWRTJHBjhb98uibe@localhost:5432/nvims"


def get_conn(dsn: str):
    conn = psycopg2.connect(dsn)
    conn.autocommit = False
    return conn


def seed(conn):
    cur = conn.cursor()

    # ── Add your real data below ──────────────────────────────────────────────
    #
    # Example: insert an organisation
    # cur.execute("""
    #     INSERT INTO training_organisations (rto_code, name, state_code)
    #     VALUES (%s, %s, %s)
    #     ON CONFLICT (rto_code) DO NOTHING
    # """, ('99999', 'My Institute', 'VIC'))
    #
    # Example: create an admin user (password will be bcrypt-hashed)
    # pw_hash = bcrypt.hashpw(b'changeme', bcrypt.gensalt()).decode()
    # cur.execute("""
    #     INSERT INTO app_users (username, password_hash, is_active)
    #     VALUES (%s, %s, true)
    #     ON CONFLICT (username) DO NOTHING
    # """, ('admin', pw_hash))
    #
    # ─────────────────────────────────────────────────────────────────────────

    conn.commit()
    print("private_seed: done.")


def main():
    ap = argparse.ArgumentParser(description="Seed nvims with private/real data")
    ap.add_argument("--dsn", default=DEFAULT_DSN)
    ap.add_argument("--clean", action="store_true", help="Truncate before seeding")
    args = ap.parse_args()

    conn = get_conn(args.dsn)
    try:
        if args.clean:
            print("--clean not implemented in private_seed; skipping.")
        seed(conn)
    except Exception as e:
        conn.rollback()
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
