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
* `timestamp` - Timestamp (in UNIX epoch format) at which the measurement was read.

# Example usage

1. Start stag and point it at a Graphite server, e.g. ```stag -address=0.0.0.0:8126 -webAddress=127.0.0.1:8127 -bucket-prefix=bucket. -mean-prefix=mean. -count-prefix=count. -flush-interval=2 -flush-delay=1 -graphite=<some_graphite_host>:2013 -graphite-timeout=10 -logfile=./stag.log -maxprocs=4 -metric-prefix= -profilecpu=true```
1. In another terminal, echo some data into stag using netcat:  ```echo my_metrics:some_thing:0.5g@1427259669 | nc localhost 8126 ; echo my_metrics:some_thing:1g@1427259669 | nc localhost 8126```
1. Discover the new metrics you've added in Graphite


# Command Line Options

`stag --help` to see all options.

```
Usage of stag:
  -address string
    	UDP service address (default "0.0.0.0:8126")
  -bucket-prefix string
    	Default prefix for buckets (note the trailing dot) (default "bucket.")
  -count-prefix string
    	Default prefix for counts (note the trailing dot) (default "count.")
  -debug
    	print statistics sent to graphite
  -default-ttl int
    	Default TTL (default 10)
  -flush-delay int
    	Delay before flushing data to Graphite (seconds) (default 1)
  -flush-interval int
    	Flush to Graphite interval (seconds) (default 2)
  -graphite string
    	Graphite service address (default "127.0.0.1:2003")
  -graphite-timeout int
    	Default Graphite write timeout (default 10)
  -logfile string
    	Log File path (defaults to stdout)
  -maxprocs int
    	Default max number of OS processes (default 2)
  -mean-prefix string
    	Default prefix for means (note the trailing dot) (default "mean.")
  -metric-prefix string
    	Default Graphite Prefix
  -version
    	print version string and exit
  -webAddress string
    	HTTP stats interface (default "0.0.0.0:8127")
```

# Internal Metrics

You can observe internal metrics about the process by connecting to the local webserver on port 8127 (by default).  Here's a list of the URLs available:

* ```/metrics``` - Internal statistics on what the service is doing e.g.
  * e.g. `http://<host_running_stag>:8127/metrics`
  * or if testing locally:  `http://0.0.0.0:8127/metrics`
* ```/debug/vars``` - Lower level go statistcs about the service
  * e.g. `http://<host_running_stag>:8127/debug/vars`
  * or if testing locally: `http://0.0.0.0:8127/debug/vars`

# Profiling

Using the built-in endpoints provided by the pprof package, you can profile cpu and memory anytime stag is running.
This assumes that you have go installed and properly configured on your workstation, on a mac, e.g. `brew install go`

* `go tool pprof --help` for full documentation
* The following examples assume you are testing on localhost.  For other hosts replace '0.0.0.0' with target IP or hostname.
* For more info: [https://golang.org/pkg/net/http/pprof/]()

To view all currently available profiles, open http://0.0.0.0:8127/debug/pprof/ in your browser.

## Command Line

* CPU profile (default): `go tool pprof http://0.0.0.0:8127`
  * 30 seconds is default, adjust to taste, e.g. http://0.0.0.0:8127?seconds=60
  * Explicit style for default:  `http://0.0.0.0:8127/debug/pprof/profile?seconds=30`

* Memory heap profile (default inuse\_space): `go tool pprof http://0.0.0.0:8127/debug/pprof/heap`
* For alloc\_space, e.g. `go tool pprof -alloc_space http://0.0.0.0:8127/debug/pprof/heap`


* Output formats, .png .pdf .gif et al.
  * e.g. CPU profile output to .png - `go tool pprof -png http://0.0.0.0:8127`
  * e.g. CPU profile output to .pdf - `go tool pprof -pdf http://0.0.0.0:8127`
  * e.g. Memory heap profile output to .png `go tool pprof -png http://0.0.0.0:8127/debug/pprof/heap`

## Web UI

Launch on 0.0.0.0, port 6060 (or port of your choice), an interactive web interface that can be used to navigate through
various views of a profile.  Same syntax as command line, adding after `pprof` the format 'http' with host:port, i.e. `-http 0.0.0.0:6060`

* CPU profile (default 30s): `go tool pprof -http 0.0.0.0:6060 http://0.0.0.0:8127`
* CPU profile for a minute: `go tool pprof -http 0.0.0.0:6060 http://0.0.0.0:8127?seconds=60`
* Memory heap profile (inuse\_space): `go tool pprof -http 0.0.0.0:6060 http://0.0.0.0:8127/debug/pprof/heap`
* Memory heap profile (alloc\_space): `go tool pprof -http 0.0.0.0:6060 -alloc_space http://0.0.0.0:8127/debug/pprof/heap`

# TODO

* Add testing.
* Make bucket sizes configurable per metric prefix and/or metric name.  Splitting this out into a JSON config structure is probably the most logical thing to do here.
* Add support for more types of calculations (percentiles, etc.). JSON driven configuration probably drives this too.
