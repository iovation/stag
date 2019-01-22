#!/usr/bin/env bash

if [ -z "$1" ]; then
  NUMPROCS=16
else
  NUMPROCS="$1"
fi

echo "Spawning $NUMPROCS processes"
for i in {1..$NUMPROCS}
do
  ( ./test/send_test_data.sh 100 & )
done


