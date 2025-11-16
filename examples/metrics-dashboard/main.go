package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/adrianmcphee/smarterbase/v2"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ExampleEntity for demonstration
type ExampleEntity struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Value     int       `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	// Create Prometheus registry
	registry := prometheus.NewRegistry()
	metrics := smarterbase.NewPrometheusMetrics(registry)

	// Create store with observability
	backend := smarterbase.NewFilesystemBackend("/tmp/smarterbase-metrics-demo")
	logger := &smarterbase.StdLogger{}
	store := smarterbase.NewStoreWithObservability(backend, logger, metrics)
	im := smarterbase.NewIndexManager(store)

	ctx := context.Background()

	// Start background workload simulator
	go simulateWorkload(ctx, im)

	// Expose metrics endpoint
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	// Stats endpoint (human-readable)
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Smarterbase Metrics Dashboard\n")
		fmt.Fprintf(w, "==============================\n\n")
		fmt.Fprintf(w, "Metrics endpoint: http://localhost:9090/metrics\n")
		fmt.Fprintf(w, "Prometheus: http://localhost:9091\n")
		fmt.Fprintf(w, "Grafana: http://localhost:3000\n")
	})

	log.Println("Starting metrics server on :9090")
	log.Println("- Metrics: http://localhost:9090/metrics")
	log.Println("- Health: http://localhost:9090/health")
	log.Println("- Stats: http://localhost:9090/stats")
	log.Fatal(http.ListenAndServe(":9090", nil))
}

// simulateWorkload generates realistic traffic patterns
func simulateWorkload(ctx context.Context, im *smarterbase.IndexManager) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	entityID := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Simulate various operations
			op := rand.Intn(100)
			switch {
			case op < 50: // 50% creates
				entity := &ExampleEntity{
					ID:        fmt.Sprintf("entity-%d", entityID),
					Name:      fmt.Sprintf("Entity %d", entityID),
					Value:     rand.Intn(1000),
					CreatedAt: time.Now(),
				}
				key := fmt.Sprintf("entities/%s.json", entity.ID)
				im.Create(ctx, key, entity)
				entityID++

			case op < 80: // 30% reads
				if entityID > 0 {
					readID := rand.Intn(entityID)
					key := fmt.Sprintf("entities/entity-%d.json", readID)
					var entity ExampleEntity
					im.Get(ctx, key, &entity)
				}

			case op < 95: // 15% updates
				if entityID > 0 {
					updateID := rand.Intn(entityID)
					key := fmt.Sprintf("entities/entity-%d.json", updateID)
					var entity ExampleEntity
					if err := im.Get(ctx, key, &entity); err == nil {
						entity.Value = rand.Intn(1000)
						im.Update(ctx, key, &entity)
					}
				}

			default: // 5% deletes
				if entityID > 0 {
					deleteID := rand.Intn(entityID)
					key := fmt.Sprintf("entities/entity-%d.json", deleteID)
					im.Delete(ctx, key)
				}
			}
		}
	}
}
