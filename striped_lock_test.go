package smarterbase

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStripedLocksBasic(t *testing.T) {
	locks := NewStripedLocks(4)

	// Verify creation
	if locks == nil {
		t.Fatal("NewStripedLocks returned nil")
	}
	if locks.count != 4 {
		t.Errorf("stripe count = %d, want 4", locks.count)
	}
}

func TestStripedLocksDefaultCount(t *testing.T) {
	locks := NewStripedLocks(0)
	if locks.count != 32 {
		t.Errorf("default stripe count = %d, want 32", locks.count)
	}

	locks2 := NewStripedLocks(-1)
	if locks2.count != 32 {
		t.Errorf("default stripe count = %d, want 32", locks2.count)
	}
}

func TestStripedLocksExclusiveLock(t *testing.T) {
	locks := NewStripedLocks(32)
	key := "test-key"

	// Acquire lock
	unlock := locks.Lock(key)
	if unlock == nil {
		t.Fatal("Lock returned nil unlock function")
	}

	// Release lock
	unlock()
}

func TestStripedLocksReadLock(t *testing.T) {
	locks := NewStripedLocks(32)
	key := "test-key"

	// Acquire read lock
	unlock := locks.RLock(key)
	if unlock == nil {
		t.Fatal("RLock returned nil unlock function")
	}

	// Release lock
	unlock()
}

func TestStripedLocksConcurrentReads(t *testing.T) {
	locks := NewStripedLocks(32)
	key := "shared-key"
	var wg sync.WaitGroup
	readCount := int32(0)

	// Multiple readers should be able to acquire lock simultaneously
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := locks.RLock(key)
			defer unlock()

			atomic.AddInt32(&readCount, 1)
			time.Sleep(10 * time.Millisecond)
		}()
	}

	// Give goroutines time to start
	time.Sleep(5 * time.Millisecond)

	// All readers should have acquired the lock
	if atomic.LoadInt32(&readCount) < 5 {
		t.Error("readers should be able to acquire lock concurrently")
	}

	wg.Wait()
	if atomic.LoadInt32(&readCount) != 10 {
		t.Errorf("readCount = %d, want 10", readCount)
	}
}

func TestStripedLocksExclusiveBlocking(t *testing.T) {
	locks := NewStripedLocks(32)
	key := "exclusive-key"
	counter := int32(0)

	// First goroutine acquires lock
	unlock := locks.Lock(key)

	done := make(chan bool)
	go func() {
		// Second goroutine tries to acquire same lock (should block)
		unlock2 := locks.Lock(key)
		atomic.AddInt32(&counter, 1)
		unlock2()
		done <- true
	}()

	// Verify second goroutine is blocked
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&counter) != 0 {
		t.Error("second lock should be blocked")
	}

	// Release first lock
	unlock()

	// Second lock should now succeed
	<-done
	if atomic.LoadInt32(&counter) != 1 {
		t.Errorf("counter = %d, want 1", atomic.LoadInt32(&counter))
	}
}

func TestStripedLocksStripeDistribution(t *testing.T) {
	locks := NewStripedLocks(4)

	// Test that same key always maps to same stripe
	key := "consistent-key"
	idx1 := locks.getStripeIndex(key)
	idx2 := locks.getStripeIndex(key)
	idx3 := locks.getStripeIndex(key)

	if idx1 != idx2 || idx1 != idx3 {
		t.Errorf("same key should map to same stripe: %d, %d, %d", idx1, idx2, idx3)
	}

	// Test that indices are in valid range
	if idx1 >= locks.count {
		t.Errorf("stripe index %d out of range [0, %d)", idx1, locks.count)
	}
}

func TestStripedLocksMultipleKeys(t *testing.T) {
	locks := NewStripedLocks(32)
	var wg sync.WaitGroup
	counter := int32(0)

	// Different keys should not block each other (most of the time)
	keys := []string{"key1", "key2", "key3", "key4", "key5"}
	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			unlock := locks.Lock(k)
			defer unlock()

			atomic.AddInt32(&counter, 1)
			time.Sleep(10 * time.Millisecond)
		}(key)
	}

	wg.Wait()
	if atomic.LoadInt32(&counter) != 5 {
		t.Errorf("counter = %d, want 5", counter)
	}
}

func TestStripedLocksHashDistribution(t *testing.T) {
	locks := NewStripedLocks(8)

	// Generate many keys and verify they distribute across stripes
	stripeUsage := make(map[uint32]int)
	for i := 0; i < 1000; i++ {
		key := string(rune(i))
		idx := locks.getStripeIndex(key)
		stripeUsage[idx]++
	}

	// Verify all stripes are used (with high probability)
	if len(stripeUsage) < 6 {
		t.Errorf("only %d/8 stripes used, distribution may be poor", len(stripeUsage))
	}

	// Verify reasonable distribution (no stripe should have >50% of keys)
	for idx, count := range stripeUsage {
		if count > 500 {
			t.Errorf("stripe %d has %d keys (>50%%), distribution is skewed", idx, count)
		}
	}
}

func TestStripedLocksDefer(t *testing.T) {
	locks := NewStripedLocks(32)
	counter := int32(0)

	func() {
		unlock := locks.Lock("test")
		defer unlock()

		atomic.AddInt32(&counter, 1)
	}()

	// Lock should be released after function returns
	if atomic.LoadInt32(&counter) != 1 {
		t.Errorf("counter = %d, want 1", counter)
	}

	// Should be able to acquire lock again
	unlock := locks.Lock("test")
	unlock()
}

func BenchmarkStripedLockExclusive(b *testing.B) {
	locks := NewStripedLocks(32)
	key := "bench-key"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unlock := locks.Lock(key)
		unlock()
	}
}

func BenchmarkStripedLockRead(b *testing.B) {
	locks := NewStripedLocks(32)
	key := "bench-key"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unlock := locks.RLock(key)
		unlock()
	}
}

func BenchmarkStripedLockConcurrentReads(b *testing.B) {
	locks := NewStripedLocks(32)
	key := "bench-key"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			unlock := locks.RLock(key)
			unlock()
		}
	})
}

func BenchmarkStripedLockDifferentKeys(b *testing.B) {
	locks := NewStripedLocks(32)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune(i % 100))
			unlock := locks.Lock(key)
			unlock()
			i++
		}
	})
}
