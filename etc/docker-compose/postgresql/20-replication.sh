#!/bin/bash
set -e

psql -U postgres -c "CREATE ROLE $REPLICATION_USER WITH REPLICATION PASSWORD '$REPLICATION_PASSWORD' LOGIN"

echo [*] $REPLICATION_ROLE instance configured!
