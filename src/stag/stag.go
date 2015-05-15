package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	_ "expvar" // Exposes metrics under http://localhost/debug/vars
	"flag"
	"fmt"
	"github.com/codahale/metrics"
	"github.com/davecheney/profile"
	"log"
	"net"
	"net/http"
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
)

const VERSION = "0.4.0"

var signalchan chan os.Signal

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
	serviceAddress  = flag.String("address", "0.0.0.0:8126", "UDP service address")
	webAddress      = flag.String("webAddress", "127.0.0.1:8127", "HTTP stats interface")
	graphiteAddress = flag.String("graphite", "127.0.0.1:2003", "Graphite service address")
	graphitePrefix  = flag.String("metric-prefix", "", "Default Graphite Prefix")
	flushInterval   = flag.Int("flush-interval", 2, "Flush interval (seconds)")
	defaultTTL      = flag.Int("default-ttl", 10, "Default TTL")
	debug           = flag.Bool("debug", false, "print statistics sent to graphite")
	showVersion     = flag.Bool("version", false, "print version string")
	meanPrefix      = flag.String("mean-prefix", "mean.", "Default prefix for means (note the trailing dot)")
	countPrefix     = flag.String("count-prefix", "count.", "Default prefix for counts (note the trailing dot)")
	bucketPrefix    = flag.String("bucket-prefix", "bucket.", "Default prefix for buckets (note the trailing dot)")
	maxProcs        = flag.Int("maxprocs", 2, "Default max number of OS processes")
	profileMode     = flag.Bool("profilemode", false, "Turn on app profiling")
)

var (
	statsGraphiteConnections    = metrics.Counter("graphite_connection_count")
	statErrantMetricCount       = metrics.Counter("metric_errors")
	statProcessedMetricCount    = metrics.Counter("metric_count")
	statGraphitePointsSentCount = metrics.Counter("graphite_points_submitted_count")
	statIncomingConnectionCount = metrics.Counter("incoming_connection_count")
)

// Type for a single incoming metric
type Metric struct {
	Prefix string  // The Graphite prefix of the metric
	Name   string  // Graphite name of the metric
	Value  float64 // Values of events for this period
	Epoch  uint64  // epoch of time slice (i.e. events happened here)
}

var (
	MetricsIn      = make(chan *Metric)
	GraphiteOut    = make(chan string)
	ValueBuckets   = []float64{0, 0.125, 0.5, 1, 2, 5}
	MetricMap      = make(map[string]*SliceContainer)
	MetricMapMutex = &sync.RWMutex{}
)

// Slice of metric data for a given period of time
type TimeSlice struct {
	Prefix string    // Graphite prefix for the metric
	Name   string    // Graphite name of the metric
	Values []float64 // Values of events for this period
	Epoch  uint64    // epoch of time slice (i.e. events happened here)
	TTL    time.Time // The time when the epoch was last updated TODO: change name
	// TTL    *time.Timer // TTL timer for the slice
}

// Used to create a new TimeSlice
func (t *TimeSlice) Create(m *Metric) {
	t.Prefix = m.Prefix
	t.Name = m.Name
	t.Epoch = m.Epoch
	// README: Keep me in sync with the time under the Add method
	// t.TTL = time.NewTimer(time.Duration(*defaultTTL) * time.Second)
	t.TTL = time.Now()
	t.Values = make([]float64, 0)
}

// Add a value to a TimeSlice
func (t *TimeSlice) Add(v float64) {
	t.Values = append(t.Values, v)
	// NOTE: Keep me in sync with the time under the Create method
	t.TTL = time.Now()
	// t.TTL.Reset(time.Duration(*defaultTTL) * time.Second)
}

type MetricCalculator interface {
	Value() float64
}

type MeanContents struct {
	values []float64
}

func (a MeanContents) Value() float64 {
	sum := float64(0.0)
	length := len(a.values)
	for i := 0; i < length; i++ {
		sum += a.values[i]
	}
	return sum / float64(length)
}

type CountContents struct {
	values []float64
}

func (c CountContents) Value() float64 {
	return float64(len(c.values))
}

type BucketResults struct {
	buckets map[float64]float64
}
type BucketsAndValues struct {
	buckets []float64
	values  []float64
}

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
					br[b] += 1
				}
			} else if v >= b {
				br[b] += 1
			}
		}
	}

	return br
}

// A slice container that contains a ticker for iteration
type SliceContainer struct {
	Name     string                // Graphite name of the metric
	SliceMap map[uint64]*TimeSlice // Time Slices for this metric
	//TODO: Should ActiveSlices be named something more like RecentEpochs?
	//TODO: Should ActiveSlices be a map or a slice?
	ActiveSlices map[uint64]uint64 // Set of *active* (timeslices are removed from this after they've been submitted, and re-added after new data comes in)
	Input        chan *Metric      // Input channel for new Metrics
}

func (s *SliceContainer) Create(m *Metric) {
	go func() {
		for {
			i := <-s.Input
			MetricMapMutex.Lock()
			s.Add(i)
			s.ActiveSlices[i.Epoch] = i.Epoch
			MetricMapMutex.Unlock()
		}
	}()
}

func (s *SliceContainer) Add(m *Metric) {
	s.SliceMap[m.Epoch].Add(m.Value)
	s.ActiveSlices[m.Epoch] = m.Epoch
}

func submit(Slice *TimeSlice) {
	// TODO: Submit pickled data to Graphite for performance.

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

// Grabbed from stasdaemon.go
func parseMessage(line string) []*Metric {
	// Example message: metric_prefix:some.metric:1.24g:1415833364

	/* TODO: Evaluate something like the bitly statsdaemon style byte parser:
	https://github.com/bitly/statsdaemon/commit/c1816f025d3ccec416dc11098605087a6d7e138d */
	var packetRegexp = regexp.MustCompile("^([^:]+):([^:]+):([0-9.]+)(g)@([0-9]+)$")
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
		var epoch uint64
		modifier := item[4]
		switch modifier {
		default: // Assuming a g(gauge) modifier for now
			value, valueErr = strconv.ParseFloat(item[3], 64)
			if valueErr != nil {
				log.Printf("ERROR: failed to ParseFloat %s - %s", item[3], valueErr.Error())
			}
			epoch, epochErr = strconv.ParseUint(item[5], 10, 64)
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
			log.Println("Caught a connection error")
			continue
		}
	}
}

func SubmitToGraphite(client net.Conn, errCh chan error) {
	for {
		datain := <-GraphiteOut
		buffer := bytes.NewBuffer([]byte{})
		fmt.Fprintf(buffer, "%s", datain)
		data := buffer.Bytes()

		err := client.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err != nil {
			log.Println("SetWriteDeadline failed: %v\n", err)
		}

		_, err = client.Write(data)
		statGraphitePointsSentCount.Add()
		if err != nil {
			log.Println("Caught a connection error")
			errCh <- err
			break
		}
	}
}

func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	c, g := metrics.Snapshot()
	cJSON, err := json.Marshal(c)
	if err != nil {
		log.Println("ERROR formatting JSON for counts: %v", err)
	}
	gJSON, err := json.Marshal(g)
	if err != nil {
		log.Println("ERROR formatting JSON for gauges: %v", err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, "{ \"counts\": %s,\n \"gauges\": %s }", string(cJSON), string(gJSON))
}

func RunWebServer() {
	sock, err := net.Listen("tcp", *webAddress)
	if err != nil {
		log.Fatalf("Error starting HTTP listener: %s", err.Error())
	}
	log.Printf("Listening on http://%s", *webAddress)
	http.HandleFunc("/metrics", MetricsHandler)
	http.Serve(sock, nil)
}

func TTLLoop() {
	ttlTicker := time.NewTicker(time.Second * 5)
	for {
		<-ttlTicker.C
		// log.Println("TTLLoop firing")
		MetricMapMutex.Lock()
		for _, sliceContainer := range MetricMap {
			for epoch, slice := range sliceContainer.SliceMap {
				if _, present := sliceContainer.ActiveSlices[epoch]; present { // If the slice is not submitted yet it'll still be in ActiveSlices
					continue
				}
				if slice.TTL.Before(time.Now().Add(time.Duration(*defaultTTL*-1) * time.Second)) {
					delete(sliceContainer.SliceMap, epoch)
				}
			}
		}
		MetricMapMutex.Unlock()
	}
}

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Printf("stag v%s\n", VERSION)
		return
	} else {
		log.Printf("stag v%s starting\n", VERSION)
	}

	runtime.GOMAXPROCS(*maxProcs)

	if *profileMode {
		profileCfg := profile.Config{
			CPUProfile: true,
			MemProfile: true,
		}
		defer profile.Start(&profileCfg).Stop()
	}

	if *graphitePrefix != "" {
		*graphitePrefix = fmt.Sprintf("%s.", *graphitePrefix)
	}
	signalchan = make(chan os.Signal, 1)
	signal.Notify(signalchan, syscall.SIGTERM)

	FlushTicker := time.NewTicker(time.Duration(*flushInterval) * time.Second)
	// TODO: Collect metrics on number of keys in the metric map
	// MetricMap := make(map[string]*SliceContainer)
	//TODO: Turn metricTouchList into a heap with pop and push methods
	metricTouchList := make(map[string]time.Time) // metric name along with time updated
	go func() {
		/* TODO: Add MetricMap cleanup functionality (ie cleaning up old
		keys in the map that are haven't been updated in the TTL window) */
		for {
			select {
			case sig := <-signalchan:
				fmt.Printf("!! Caught signal %d... shutting down\n", sig)
				//TODO: Deal with submitting metrics before shutting down
				return
			case metric := <-MetricsIn:
				var mn string = metric.Prefix + metric.Name
				_, present := MetricMap[mn]
				// Do all the stuff to initalize the new Metric
				if present != true {
					MetricMapMutex.Lock()
					// Create a new SliceContainer for that epoch
					MetricMap[mn] = new(SliceContainer)
					MetricMap[mn].Name = metric.Name
					MetricMap[mn].SliceMap = make(map[uint64]*TimeSlice)
					MetricMap[mn].ActiveSlices = make(map[uint64]uint64)
					MetricMap[mn].Input = make(chan *Metric)
					MetricMap[mn].Create(metric)
					MetricMapMutex.Unlock()
				}

				// Initialize bit bits for a new epoch in the metric we've received
				MetricMapMutex.RLock()
				_, Epresent := MetricMap[mn].SliceMap[metric.Epoch]
				MetricMapMutex.RUnlock()
				if Epresent != true {
					MetricMapMutex.Lock()
					MetricMap[mn].SliceMap[metric.Epoch] = new(TimeSlice)
					MetricMap[mn].SliceMap[metric.Epoch].Create(metric)
					MetricMapMutex.Unlock()
				}
				MetricMap[mn].Input <- metric
				metricTouchList[mn] = time.Now()
			case <-FlushTicker.C: // TODO: Split this out into a goroutine?
				for metricName, _ := range metricTouchList {
					// fmt.Println("Found a point to flush: ", metricName)
					MetricMapMutex.Lock()
					for epoch, _ := range MetricMap[metricName].ActiveSlices { // Submit the slice(s) that have been recently updated
						submit(MetricMap[metricName].SliceMap[epoch])
						delete(MetricMap[metricName].ActiveSlices, epoch)
					}
					MetricMapMutex.Unlock()
					delete(metricTouchList, metricName)
				}
			}
		}
	}()

	go TTLLoop()
	go RunWebServer()
	go ConnectToGraphite()
	tcpListener()
}
