package main

import (
	"fmt"
	"math/rand"
	// "strconv"
	"time"
)

// Slice of metric data for a given period of time
type TimeSlice struct {
	Name   string      // Graphite name of the metric
	Values []float64   // Values of events for this period
	Epoch  int         // epoch of time slice (i.e. events happened here)
	TTL    *time.Timer // TTL timer for the slice
}

// Used to create a new TimeSlice
func (t *TimeSlice) Create(m *Metric) {
	t.Name = m.Name
	t.Epoch = m.Epoch
	// README: Keep me in sync with the time under the Add method
	// TODO: Make this configurable
	t.TTL = time.NewTimer(10 * time.Second) // TODO: Make this configurable
	t.Values = make([]float64, 10)
	t.Add(m.Value)
}

// Add a value to a TimeSlice
func (t *TimeSlice) Add(v float64) {
	t.Values = append(t.Values, v)
	// NOTE: Keep me in sync with the time under the Create method
	// TODO: Make this configurable
	t.TTL.Reset(10 * time.Second)
}

type MetricCalculator interface {
	Value() float64
}

type AverageContents struct {
	values []float64
}

func (a AverageContents) Value() float64 {
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

// A slice container that contains a ticker for iteration
// TODO: mutex per slice container that prevents the main loop from adding metrics to the timeslice list or calculating values from the timeslice list.
// 		Goroutine in Create will do submission handling
type SliceContainer struct {
	Name         string             // Graphite name of the metric
	SliceMap     map[int]*TimeSlice // Time Slices for this metric
	ActiveSlices map[int]int        // Set of *active* (timeslices are removed from this after they've been submitted, and re-added after new data comes in)
	SubmitTicker *time.Ticker       // Ticker for submission
	Input        chan *Metric       // Input channel for new Metrics
}

func (s *SliceContainer) Create(m *Metric) {
	go func() {
		// fmt.Println("SliceContainer.Create\tInner loop starting")
		for {
			select {
			case i := <-s.Input:
				// fmt.Println("SliceContainer.Create\tGot input from inner channel")
				s.Add(i)
				// fmt.Println("SliceContainer.Create\tAdded value to Slice")
				s.ActiveSlices[i.Epoch] = i.Epoch
				// fmt.Println("SliceContainer.Create\tAdded value to active slices")
			case <-s.SubmitTicker.C:
				if len(s.ActiveSlices) == 0 {
					break
				} // Break if there's no data yet.

				// Now iterate over each active slice
				for _, Slice := range s.ActiveSlices {
					// TODO: Handle these calculations via graphite output
					_ = MetricCalculator(AverageContents{values: s.SliceMap[Slice].Values})
					_ = MetricCalculator(CountContents{values: s.SliceMap[Slice].Values})
					// fmt.Println("Average of all values is ", a.Value())
					// fmt.Println("Count of all values is ", strconv.FormatFloat(c.Value(), 'f', 2, 64))

					// Remove these slices from the active list
					delete(s.ActiveSlices, Slice)
				}
			}
			// fmt.Println("SliceContainer.Create\tFinished inner loop")
		}
	}()
}

func (s *SliceContainer) Add(m *Metric) {
	// fmt.Println("SliceContainer.Add\tAdding Value")
	s.SliceMap[m.Epoch].Add(m.Value)
	// fmt.Println("SliceContainer.Add\tAdding Adding epoch to active list")
	s.ActiveSlices[m.Epoch] = m.Epoch
}

// Type for a single incoming metric
type Metric struct {
	Name  string  // Graphite name of the metric
	Value float64 // Values of events for this period
	Epoch int     // epoch of time slice (i.e. events happened here)
}

// Random strings
var letters = []rune("abcdefghijklmnopqrstuvwxyz")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func main() {
	MetricsIn := make(chan *Metric, 100)

	go func() {
		MetricMap := make(map[string]*SliceContainer)
		// fmt.Println("Got a MetricMap")

		for {
			select {
			case metric := <-MetricsIn:
				// fmt.Println(metric.Name + "." + strconv.Itoa(metric.Epoch))

				_, present := MetricMap[metric.Name]
				// Do all the stuff to initalize the new Metric
				if present != true {
					// fmt.Println("Metric not present in MetricMap, creating a new SliceContainer")
					// Create a new TimeSlice for that epoch
					MetricMap[metric.Name] = new(SliceContainer)
					MetricMap[metric.Name].Name = metric.Name
					MetricMap[metric.Name].SliceMap = make(map[int]*TimeSlice)
					MetricMap[metric.Name].ActiveSlices = make(map[int]int)
					MetricMap[metric.Name].SubmitTicker = time.NewTicker(time.Duration(2 * time.Second)) // TODO: Make this configurable
					MetricMap[metric.Name].Input = make(chan *Metric, 100)
					// fmt.Println("SliceContainer initalized")
					MetricMap[metric.Name].Create(metric)
					// fmt.Println("SliceContainer Created")
				}
				_, Epresent := MetricMap[metric.Name].SliceMap[metric.Epoch]
				if Epresent != true {
					MetricMap[metric.Name].SliceMap[metric.Epoch] = new(TimeSlice)
					MetricMap[metric.Name].SliceMap[metric.Epoch].Create(metric)
					go func() { // Fire off a TTL watcher for the new Epoch
						<-MetricMap[metric.Name].SliceMap[metric.Epoch].TTL.C
						delete(MetricMap[metric.Name].SliceMap, metric.Epoch)
					}()
				}
				go func() {
					// fmt.Println("Passing Input to SliceMap")
					MetricMap[metric.Name].Input <- metric
				}()
			}
		}
	}()

	StartTime := time.Now()
	fmt.Println(StartTime)
	// Generate some test data
	for i := 0; i <= 10000000; i++ {
		testMetric := Metric{Name: randSeq(2), Value: rand.Float64(), Epoch: rand.Intn(10)}
		// fmt.Println("Got a test metric")
		MetricsIn <- &testMetric
		// time.Sleep(100 * time.Millisecond)
		// fmt.Println("Passed in testMetric")
	}
	fmt.Println(time.Since(StartTime))
	time.Sleep(15 * time.Second) // Sleep 11 seconds so we have a chance to calculate and TTL
}
