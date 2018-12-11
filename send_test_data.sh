#!/usr/bin/env bash

if [ -z "$1" ]; then
  echo "Enter numeric argument for n times to send test data to graphite"
  exit
fi
if pgrep -x "stag" > /dev/null; then
  echo "Stag is running.  Proceeding to send test data to graphite $1 times x 8."
else
  echo "Make sure stag is running locally first"
  exit
fi

n=1
while [ $n -le $1 ]; do
  echo -n "."
  timestamp=$(date +%s)
  `echo stag_testing_metrics:first_thing:0.1g@$timestamp | nc localhost 8126`
  `echo stag_testing_metrics:first_thing:0.2g@$timestamp | nc localhost 8126`
  `echo stag_testing_metrics:first_thing:0.5g@$timestamp | nc localhost 8126`
  `echo stag_testing_metrics:first_thing:1.0g@$timestamp | nc localhost 8126`
  `echo stag_testing_metrics:second_thing:0.1g@$timestamp | nc localhost 8126`
  `echo stag_testing_metrics:second_thing:0.2g@$timestamp | nc localhost 8126`
  `echo stag_testing_metrics:second_thing:0.5g@$timestamp | nc localhost 8126`
  `echo stag_testing_metrics:second_thing:1.0g@$timestamp | nc localhost 8126`
  (( n++ ))
done
