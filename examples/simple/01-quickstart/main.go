// Quickstart: Track your coffee consumption with automatic indexing.
// Run it multiple times with different coffees - it remembers everything!
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/adrianmcphee/smarterbase/v2/simple"
)

type Coffee struct {
	ID      string    `json:"id" sb:"id"`
	Type    string    `json:"type" sb:"index"` // Automatically indexed!
	Size    string    `json:"size" sb:"index"` // Query by size
	DrankAt time.Time `json:"drank_at"`
}

func main() {
	ctx := context.Background()
	db := simple.MustConnect()
	defer db.Close()

	coffees := simple.NewCollection[Coffee](db)

	// Record today's coffee
	coffees.Create(ctx, &Coffee{
		Type:    "Espresso",
		Size:    "Double",
		DrankAt: time.Now(),
	})

	// Query: How many espressos have I had?
	espressos, _ := coffees.Find(ctx, "type", "Espresso")

	// Query: How many doubles?
	doubles, _ := coffees.Find(ctx, "size", "Double")

	// Total coffee count
	total, _ := coffees.Count(ctx)

	fmt.Printf("☕ Coffee #%d logged!\n", total)
	fmt.Printf("   Total Espressos: %d\n", len(espressos))
	fmt.Printf("   Total Doubles: %d\n\n", len(doubles))
	fmt.Printf("💡 The indexes were created automatically from struct tags.\n")
	fmt.Printf("   Run me again with different coffees!\n")
}
