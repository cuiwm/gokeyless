package testapi

import (
	"sync"
	"time"

	"github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/go-metrics"
)

// Input is a JSON struct representing a test suite to be run.
type Input struct {
	Clients            []string `json:"clients"`
	Keyserver          string   `json:"keyserver"`
	Domain             string   `json:"domain,omitempty"`
	CertsPEM           string   `json:"certs,omitempty"`
	HashedToken        []byte   `json:"hashed_token,omitempty"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify,omitempty"`
	ServerIP           string   `json:"cf_ip,omitempty"`
	TestLen            string   `json:"testlen,omitempty"`
	Workers            string   `json:"workers,omitempty"`
}

// Results is a registry of metrics representing the success stats of an entire test suite.
type Results struct {
	metrics.Registry `json:"results"`
	Tests            map[string]*Test `json:"tests"`
}

// NewResults initializes a new API Results registry.
func NewResults() *Results {
	results := &Results{
		Registry: metrics.NewRegistry(),
		Tests:    make(map[string]*Test),
	}
	results.Register("latency", metrics.NewTimer())
	results.Register("success", metrics.NewCounter())
	results.Register("failure", metrics.NewCounter())
	return results
}

// TestFunc represents generic test to be run.
type TestFunc func() error

// Test represents the success stats for an individual test.
type Test struct {
	metrics.Registry `json:"results"`
	Errors           metrics.Registry `json:"errors,omitempty"`
	run              TestFunc
}

// RegisterTest initializes a new Test struct and adds it to results.
func (results *Results) RegisterTest(name string, run TestFunc) {
	test := &Test{
		Registry: metrics.NewRegistry(),
		Errors:   metrics.NewRegistry(),
		run:      run,
	}
	test.Register("latency", metrics.NewTimer())
	test.Register("success", metrics.NewCounter())
	test.Register("failure", metrics.NewCounter())
	results.Tests[name] = test
}

// RunTests continually runs the tests stored in results for testLen.
func (results *Results) RunTests(testLen time.Duration, workers int) {
	log.Debugf("Running tests for %v with %d workers", testLen, workers)
	tests := make(chan string, workers)
	for i := 0; i < workers; i++ {
		go func() {
			for name := range tests {
				test := results.Tests[name]
				testStart := time.Now()
				if err := test.run(); err != nil {
					results.Get("failure").(metrics.Counter).Inc(1)
					test.Get("failure").(metrics.Counter).Inc(1)
					errCount := metrics.GetOrRegisterCounter(err.Error(), test.Errors)
					errCount.Inc(1)
					log.Infof("--- %s - Running %s: %v", "FAIL", name, err)
				} else {
					test.Get("success").(metrics.Counter).Inc(1)
					results.Get("success").(metrics.Counter).Inc(1)
					log.Infof("--- %s - Running %s", "PASS", name)
				}
				test.Get("latency").(metrics.Timer).UpdateSince(testStart)
				results.Get("latency").(metrics.Timer).UpdateSince(testStart)
			}
		}()
	}

	timeout := time.After(testLen)
	for {
		for name := range results.Tests {
			select {
			case <-timeout:
				close(tests)
				return

			case tests <- name:
			}
		}
	}
}

// RunBenchmarkTests runs each tests repetitively with multiple goroutines.
func (results *Results) RunBenchmarkTests(repeats, workers int) {
	log.Debugf("Running each test for %d times with %d workers", repeats, workers)

	var wg sync.WaitGroup
	for name := range results.Tests {
		test := results.Tests[name]
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(name string, test *Test) {
				defer wg.Done()
				for i := 0; i < repeats; i++ {
					testStart := time.Now()
					err := test.run()
					test.Get("latency").(metrics.Timer).UpdateSince(testStart)
					results.Get("latency").(metrics.Timer).UpdateSince(testStart)
					if err != nil {
						results.Get("failure").(metrics.Counter).Inc(1)
						test.Get("failure").(metrics.Counter).Inc(1)
						errCount := metrics.GetOrRegisterCounter(err.Error(), test.Errors)
						errCount.Inc(1)
						log.Infof("--- %s - Running %s: %v", "FAIL", name, err)
					} else {
						test.Get("success").(metrics.Counter).Inc(1)
						results.Get("success").(metrics.Counter).Inc(1)
						log.Infof("--- %s - Running %s", "PASS", name)
					}
				}
			}(name, test)
		}
	}

	wg.Wait()
}
