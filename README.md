# Stag
Statistics Aggregator

```
     {__}
      \/_____!
        \----|
ejm96   /|   |\
```

Stag is a tool for collecting and aggregating statistics from streams of data.  It also collects the timestamp at which the event occured, which makes it great for situations where you can't rely on messages arriving in order or on time.  Stag lets you collect many data points relatively quickly and submit them  data into Graphite early and often after we receive it.

# What it's doing

Metrics come into Stag via TCP.  The values are grouped together in memory by the metric's prefix and name until the TTL (default: 10 seconds) expires.  Every time the flush interval occurs (default: 2 seconds) all metrics that have been updated are submitted (or resubmitted) to Graphite.  It's designed to be fast, though it will consume a lot of RAM in the event that you're holding a lot of data for a long time.

# Input format

metric_prefix:metric_name:valueg@timestamp

* `metric_prefix` - The graphite prefix for the metric
* `metric_name` - Name of the metric
* `value` - Value of the metric in question.  Suffixed with a metric type ("g" to indicate that it's a gauge).  Other types of metrics could be added in the future (counts, etc.)
* timestamp - Timestamp (in UNIX epoch format) at which the measurement was read.

# Example usage

1. Start stag and point it at a Graphite server
1. In another terminal, echo some data into stag using netcat:  ```echo my_metrics:some_thing:0.5g@1427259669 | nc localhost 8126 ; echo my_metrics:some_thing:1g@1427259669 | nc localhost 8126```
1. Discover the new metics you've added in Graphite



# Command Line Options

```
Usage of stag:
  -address="0.0.0.0:8126": UDP service address
  -bucket-prefix="bucket.": Default prefix for buckets (note the trailing dot)
  -count-prefix="count.": Default prefix for counts (note the trailing dot)
  -debug=false: print statistics sent to graphite
  -default-ttl=10: Default TTL
  -flush-delay=1: Delay before flushing data to Graphite (seconds)
  -flush-interval=2: Flush to Graphite interval (seconds)
  -graphite="127.0.0.1:2003": Graphite service address
  -graphite-timeout=10: Default Graphite write timeout
  -maxprocs=2: Default max number of OS processes
  -mean-prefix="mean.": Default prefix for means (note the trailing dot)
  -metric-prefix="": Default Graphite Prefix
  -profilemode=false: Turn on app profiling
  -version=false: print version string
  -webAddress="127.0.0.1:8127": HTTP stats interface
```

# Internal Metrics

You can observe internal metrics about the process by connecting to the local webserver on port 8127 (by default).  Here's a list of the URLs available:

* ```/metrics``` - Internal statistics on what the service is doing
* ```/debug/vars``` - Lower level go statistcs about the service

# TODO

* Add testing.
* Make bucket sizes configurable per metric prefix and/or metric name.  Splitting this out into a JSON config structure is probably the most logical thing to do here.
* Add support for more types of calculations (percentiles, etc.). JSON driven configuration probably drives this too.

# Licence

Copyright © 2015 iovation Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
