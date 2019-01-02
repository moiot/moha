#!/bin/bash
set -e

echo [*] configuring $REPLICATION_ROLE instance
cp /var/lib/postgresql.conf $PGDATA/
echo "max_connections = $MAX_CONNECTIONS" >> "$PGDATA/postgresql.conf"

# We set master replication-related parameters for both slave and master,
# so that the slave might work as a primary after failover.
echo "wal_level = hot_standby" >> "$PGDATA/postgresql.conf"
echo "wal_keep_segments = $WAL_KEEP_SEGMENTS" >> "$PGDATA/postgresql.conf"
echo "max_wal_senders = $MAX_WAL_SENDERS" >> "$PGDATA/postgresql.conf"
# slave settings, ignored on master
echo "hot_standby = on" >> "$PGDATA/postgresql.conf"
echo "wal_log_hints = on" >> "$PGDATA/postgresql.conf"
echo "default_transaction_read_only = off" >> "$PGDATA/postgresql.conf"

echo "host replication $REPLICATION_USER 0.0.0.0/0 trust" >> "$PGDATA/pg_hba.conf"
