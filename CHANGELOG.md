# Stag Changelog

## 0.5.1

* updating Go version to current 1.11
* linting and refactoring

## 0.4.6

* Added logfile parameter and capability to write to logs.

## 0.4.5

* Packaging fixes, included init script and sysconfig file in dist dir

## 0.4.4

* Added Cassandra write timeout to allow configurability of Graphite connection deadline

## 0.4.3

* Added workaround for issue that arises when long Graphite connection interruptions occur.
* Improved connection issue logging

## 0.4.2

* Separated time slice calculation and metric submission into separate goroutines for performance
* Fixed shutdown message to be emitted through log instead of fmt.Printf

## 0.4.1

* Added flush delay option to reduce sending rapidly updated metrics to Graphite too often, which can pollute the cache.