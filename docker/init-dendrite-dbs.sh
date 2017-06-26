#!/bin/bash
set -e

sql_command=""
for i in account device media_api sync_api room_server server_key federation_sender; do
    db="dendrite_$i"
    sql_command="$sql_command CREATE DATABASE $db;"
done

echo "$sql_command" | psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER"
