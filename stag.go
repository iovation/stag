package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	_ "expvar" // Exposes metrics under http://localhost/debug/vars
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	metrics "github.com/codahale/metrics"
)

//VERSION is the application release number
const VERSION = "0.5.0"

/*
TODO: A bunch of these flags (c/s)hould get turned into config file params.
		Something in the same idea as what Graphite does for metric wildcards
		seems like it'd make a lot of sense.  Config should include what
		kinds of output are expected for each kind of metric (count,
		throughput, histogram, etc.) as well as what prefix each metric type
		should get.
TODO: We should make use of pprof's HTTP server option to expose stats on running
		instances when in debug mode: https://golang.org/pkg/net/http/pprof/
*/
var (
	bucketPrefix         = flag.String("bucket-prefix", "bucket.", "Default prefix for buckets (note the trailing dot)")
	countPrefix          = flag.String("count-prefix", "count.", "Default prefix for counts (note the trailing dot)")
	debug                = flag.Bool("debug", false, "print statistics sent to graphite")
	defaultTTL           = flag.Int("default-ttl", 10, "Default TTL")
	flushDelay           = flag.Int("flush-delay", 1, "Delay before flushing data to Graphite (seconds)")
	flushInterval        = flag.Int("flush-interval", 2, "Flush to Graphite interval (seconds)")
	graphiteAddress      = flag.String("graphite", "127.0.0.1:2003", "Graphite service address")
	graphitePrefix       = flag.String("metric-prefix", "", "Default Graphite Prefix")
	graphiteWriteTimeout = flag.Int("graphite-timeout", 10, "Default Graphite write timeout")
	logFilePath          = flag.String("logfile", "", "Log File path (defaults to stdout)")
	maxProcs             = flag.Int("maxprocs", 2, "Default max number of OS processes")
	meanPrefix           = flag.String("mean-prefix", "mean.", "Default prefix for means (note the trailing dot)")
	serviceAddress       = flag.String("address", "0.0.0.0:8126", "UDP service address")
	showVersion          = flag.Bool("version", false, "print version string and exit")
	webAddress           = flag.String("webAddress", "0.0.0.0:8127", "HTTP stats interface")
)

var (
	statsGraphiteConnections    = metrics.Counter("graphite_connection_count")
	statErrantMetricCount       = metrics.Counter("metric_errors")
	statProcessedMetricCount    = metrics.Counter("metric_count")
	statGraphitePointsSentCount = metrics.Counter("graphite_points_submitted_count")
	statIncomingConnectionCount = metrics.Counter("incoming_connection_count")
)

//Metric is type for a single incoming metric
type Metric struct {
	Prefix string  // The Graphite prefix of the metric
	Name   string  // Graphite name of the metric
	Value  float64 // Values of events for this period
	Epoch  int64   // epoch of time slice (i.e. events happened here)
}

//MetricsIn
var (
	MetricsIn    = make(chan *Metric)
	SubmitBuffer = make(chan *TimeSlice, 1000)
	GraphiteOut  = make(chan string)
	ValueBuckets = []float64{0, 0.125, 0.5, 1, 2, 5}
)

//MetricStore ...
type MetricStore struct {
	sync.RWMutex
	Map   map[string]*SliceContainer
	Touch map[string]time.Time
}

//CreateMetricStore ...
func CreateMetricStore() *MetricStore {
	return &MetricStore{
		Map:   make(map[string]*SliceContainer),
		Touch: make(map[string]time.Time),
	}
}

//TimeSlice is slice of metric data for a given period of time
type TimeSlice struct {
	Prefix string    // Graphite prefix for the metric
	Name   string    // Graphite name of the metric
	Values []float64 // Values of events for this period
	Epoch  int64     // epoch of time slice (i.e. events happened here)
	TTL    time.Time // The time when the epoch was last updated TODO: change name
	// TTL    *time.Timer // TTL timer for the slice
}

//CreateTimeSlice ...
func CreateTimeSlice(m *Metric) *TimeSlice {
	return &TimeSlice{
		Prefix: m.Prefix,
		Name:   m.Name,
		Epoch:  m.Epoch,
		TTL:    time.Now(),
	}
}

// Add a value to a TimeSlice
func (t *TimeSlice) Add(v float64) {
	t.Values = append(t.Values, v)
	// NOTE: Keep me in sync with the time under the Create method
	t.TTL = time.Now()
	// t.TTL.Reset(time.Duration(*defaultTTL) * time.Second)
}

// MetricCalculator calculates metrics
type MetricCalculator interface {
	Value() float64
}

//MeanContents contains values
type MeanContents struct {
	values []float64
}

//Value of MeanContents
func (a MeanContents) Value() float64 {
	sum := float64(0.0)
	length := len(a.values)
	for i := 0; i < length; i++ {
		sum += a.values[i]
	}
	return sum / float64(length)
}

//CountContents contains values
type CountContents struct {
	values []float64
}

//Value returns values
func (c CountContents) Value() float64 {
	return float64(len(c.values))
}

//BucketResults contains buckets
type BucketResults struct {
	buckets map[float64]float64
}

//BucketsAndValues contains buckets and values
type BucketsAndValues struct {
	buckets []float64
	values  []float64
}

//BucketedResults return bucketed results
func BucketedResults(h BucketsAndValues) map[float64]float64 {
	br := make(map[float64]float64)
	sort.Float64s(h.buckets) // Sort the buckets so we can go over them small to big
	sort.Float64s(h.values)  // Sort the values before we iterate

	// iterate over the values, then iterate each bucket to see if it should fall into it's bucket
	// TODO: Improve the default algorithm or make it something the user can choose
	for bindex, b := range h.buckets {
		_, bpresent := br[b]
		if bpresent != true {
			br[b] = 0
		}
		// Since we've got a sorted list of values, iterate on them and put them
		// in the appropriate bucket
		for _, v := range h.values {
			if bindex+1 != len(h.buckets) {
				if v >= b && v < h.buckets[bindex+1] {
					br[b]++
				}
			} else if v >= b {
				br[b]++
			}
		}
	}

	return br
}

//SliceContainer contains a ticker for iteration
type SliceContainer struct {
	sync.Mutex
	Name     string               // Graphite name of the metric
	SliceMap map[int64]*TimeSlice // Time Slices for this metric
	//TODO: Should ActiveSlices be named something more like RecentEpochs?
	//TODO: Should ActiveSlices be a map or a slice?
	ActiveSlices map[int64]time.Time // Set of *active* (timeslices are removed from this after they've been submitted, and re-added after new data comes in)
	Input        chan *Metric        // Input channel for new Metrics
}

//CreateSliceContainer ...
func CreateSliceContainer(m *Metric) *SliceContainer {
	return &SliceContainer{
		Name:         m.Name,
		SliceMap:     make(map[int64]*TimeSlice),
		ActiveSlices: make(map[int64]time.Time),
		Input:        make(chan *Metric),
	}
}

//Loop ...
func (s *SliceContainer) Loop(m *Metric) {
	for {
		i, ok := <-s.Input
		if !ok {
			return
		}
		s.Add(i)
	}
}

//Add adds a metric to SliceContainer
func (s *SliceContainer) Add(m *Metric) {
	s.Lock()
	if _, ok := s.SliceMap[m.Epoch]; ok {
		s.SliceMap[m.Epoch].Add(m.Value)
		s.ActiveSlices[m.Epoch] = time.Now()
	} else {
		log.Println("Missing epoch ", m.Epoch, " - Some data will be missing from the aggregate!")
	}
	s.Unlock()
}

//CalculateSlices calcuates a slice
func CalculateSlices() {
	// TODO: Submit pickled data to Graphite for performance.
	for {
		Slice := <-SubmitBuffer

		// Means
		a := MetricCalculator(MeanContents{values: Slice.Values})
		GraphiteOut <- fmt.Sprintf("%s%s%s%s %f %d\n", *graphitePrefix, Slice.Prefix, *meanPrefix, Slice.Name, a.Value(), Slice.Epoch)

		// Counts
		c := MetricCalculator(CountContents{values: Slice.Values})
		GraphiteOut <- fmt.Sprintf("%s%s%s%s %f %d\n", *graphitePrefix, Slice.Prefix, *countPrefix, Slice.Name, c.Value(), Slice.Epoch)

		// Buckets
		bv := BucketsAndValues{buckets: ValueBuckets, values: Slice.Values}
		for bucket, count := range BucketedResults(bv) {
			GraphiteOut <- fmt.Sprintf("%s%s%s%s.%s %f %d\n", *graphitePrefix, Slice.Prefix, *bucketPrefix, Slice.Name, strings.Replace(strconv.FormatFloat(bucket, 'f', 3, 32), ".", "_", -1), count, Slice.Epoch)
		}
	}
}

// Grabbed from stasdaemon.go
func tcpListener() {
	log.Printf("Listening on %s/tcp", *serviceAddress)
	listener, err := net.Listen("tcp", *serviceAddress)
	if err != nil {
		log.Fatalf("Error starting TCP listener: %s", err.Error())
	}
	defer listener.Close()
	for {
		// Listen for an incoming connection.
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	statIncomingConnectionCount.Add()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		// Read the incoming data from the string
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		// Pass the line on to the parser
		packets := parseMessage(line)
		for _, p := range packets {
			MetricsIn <- p
		}
	}
}

var packetRegexp = regexp.MustCompile("^([^:]+):([^:]+):([0-9.]+)(g)@([0-9]+)$")

// Grabbed from stasdaemon.go
func parseMessage(line string) []*Metric {
	// Example message: metric_prefix:some.metric:1.24g:1415833364

	/* TODO: Evaluate something like the bitly statsdaemon style byte parser:
	https://github.com/bitly/statsdaemon/commit/c1816f025d3ccec416dc11098605087a6d7e138d */
	var output []*Metric
	var valueErr, epochErr error
	if line != "" {
		item := packetRegexp.FindStringSubmatch(line)

		if len(item) != 6 {
			statErrantMetricCount.Add()
			log.Printf("ERROR handling input: %s\n", line)
			return output
		}
		var value float64
		var epoch int64
		modifier := item[4]
		switch modifier {
		default: // Assuming a g(gauge) modifier for now
			value, valueErr = strconv.ParseFloat(item[3], 64)
			if valueErr != nil {
				log.Printf("ERROR: failed to ParseFloat %s - %s", item[3], valueErr.Error())
			}
			epoch, epochErr = strconv.ParseInt(item[5], 10, 64)
			if epochErr != nil {
				log.Printf("ERROR: failed to ParseInt %s - %s", item[5], epochErr.Error())
			}
		}

		metric := &Metric{
			Prefix: item[1] + ".", // Graphite Prefix
			Name:   item[2],
			Value:  value,
			Epoch:  epoch,
		}
		output = append(output, metric)
	}

	statProcessedMetricCount.Add()
	return output
}

//ConnectToGraphite connects to a graphite host
func ConnectToGraphite() {
	errCh := make(chan error)

	for {
		client, err := net.Dial("tcp", *graphiteAddress)
		statsGraphiteConnections.Add()
		if err != nil {
			log.Printf("Error with connection to: %s %s - RETRYING in 5s", *graphiteAddress, err.Error())
			time.Sleep(5 * time.Second)
			continue
		} else {
			log.Printf("Connected to Graphite: %s\n", *graphiteAddress)
			defer client.Close()
		}
		go SubmitToGraphite(client, errCh)
		err = <-errCh
		if err != nil {
			continue
		}
	}
}

//SubmitToGraphite submits metrics to the connected graphite host
func SubmitToGraphite(client net.Conn, errCh chan error) {
	for {
		datain := <-GraphiteOut
		buffer := bytes.NewBuffer([]byte{})
		fmt.Fprintf(buffer, "%s", datain)
		data := buffer.Bytes()

		err := client.SetWriteDeadline(time.Now().Add(time.Duration(*graphiteWriteTimeout) * time.Second))
		if err != nil {
			log.Printf("SetWriteDeadline failed: %v\n", err)
		}

		_, err = client.Write(data)
		statGraphitePointsSentCount.Add()
		if err != nil {
			log.Println("Caught a connection error: ", err)
			errCh <- err
			break
		}
	}
}

//MetricsHandler reads a request and writes a response
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	c, g := metrics.Snapshot()
	cJSON, err := json.Marshal(c)
	if err != nil {
		log.Printf("ERROR formatting JSON for counts: %v", err)
	}
	gJSON, err := json.Marshal(g)
	if err != nil {
		log.Printf("ERROR formatting JSON for gauges: %v", err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, "{ \"counts\": %s,\n \"gauges\": %s }", string(cJSON), string(gJSON))
}

//RunWebServer starts an HTTP listener
func RunWebServer() {
	sock, err := net.Listen("tcp", *webAddress)
	if err != nil {
		log.Fatalf("Error starting HTTP listener: %s", err.Error())
	}
	log.Printf("Listening on http://%s", *webAddress)
	http.HandleFunc("/metrics", MetricsHandler)
	http.Serve(sock, nil)
}

//TTLLoop ...
func (ms *MetricStore) TTLLoop() {
	ttlTicker := time.NewTicker(time.Second * 5)
	for {
		<-ttlTicker.C
		// log.Println("TTLLoop firing")
		ms.RLock()
		for _, sliceContainer := range ms.Map {
			sliceContainer.Lock()
			for epoch, slice := range sliceContainer.SliceMap {
				if _, ok := sliceContainer.ActiveSlices[epoch]; ok { // If the slice is not submitted yet it'll still be in ActiveSlices
					continue
				}
				if slice.TTL.Before(time.Now().Add(time.Duration(*defaultTTL*-1) * time.Second)) {
					delete(sliceContainer.SliceMap, epoch)
				}
			}
			sliceContainer.Unlock()
		}
		ms.RUnlock()
	}
}

//SubmitLoop ...
func (ms *MetricStore) SubmitLoop() {
	flushTicker := time.NewTicker(time.Duration(*flushInterval) * time.Second)
	for {
		<-flushTicker.C // TODO: Split this out into a goroutine?
		ms.Lock()
	FlushLoop:
		for metricName := range ms.Touch {
			sc := ms.Map[metricName]
			sc.Lock()
			for epoch, updateTime := range sc.ActiveSlices { // Submit the slice(s) that have been recently updated
				if time.Since(updateTime) > time.Duration(*flushDelay) {
					SubmitBuffer <- sc.SliceMap[epoch]
					// submit(MetricMap[metricName].SliceMap[epoch])
					delete(sc.ActiveSlices, epoch)
				} else {
					sc.Unlock()
					continue FlushLoop
				}
			}
			sc.Unlock()
			delete(ms.Touch, metricName)
		}
		ms.Unlock()
	}
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("stag v%s\n", VERSION)
		return
	}

	log.Printf("stag v%s starting\n", VERSION)

	if *logFilePath != "" {
		f, err := os.OpenFile(*logFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("Error opening log file: %v", err)
		}
		defer f.Close()
		log.SetOutput(f)
	}

	runtime.GOMAXPROCS(*maxProcs)

	if *graphitePrefix != "" {
		*graphitePrefix = fmt.Sprintf("%s.", *graphitePrefix)
	}
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	go func() {
		sig := <-gracefulStop
		fmt.Printf("caught sig: %+v", sig)
		fmt.Printf("\n!! Caught signal %d... shutting down\n", sig)
		fmt.Println("Waiting 2 seconds to finish processing")
		time.Sleep(2 * time.Second)
		log.Printf("!! Caught signal %d... shutting down\n", sig)
		//TODO: Deal with submitting metrics before shutting down
		os.Exit(0)
	}()

	// TODO: Collect metrics on number of keys in the metric map
	// TODO: Turn metricTouchList into a heap with pop and push methods
	ms := CreateMetricStore()
	go func() {
		/* TODO: Add MetricMap cleanup functionality (ie cleaning up old
		keys in the map that are haven't been updated in the TTL window) */
		for {
			metric := <-MetricsIn
			mn := metric.Prefix + metric.Name
			ms.RLock()
			sc, ok := ms.Map[mn]
			ms.RUnlock()
			// Do all the stuff to initalize the new Metric
			if !ok {
				ms.Lock()
				// Create a new SliceContainer for that epoch
				sc = CreateSliceContainer(metric)
				go sc.Loop(metric)
				ms.Map[mn] = sc
				ms.Unlock()
			}

			// Initialize bits for a new epoch in the metric we've received
			sc.Lock()
			if _, ok := sc.SliceMap[metric.Epoch]; !ok {
				sc.SliceMap[metric.Epoch] = CreateTimeSlice(metric)
			}
			sc.Unlock()
			sc.Input <- metric
			ms.Lock()
			ms.Touch[mn] = time.Now()
			ms.Unlock()
		}
	}()

	go ms.SubmitLoop()
	go ms.TTLLoop()
	go CalculateSlices()
	go RunWebServer()
	go ConnectToGraphite()
	tcpListener()
}
