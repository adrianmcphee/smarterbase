package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/adrianmcphee/smarterbase"
)

// Version 0: Original schema (no version field)
type ProductV0 struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
}

// Version 1: Added inventory tracking
type ProductV1 struct {
	V           int     `json:"_v"`
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`      // New field
	SKU         string  `json:"sku"`        // New field
	CreatedAt   string  `json:"created_at"` // New field
}

// Version 2: Split name into brand and product name
type ProductV2 struct {
	V           int     `json:"_v"`
	ID          string  `json:"id"`
	Brand       string  `json:"brand"`        // Split from name
	ProductName string  `json:"product_name"` // Split from name
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	SKU         string  `json:"sku"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"` // New field
}

// Version 3: Added pricing tiers and categories
type ProductV3 struct {
	V           int                `json:"_v"`
	ID          string             `json:"id"`
	Brand       string             `json:"brand"`
	ProductName string             `json:"product_name"`
	Description string             `json:"description"`
	Pricing     map[string]float64 `json:"pricing"` // Changed from single price
	Stock       int                `json:"stock"`
	SKU         string             `json:"sku"`
	Categories  []string           `json:"categories"` // New field
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
}

func main() {
	ctx := context.Background()

	fmt.Println("\n=== Schema Migrations with SmarterBase ===")
	fmt.Println("\n📋 THE CHALLENGE:")
	fmt.Println("Traditional databases require:")
	fmt.Println("  • Complex ALTER TABLE statements")
	fmt.Println("  • Downtime for schema migrations")
	fmt.Println("  • Backfill scripts for existing data")
	fmt.Println("  • Version management and rollback strategies")
	fmt.Println("\n✨ THE SMARTERBASE SOLUTION:")
	fmt.Println("  ✅ No downtime - migrations happen on read")
	fmt.Println("  ✅ Schema-less storage - JSON adapts naturally")
	fmt.Println("  ✅ Version tracking - explicit _v field")
	fmt.Println("  ✅ Migration registry - centralized transformation logic")
	fmt.Println("  ✅ Lazy migration - only migrates data when accessed")
	fmt.Println()

	// Setup
	backend := smarterbase.NewFilesystemBackend("./data")
	defer backend.Close()

	store := smarterbase.NewStore(backend)

	// Register migrations at app startup
	registerMigrations()

	fmt.Println("=== Demonstrating Schema Evolution ===")

	// 1. Write Version 0 data (original schema)
	fmt.Println("1. Writing Version 0 products (original schema)...")
	oldProducts := []ProductV0{
		{ID: "p1", Name: "Apple MacBook Pro", Description: "High-performance laptop", Price: 2499.99},
		{ID: "p2", Name: "Samsung Galaxy S23", Description: "Flagship smartphone", Price: 999.99},
		{ID: "p3", Name: "Sony WH-1000XM5", Description: "Noise-canceling headphones", Price: 399.99},
	}

	for _, product := range oldProducts {
		key := fmt.Sprintf("products/%s.json", product.ID)
		if err := store.PutJSON(ctx, key, product); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("   Wrote %d products with Version 0 schema (no version field)\n", len(oldProducts))

	// 2. Read as Version 3 (automatic migration)
	fmt.Println("\n2. Reading products as Version 3 (with automatic migration)...")
	var product3 ProductV3
	product3.V = 3 // Set expected version

	key := "products/p1.json"
	if err := store.GetJSON(ctx, key, &product3); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("   Migrated product: %s %s\n", product3.Brand, product3.ProductName)
	fmt.Printf("   Price tiers: retail=$%.2f\n", product3.Pricing["retail"])
	fmt.Printf("   Stock: %d units\n", product3.Stock)
	fmt.Printf("   SKU: %s\n", product3.SKU)

	// 3. Write new Version 3 products
	fmt.Println("\n3. Writing new products with Version 3 schema...")
	newProduct := ProductV3{
		V:           3,
		ID:          "p4",
		Brand:       "Dell",
		ProductName: "XPS 13",
		Description: "Ultraportable laptop",
		Pricing: map[string]float64{
			"retail":    1299.99,
			"wholesale": 1099.99,
			"student":   1199.99,
		},
		Stock:      150,
		SKU:        "DELL-XPS13-2024",
		Categories: []string{"laptops", "ultraportable", "business"},
		CreatedAt:  time.Now().Format(time.RFC3339),
		UpdatedAt:  time.Now().Format(time.RFC3339),
	}

	if err := store.PutJSON(ctx, "products/p4.json", newProduct); err != nil {
		log.Fatal(err)
	}
	fmt.Println("   Wrote new product with full Version 3 schema")

	// 4. Demonstrate migration chain
	fmt.Println("\n4. Reading all products (showing migration path)...")
	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		var p ProductV3
		p.V = 3

		key := fmt.Sprintf("products/%s.json", id)
		if err := store.GetJSON(ctx, key, &p); err != nil {
			log.Printf("   Error reading %s: %v", id, err)
			continue
		}

		fmt.Printf("   %s: %s %s ($%.2f)\n",
			p.ID, p.Brand, p.ProductName, p.Pricing["retail"])
	}

	// 5. Demonstrate MigrateAndWrite policy
	fmt.Println("\n5. Enabling MigrateAndWrite policy...")
	store.WithMigrationPolicy(smarterbase.MigrateAndWrite)

	fmt.Println("   Reading p2 (will migrate AND write back)...")
	var p2 ProductV3
	p2.V = 3
	if err := store.GetJSON(ctx, "products/p2.json", &p2); err != nil {
		log.Fatal(err)
	}

	// Verify it was written back
	data, _ := backend.Get(ctx, "products/p2.json")
	fmt.Printf("   Product p2 now stored as Version 3 in S3: %d bytes\n", len(data))

	fmt.Println("\n=== Migration Complete ===")
	fmt.Println("\nKey Takeaways:")
	fmt.Println("  • Old data (v0) still readable, migrates automatically to v3")
	fmt.Println("  • New data written directly as v3")
	fmt.Println("  • No downtime, no ALTER TABLE statements")
	fmt.Println("  • Migrations happen lazily on read")
	fmt.Println("  • Optional write-back for gradual data upgrade")
}

// registerMigrations sets up all schema migrations
func registerMigrations() {
	// Migration 0 → 1: Add inventory tracking
	smarterbase.Migrate("ProductV3").From(0).To(1).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
		data["stock"] = 0
		data["sku"] = fmt.Sprintf("SKU-%s", data["id"])
		data["created_at"] = time.Now().Format(time.RFC3339)
		data["_v"] = 1
		return data, nil
	})

	// Migration 1 → 2: Split name into brand and product name
	smarterbase.Migrate("ProductV3").From(1).To(2).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
		if name, ok := data["name"].(string); ok {
			parts := strings.SplitN(name, " ", 2)
			data["brand"] = parts[0]
			if len(parts) > 1 {
				data["product_name"] = parts[1]
			} else {
				data["product_name"] = name
			}
			delete(data, "name")
		}
		data["updated_at"] = time.Now().Format(time.RFC3339)
		data["_v"] = 2
		return data, nil
	})

	// Migration 2 → 3: Convert price to pricing tiers, add categories
	smarterbase.Migrate("ProductV3").From(2).To(3).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
		// Convert single price to pricing tiers
		if price, ok := data["price"].(float64); ok {
			data["pricing"] = map[string]interface{}{
				"retail":    price,
				"wholesale": price * 0.85,
				"student":   price * 0.90,
			}
			delete(data, "price")
		}

		// Add default categories based on price
		pricing, _ := data["pricing"].(map[string]interface{})
		retail, _ := pricing["retail"].(float64)

		categories := []string{}
		if retail < 100 {
			categories = append(categories, "budget")
		} else if retail < 1000 {
			categories = append(categories, "mid-range")
		} else {
			categories = append(categories, "premium")
		}

		data["categories"] = categories
		data["_v"] = 3
		return data, nil
	})

	fmt.Println("✓ Registered migration chain: V0 → V1 → V2 → V3")
}
