package smarterbase

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// BenchmarkEntity is a simple entity for benchmarking
type BenchmarkEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Count       int    `json:"count"`
}

// Benchmark IndexManager Create operations
func BenchmarkIndexManager_Create(b *testing.B) {
	tmpDir := b.TempDir()
	backend := NewFilesystemBackend(tmpDir)
	store := NewStoreWithObservability(backend, &NoOpLogger{}, NewInMemoryMetrics())
	im := NewIndexManager(store)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := &BenchmarkEntity{
			ID:          fmt.Sprintf("entity-%d", i),
			Name:        fmt.Sprintf("Entity %d", i),
			Description: "Benchmark entity for testing",
			Count:       i,
		}
		key := fmt.Sprintf("entities/%s.json", entity.ID)
		if err := im.Create(ctx, key, entity); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark IndexManager Get operations
func BenchmarkIndexManager_Get(b *testing.B) {
	tmpDir := b.TempDir()
	backend := NewFilesystemBackend(tmpDir)
	store := NewStoreWithObservability(backend, &NoOpLogger{}, NewInMemoryMetrics())
	im := NewIndexManager(store)
	ctx := context.Background()

	// Seed data
	numEntities := 1000
	for i := 0; i < numEntities; i++ {
		entity := &BenchmarkEntity{
			ID:          fmt.Sprintf("entity-%d", i),
			Name:        fmt.Sprintf("Entity %d", i),
			Description: "Benchmark entity for testing",
			Count:       i,
		}
		key := fmt.Sprintf("entities/%s.json", entity.ID)
		if err := im.Create(ctx, key, entity); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entityID := fmt.Sprintf("entity-%d", i%numEntities)
		key := fmt.Sprintf("entities/%s.json", entityID)
		var entity BenchmarkEntity
		if err := im.Get(ctx, key, &entity); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark IndexManager Update operations
func BenchmarkIndexManager_Update(b *testing.B) {
	tmpDir := b.TempDir()
	backend := NewFilesystemBackend(tmpDir)
	store := NewStoreWithObservability(backend, &NoOpLogger{}, NewInMemoryMetrics())
	im := NewIndexManager(store)
	ctx := context.Background()

	// Seed initial entity
	entity := &BenchmarkEntity{
		ID:          "entity-1",
		Name:        "Entity 1",
		Description: "Benchmark entity for testing",
		Count:       0,
	}
	key := "entities/entity-1.json"
	if err := im.Create(ctx, key, entity); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity.Count = i
		entity.Name = fmt.Sprintf("Entity %d", i)
		if err := im.Update(ctx, key, entity); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark IndexManager Delete operations
func BenchmarkIndexManager_Delete(b *testing.B) {
	tmpDir := b.TempDir()
	backend := NewFilesystemBackend(tmpDir)
	store := NewStoreWithObservability(backend, &NoOpLogger{}, NewInMemoryMetrics())
	im := NewIndexManager(store)
	ctx := context.Background()

	// Seed data
	for i := 0; i < b.N; i++ {
		entity := &BenchmarkEntity{
			ID:          fmt.Sprintf("entity-%d", i),
			Name:        fmt.Sprintf("Entity %d", i),
			Description: "Benchmark entity for testing",
			Count:       i,
		}
		key := fmt.Sprintf("entities/%s.json", entity.ID)
		if err := im.Create(ctx, key, entity); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("entities/entity-%d.json", i)
		if err := im.Delete(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark Query operations
func BenchmarkQuery_FilterAndSort(b *testing.B) {
	tmpDir := b.TempDir()
	backend := NewFilesystemBackend(tmpDir)
	store := NewStore(backend)
	ctx := context.Background()

	// Seed data
	numEntities := 1000
	for i := 0; i < numEntities; i++ {
		entity := &BenchmarkEntity{
			ID:          fmt.Sprintf("entity-%d", i),
			Name:        fmt.Sprintf("Entity %d", i),
			Description: "Benchmark entity for testing",
			Count:       i,
		}
		key := fmt.Sprintf("entities/%s.json", entity.ID)
		if err := PutJSON(backend, ctx, key, entity); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var results []*BenchmarkEntity
		err := store.Query("entities/").
			FilterJSON(func(obj map[string]interface{}) bool {
				count, ok := obj["count"].(float64)
				return ok && int(count) > 500
			}).
			SortByField("count", false). // Descending
			Limit(10).
			All(ctx, &results)
		if err != nil {
			b.Fatal(err)
		}
		if len(results) != 10 {
			b.Fatalf("expected 10 results, got %d", len(results))
		}
	}
}

// Benchmark with and without observability
func BenchmarkObservability_Overhead(b *testing.B) {
	tmpDir := b.TempDir()
	backend := NewFilesystemBackend(tmpDir)

	b.Run("NoObservability", func(b *testing.B) {
		store := NewStore(backend)
		im := NewIndexManager(store)
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			entity := &BenchmarkEntity{
				ID:          fmt.Sprintf("entity-%d", i),
				Name:        fmt.Sprintf("Entity %d", i),
				Description: "Benchmark entity for testing",
				Count:       i,
			}
			key := fmt.Sprintf("entities/%s.json", entity.ID)
			if err := im.Create(ctx, key, entity); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Clean up for next run
	os.RemoveAll(tmpDir)
	os.MkdirAll(tmpDir, 0755)
	backend = NewFilesystemBackend(tmpDir)

	b.Run("WithObservability", func(b *testing.B) {
		store := NewStoreWithObservability(backend, &StdLogger{}, NewInMemoryMetrics())
		im := NewIndexManager(store)
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			entity := &BenchmarkEntity{
				ID:          fmt.Sprintf("entity-%d", i),
				Name:        fmt.Sprintf("Entity %d", i),
				Description: "Benchmark entity for testing",
				Count:       i,
			}
			key := fmt.Sprintf("entities/%s.json", entity.ID)
			if err := im.Create(ctx, key, entity); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Benchmark List operations with different sizes
func BenchmarkList_ScaleTest(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			tmpDir := b.TempDir()
			backend := NewFilesystemBackend(tmpDir)
			ctx := context.Background()

			// Seed data
			for i := 0; i < size; i++ {
				entity := &BenchmarkEntity{
					ID:          fmt.Sprintf("entity-%d", i),
					Name:        fmt.Sprintf("Entity %d", i),
					Description: "Benchmark entity for testing",
					Count:       i,
				}
				key := fmt.Sprintf("entities/%s.json", entity.ID)
				if err := PutJSON(backend, ctx, key, entity); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				keys, err := backend.List(ctx, "entities/")
				if err != nil {
					b.Fatal(err)
				}
				if len(keys) != size {
					b.Fatalf("expected %d keys, got %d", size, len(keys))
				}
			}
		})
	}
}

// Benchmark concurrent operations
func BenchmarkConcurrent_Creates(b *testing.B) {
	tmpDir := b.TempDir()
	backend := NewFilesystemBackend(tmpDir)
	store := NewStoreWithObservability(backend, &NoOpLogger{}, NewInMemoryMetrics())
	im := NewIndexManager(store)
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			entity := &BenchmarkEntity{
				ID:          fmt.Sprintf("entity-%d", i),
				Name:        fmt.Sprintf("Entity %d", i),
				Description: "Benchmark entity for testing",
				Count:       i,
			}
			key := fmt.Sprintf("entities/%s.json", entity.ID)
			if err := im.Create(ctx, key, entity); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

// Helper benchmark to show byte allocation
func BenchmarkJSON_Marshaling(b *testing.B) {
	entity := &BenchmarkEntity{
		ID:          "entity-1",
		Name:        "Entity 1",
		Description: "Benchmark entity for testing with a longer description to simulate real data",
		Count:       42,
	}

	b.Run("PutJSON", func(b *testing.B) {
		tmpDir := b.TempDir()
		backend := NewFilesystemBackend(tmpDir)
		ctx := context.Background()
		key := "entities/entity-1.json"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := PutJSON(backend, ctx, key, entity); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GetJSON", func(b *testing.B) {
		tmpDir := b.TempDir()
		backend := NewFilesystemBackend(tmpDir)
		ctx := context.Background()
		key := "entities/entity-1.json"

		// Seed data
		if err := PutJSON(backend, ctx, key, entity); err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var result BenchmarkEntity
			if err := GetJSON(backend, ctx, key, &result); err != nil {
				b.Fatal(err)
			}
		}
	})
}
