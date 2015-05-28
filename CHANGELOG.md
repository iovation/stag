# Stag Changelog

## 0.4.4

## 0.4.3

* Added workaround for issue that arises when long Graphite connection interruptions occur.
* Improved connection issue logging

## 0.4.2

* Separated time slice calculation and metric submission into separate goroutines for performance
* Fixed shutdown message to be emitted through log instead of fmt.Printf

## 0.4.1

* Added flush delay option to reduce sending rapidly updated metrics to Graphite too often, which can pollute the cache.