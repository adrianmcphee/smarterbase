package smarterbase

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// LoadTestConfig configures a load test run
type LoadTestConfig struct {
	Duration       time.Duration
	Concurrency    int
	OperationMix   OperationMix
	DataSize       int
	KeyPrefix      string
	KeyCount       int
	TargetRPS      int
}

// OperationMix defines the ratio of different operations
type OperationMix struct {
	ReadPercent   int
	WritePercent  int
	DeletePercent int
}

// LoadTestResults contains the results of a load test
type LoadTestResults struct {
	Duration        time.Duration
	TotalOperations int64
	SuccessfulOps   int64
	FailedOps       int64
	Reads           int64
	Writes          int64
	Deletes         int64
	MinLatency      float64
	MaxLatency      float64
	AvgLatency      float64
	P95Latency      float64
	P99Latency      float64
	OperationsPerSec float64
}

// LoadTester provides load testing capabilities
type LoadTester struct {
	store    *Store
	config   LoadTestConfig
	logger   Logger
	results  *LoadTestResults
	stopChan chan struct{}
}

// NewLoadTester creates a new load tester
func NewLoadTester(store *Store, config LoadTestConfig) *LoadTester {
	return &LoadTester{
		store:    store,
		config:   config,
		logger:   store.logger,
		stopChan: make(chan struct{}),
		results:  &LoadTestResults{MinLatency: 999999.0},
	}
}

// Run executes the load test
func (lt *LoadTester) Run(ctx context.Context) (*LoadTestResults, error) {
	start := time.Now()
	
	var wg sync.WaitGroup
	for i := 0; i < lt.config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			lt.worker(ctx, workerID)
		}(i)
	}
	
	select {
	case <-time.After(lt.config.Duration):
		close(lt.stopChan)
	case <-ctx.Done():
		close(lt.stopChan)
	}
	
	wg.Wait()
	lt.results.Duration = time.Since(start)
	lt.results.OperationsPerSec = float64(lt.results.TotalOperations) / lt.results.Duration.Seconds()
	
	return lt.results, nil
}

func (lt *LoadTester) worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-lt.stopChan:
			return
		case <-ctx.Done():
			return
		default:
		}
		
		roll := rand.Intn(100)
		keyNum := rand.Intn(lt.config.KeyCount)
		key := fmt.Sprintf("%s/%d.json", lt.config.KeyPrefix, keyNum)
		
		start := time.Now()
		var err error
		
		if roll < lt.config.OperationMix.ReadPercent {
			var data map[string]interface{}
			err = lt.store.GetJSON(ctx, key, &data)
			atomic.AddInt64(&lt.results.Reads, 1)
		} else if roll < lt.config.OperationMix.ReadPercent+lt.config.OperationMix.WritePercent {
			data := map[string]interface{}{"id": rand.Int63(), "data": "test"}
			err = lt.store.PutJSON(ctx, key, data)
			atomic.AddInt64(&lt.results.Writes, 1)
		} else {
			err = lt.store.Delete(ctx, key)
			atomic.AddInt64(&lt.results.Deletes, 1)
		}
		
		_ = time.Since(start).Seconds() * 1000 // latency tracking for future use
		atomic.AddInt64(&lt.results.TotalOperations, 1)

		if err != nil {
			atomic.AddInt64(&lt.results.FailedOps, 1)
		} else {
			atomic.AddInt64(&lt.results.SuccessfulOps, 1)
		}
	}
}
