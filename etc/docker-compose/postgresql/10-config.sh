#!/bin/bash
set -e

echo [*] configuring $REPLICATION_ROLE instance
cp /var/lib/postgresql.conf $PGDATA/

echo "host replication $REPLICATION_USER 0.0.0.0/0 trust" >> "$PGDATA/pg_hba.conf"
psql -U postgres -c "CREATE ROLE $REPLICATION_USER WITH REPLICATION PASSWORD '$REPLICATION_PASSWORD' LOGIN"

echo [*] $REPLICATION_ROLE instance configured!
