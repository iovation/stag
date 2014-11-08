package main

import (
    "fmt"
    "time"
    "strconv"
    "math/rand"
)

// Slice of metric data for a given period of time
type TimeSlice struct {
	Name	string			// Graphite name of the metric
	Values	[]float64		// Values of events for this period
	Epoch	int				// epoch of time slice (i.e. events happened here)
	TTL		*time.Timer		// TTL timer for the slice
}

// Type for a single incoming metric
type Metric struct {
	Name	string	// Graphite name of the metric
	Value	float64	// Values of events for this period
	Epoch	int	// epoch of time slice (i.e. events happened here)
}

// Used to create a new TimeSlice
func (t *TimeSlice) Create(m *Metric) {
	t.Name = m.Name
	t.Epoch = m.Epoch
	t.TTL = time.NewTimer(10 * time.Second) // NOTE: Keep me in sync with the time under the Add method
	t.Add(m.Value)
}

// Add a value to a TimeSlice
func (t *TimeSlice) Add(v float64) {
	t.Values = append(t.Values, v)
	t.TTL.Reset(10 * time.Second) // NOTE: Keep me in sync with the time under the Create method
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

func main() {	
	MetricsIn := make(chan *Metric)
	
	go func() {
		SliceMap := make(map[string] *TimeSlice)
		MasterSubmitTicker := time.NewTicker(time.Duration(2 * time.Second))

		for {
			select {
			case metric := <-MetricsIn:
				MetricKey := metric.Name + strconv.Itoa(metric.Epoch)
				slice, present := SliceMap[MetricKey]
				if present {
					// Add the value to the slice
					slice.Add(metric.Value)
				} else {
					// Create a new slice for that value
					SliceMap[MetricKey] = new(TimeSlice)
					SliceMap[MetricKey].Create(metric)
				}
			// If there's nothing coming in let's build a new map of slices to submit, then iterate and calculate them)
			// (or just submit them in the short term)
			case <-MasterSubmitTicker.C:
				if len(SliceMap) == 0 { break }	// Break if there's no data yet.
				for _, Slice := range SliceMap {
					_ = MetricCalculator(AverageContents{values: Slice.Values})
					// fmt.Println("Average of all values is ", a.Value())
					_ = MetricCalculator(CountContents{values: Slice.Values})
					// fmt.Println("Average of all values is ", strconv.FormatFloat(c.Value(), 'f', 2, 64))
				}					
			}
		}
	}()

	StartTime := time.Now()
	// Generate some test data
	for i := 0; i <= 10000000; i++ {
		testMetric := Metric{Name: "honk", Value: rand.Float64(), Epoch: rand.Intn(100000)}
		MetricsIn <- &testMetric
	}
	fmt.Println(time.Since(StartTime))
}