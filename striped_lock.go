package smarterbase

import (
	"hash/fnv"
	"sync"
)

// StripedLocks provides fine-grained locking using multiple mutexes
// to reduce contention compared to a single global mutex.
//
// How it works:
// - Hash the key to determine which stripe (mutex) to use
// - Multiple keys hash to different stripes → concurrent operations
// - Same key always hashes to same stripe → consistency
//
// Performance:
// - 32 stripes = ~32x better concurrency than single mutex
// - Negligible memory overhead (~256 bytes)
type StripedLocks struct {
	stripes []sync.RWMutex
	count   uint32
}

// NewStripedLocks creates a new striped lock with the specified number of stripes.
// Recommended: 32 for most use cases, 128 for high-concurrency scenarios.
func NewStripedLocks(stripeCount int) *StripedLocks {
	if stripeCount <= 0 {
		stripeCount = 32 // Default
	}
	return &StripedLocks{
		stripes: make([]sync.RWMutex, stripeCount),
		count:   uint32(stripeCount),
	}
}

// Lock acquires an exclusive lock for the given key.
// Returns an unlock function that MUST be called to release the lock.
//
// Example:
//
//	unlock := locks.Lock(key)
//	defer unlock()
//	// ... critical section
func (sl *StripedLocks) Lock(key string) func() {
	idx := sl.getStripeIndex(key)
	sl.stripes[idx].Lock()
	return func() {
		sl.stripes[idx].Unlock()
	}
}

// RLock acquires a shared read lock for the given key.
// Multiple readers can hold the lock simultaneously.
//
// Example:
//
//	unlock := locks.RLock(key)
//	defer unlock()
//	// ... read operation
func (sl *StripedLocks) RLock(key string) func() {
	idx := sl.getStripeIndex(key)
	sl.stripes[idx].RLock()
	return func() {
		sl.stripes[idx].RUnlock()
	}
}

// getStripeIndex returns the stripe index for a given key using FNV-1a hash
func (sl *StripedLocks) getStripeIndex(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32() % sl.count
}
