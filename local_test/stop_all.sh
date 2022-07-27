#!/bin/bash -Eeu

cd $(dirname "$0")

source vars.env

echo "Stopping all from ${TEST_DIR}"

PID_FILE=${TEST_DIR}/pids.txt

if [ -f "$PID_FILE" ]
then
    while read pid
    do
        [[ -n "$pid" ]] || continue
        echo killing $pid
        kill $pid
    done < $PID_FILE
fi

rm -f $PID_FILE
