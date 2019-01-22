#!/usr/bin/env bash

if [ -z "$1" ]; then
  echo "Enter numeric argument for n times to send test data to graphite"
  echo "Optionally enter 2nd argument for stag host, if not localhost"
  exit
fi
if [ -z "$2" ]; then
  if pgrep -x "stag" > /dev/null; then
    STAGHOST=localhost
    echo "Stag is running.  Proceeding to send test data to stag on $STAGHOST $1 times x 8."
  else
    echo "Make sure stag is running locally first, then retry"
    echo "or enter 2nd argument for stag host, if not localhost"
    exit
  fi
else
  STAGHOST="$2"
  echo "Proceeding to send test data to stag on $STAGHOST $1 times x 8."
fi

n=1
while [ $n -le $1 ]; do
  echo -n "."
  timestamp=$(date +%s)
  `echo stag_testing_metrics:first_thing:0.1g@$timestamp | nc $STAGHOST 8126`
  `echo stag_testing_metrics:first_thing:0.2g@$timestamp | nc $STAGHOST 8126`
  `echo stag_testing_metrics:first_thing:0.5g@$timestamp | nc $STAGHOST 8126`
  `echo stag_testing_metrics:first_thing:1.0g@$timestamp | nc $STAGHOST 8126`
  `echo stag_testing_metrics:second_thing:0.1g@$timestamp | nc $STAGHOST 8126`
  `echo stag_testing_metrics:second_thing:0.2g@$timestamp | nc $STAGHOST 8126`
  `echo stag_testing_metrics:second_thing:0.5g@$timestamp | nc $STAGHOST 8126`
  `echo stag_testing_metrics:second_thing:1.0g@$timestamp | nc $STAGHOST 8126`
  (( n++ ))
done
