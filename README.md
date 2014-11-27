# statflow

Don't sample.

Statflow is a tool for collecting and submitting stats, in this case to Graphite.  It also collects the timestamp at which the event occured, which makes it great for situations where you can't rely on messages arriving in order or on time.  The goal is to push this data into Graphite early and often after we receive it.

# What it's doing

Metrics come into statsflow via UDP.  They're held in memory until the TTL (default: 10 seconds) expires, and submitted to Graphite on the flush interval (default: 2 seconds).  It's designed to be fast, though it will consume a lot of RAM in the event that you're holding a lot of data for a long time.

# Command Line Options

```
Usage of /tmp/go-build309371040/command-line-arguments/_obj/exe/flow:
  -address=":8126": UDP service address
  -bucket-prefix="bucket.": Default prefix for buckets
  -count-prefix="count.": Default prefix for counts
  -debug=false: print statistics sent to graphite
  -default-ttl=10: Default TTL
  -flush-interval=2: Flush interval (seconds)
  -graphite="127.0.0.1:2003": Graphite service address (or - to disable)
  -mean-prefix="mean.": Default prefix for means
  -metric-prefix="": Default Graphite Prefix
  -version=false: print version string
```