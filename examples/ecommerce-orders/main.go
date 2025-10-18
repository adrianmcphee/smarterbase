package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/adrianmcphee/smarterbase"
	"github.com/redis/go-redis/v9"
)

// Order represents an e-commerce order
type Order struct {
	ID          string      `json:"id"`
	UserID      string      `json:"user_id"`
	Status      string      `json:"status"` // pending, processing, shipped, delivered, cancelled
	Items       []OrderItem `json:"items"`
	TotalAmount float64     `json:"total_amount"`
	Currency    string      `json:"currency"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

// OrderManager handles all order operations
type OrderManager struct {
	store        *smarterbase.Store
	indexManager *smarterbase.IndexManager
	redisIndexer *smarterbase.RedisIndexer
	lock         *smarterbase.DistributedLock
}

// NewOrderManager creates a new order manager
func NewOrderManager(store *smarterbase.Store, redisClient *redis.Client) *OrderManager {
	// Create Redis indexer
	redisIndexer := smarterbase.NewRedisIndexer(redisClient)

	// Register multi-value indexes
	redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
		Name:        "orders-by-user",
		EntityType:  "orders",
		ExtractFunc: smarterbase.ExtractJSONField("user_id"),
	})

	redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
		Name:        "orders-by-status",
		EntityType:  "orders",
		ExtractFunc: smarterbase.ExtractJSONField("status"),
	})

	// Create index manager
	indexManager := smarterbase.NewIndexManager(store).
		WithRedisIndexer(redisIndexer)

	// Create distributed lock for critical operations
	lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")

	return &OrderManager{
		store:        store,
		indexManager: indexManager,
		redisIndexer: redisIndexer,
		lock:         lock,
	}
}

// CreateOrder creates a new order
func (m *OrderManager) CreateOrder(ctx context.Context, userID string, items []OrderItem) (*Order, error) {
	// Calculate total
	var totalAmount float64
	for _, item := range items {
		totalAmount += item.Price * float64(item.Quantity)
	}

	order := &Order{
		ID:          smarterbase.NewID(),
		UserID:      userID,
		Status:      "pending",
		Items:       items,
		TotalAmount: totalAmount,
		Currency:    "USD",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	key := fmt.Sprintf("orders/%s.json", order.ID)

	// Create with automatic index updates
	err := m.indexManager.Create(ctx, key, order)
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	log.Printf("Created order: %s for user %s (total: $%.2f)", order.ID, order.UserID, order.TotalAmount)
	return order, nil
}

// GetOrder retrieves an order by ID
func (m *OrderManager) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	key := fmt.Sprintf("orders/%s.json", orderID)

	var order Order
	err := m.store.GetJSON(ctx, key, &order)
	if err != nil {
		if smarterbase.IsNotFound(err) {
			return nil, fmt.Errorf("order not found: %s", orderID)
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return &order, nil
}

// ListOrdersByUser returns all orders for a specific user
func (m *OrderManager) ListOrdersByUser(ctx context.Context, userID string) ([]*Order, error) {
	// ✅ NEW (ADR-0006): QueryWithFallback handles Redis → fallback + profiling
	return smarterbase.QueryWithFallback[Order](
		ctx, m.store, m.redisIndexer,
		"orders", "user_id", userID,
		"orders/",
		func(o *Order) bool { return o.UserID == userID },
	)
}

// ListOrdersByStatus returns all orders with a specific status
func (m *OrderManager) ListOrdersByStatus(ctx context.Context, status string) ([]*Order, error) {
	// ✅ NEW (ADR-0006): QueryWithFallback handles Redis → fallback + profiling
	return smarterbase.QueryWithFallback[Order](
		ctx, m.store, m.redisIndexer,
		"orders", "status", status,
		"orders/",
		func(o *Order) bool { return o.Status == status },
	)
}

// UpdateOrderStatus updates the status of an order atomically
// This is a critical operation that requires distributed locking
func (m *OrderManager) UpdateOrderStatus(ctx context.Context, orderID, newStatus string) error {
	key := fmt.Sprintf("orders/%s.json", orderID)

	// Use atomic update with distributed lock for consistency
	err := smarterbase.WithAtomicUpdate(ctx, m.store, m.lock, key, 10*time.Second,
		func(ctx context.Context) error {
			// Get current order
			var order Order
			if err := m.store.GetJSON(ctx, key, &order); err != nil {
				return fmt.Errorf("failed to parse order: %w", err)
			}

			// Validate status transition
			if !isValidStatusTransition(order.Status, newStatus) {
				return fmt.Errorf("invalid status transition: %s -> %s", order.Status, newStatus)
			}

			// Update status
			order.Status = newStatus
			order.UpdatedAt = time.Now()

			// Update with index coordination
			return m.indexManager.Update(ctx, key, &order)
		})

	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	log.Printf("Updated order %s status to: %s", orderID, newStatus)
	return nil
}

// isValidStatusTransition checks if a status transition is valid
func isValidStatusTransition(from, to string) bool {
	validTransitions := map[string][]string{
		"pending":    {"processing", "cancelled"},
		"processing": {"shipped", "cancelled"},
		"shipped":    {"delivered"},
		"delivered":  {},
		"cancelled":  {},
	}

	validNext, ok := validTransitions[from]
	if !ok {
		return false
	}

	for _, valid := range validNext {
		if valid == to {
			return true
		}
	}

	return false
}

// GetOrderStats returns statistics about orders
func (m *OrderManager) GetOrderStats(ctx context.Context) (map[string]int64, error) {
	statuses := []string{"pending", "processing", "shipped", "delivered", "cancelled"}
	stats, err := m.redisIndexer.GetIndexStats(ctx, "orders", "status", statuses)
	if err != nil {
		return nil, fmt.Errorf("failed to get order statistics: %w", err)
	}

	return stats, nil
}

// SearchOrdersByDateRange finds orders within a date range
func (m *OrderManager) SearchOrdersByDateRange(ctx context.Context, startDate, endDate time.Time) ([]*Order, error) {
	var orders []*Order

	err := m.store.Query("orders/").
		FilterJSON(func(obj map[string]interface{}) bool {
			createdAtStr, ok := obj["created_at"].(string)
			if !ok {
				return false
			}

			createdAt, err := time.Parse(time.RFC3339, createdAtStr)
			if err != nil {
				return false
			}

			return createdAt.After(startDate) && createdAt.Before(endDate)
		}).
		SortByField("created_at", false). // Newest first
		All(ctx, &orders)

	if err != nil {
		return nil, fmt.Errorf("failed to search orders: %w", err)
	}

	return orders, nil
}

// CalculateRevenue calculates total revenue for a user
func (m *OrderManager) CalculateRevenue(ctx context.Context, userID string) (float64, error) {
	orders, err := m.ListOrdersByUser(ctx, userID)
	if err != nil {
		return 0, err
	}

	var revenue float64
	for _, order := range orders {
		// Only count delivered orders
		if order.Status == "delivered" {
			revenue += order.TotalAmount
		}
	}

	return revenue, nil
}

func main() {
	ctx := context.Background()

	fmt.Println("\n=== E-Commerce Orders with SmarterBase ===")
	fmt.Println("\n📋 THE CHALLENGE:")
	fmt.Println("E-commerce platforms struggle with:")
	fmt.Println("  • Order data growing to millions/billions of records")
	fmt.Println("  • Complex sharding strategies for horizontal scaling")
	fmt.Println("  • Expensive database licenses at scale")
	fmt.Println("  • Race conditions in status updates without proper locking")
	fmt.Println("\n✨ THE SMARTERBASE SOLUTION:")
	fmt.Println("  ✅ Infinite scale - Handle billions of orders on S3")
	fmt.Println("  ✅ Schema-less - Add fields (notes, shipping_carrier) without migrations")
	fmt.Println("  ✅ Atomic updates - Distributed locks prevent order corruption")
	fmt.Println("  ✅ Fast queries - Redis indexes by user_id and status")
	fmt.Println("  ✅ Zero backups - 11 9s durability built-in")
	fmt.Println()

	// Development setup
	backend := smarterbase.NewFilesystemBackend("./data")
	defer backend.Close()

	store := smarterbase.NewStore(backend)

	// Redis configuration from environment (REDIS_ADDR, REDIS_PASSWORD, REDIS_DB)
	// Defaults to localhost:6379 for local development
	redisClient := redis.NewClient(smarterbase.RedisOptions())
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("Redis connection failed:", err)
	}

	// Create order manager
	orderManager := NewOrderManager(store, redisClient)

	fmt.Println("=== Running Example Operations ===")

	// 1. Create orders
	fmt.Println("1. Creating orders...")
	order1, _ := orderManager.CreateOrder(ctx, "user-123", []OrderItem{
		{ProductID: "prod-1", Quantity: 2, Price: 29.99},
		{ProductID: "prod-2", Quantity: 1, Price: 49.99},
	})
	fmt.Printf("   Order 1: $%.2f\n", order1.TotalAmount)

	order2, _ := orderManager.CreateOrder(ctx, "user-123", []OrderItem{
		{ProductID: "prod-3", Quantity: 1, Price: 99.99},
	})
	fmt.Printf("   Order 2: $%.2f\n", order2.TotalAmount)

	order3, _ := orderManager.CreateOrder(ctx, "user-456", []OrderItem{
		{ProductID: "prod-1", Quantity: 1, Price: 29.99},
	})
	fmt.Printf("   Order 3: $%.2f\n", order3.TotalAmount)

	// 2. Get order by ID
	fmt.Println("\n2. Getting order by ID...")
	order, _ := orderManager.GetOrder(ctx, order1.ID)
	fmt.Printf("   Found order: %s (status: %s, total: $%.2f)\n", order.ID, order.Status, order.TotalAmount)

	// 3. List orders by user (indexed lookup)
	fmt.Println("\n3. Listing orders for user-123...")
	userOrders, _ := orderManager.ListOrdersByUser(ctx, "user-123")
	fmt.Printf("   Found %d orders:\n", len(userOrders))
	for _, o := range userOrders {
		fmt.Printf("   - %s: $%.2f (%s)\n", o.ID, o.TotalAmount, o.Status)
	}

	// 4. Update order status (atomic operation)
	fmt.Println("\n4. Updating order status...")
	orderManager.UpdateOrderStatus(ctx, order1.ID, "processing")
	orderManager.UpdateOrderStatus(ctx, order1.ID, "shipped")
	orderManager.UpdateOrderStatus(ctx, order1.ID, "delivered")
	fmt.Println("   Order 1 status: pending → processing → shipped → delivered")

	// Try invalid transition
	err := orderManager.UpdateOrderStatus(ctx, order1.ID, "cancelled")
	if err != nil {
		fmt.Printf("   Prevented invalid transition: %v\n", err)
	}

	// 5. List orders by status
	fmt.Println("\n5. Listing pending orders...")
	pendingOrders, _ := orderManager.ListOrdersByStatus(ctx, "pending")
	fmt.Printf("   Found %d pending orders\n", len(pendingOrders))

	// 6. Get order statistics
	fmt.Println("\n6. Order statistics:")
	stats, _ := orderManager.GetOrderStats(ctx)
	for status, count := range stats {
		fmt.Printf("   %s: %d orders\n", status, count)
	}

	// 7. Search orders by date range
	fmt.Println("\n7. Searching orders from the last hour...")
	recentOrders, _ := orderManager.SearchOrdersByDateRange(
		ctx,
		time.Now().Add(-1*time.Hour),
		time.Now(),
	)
	fmt.Printf("   Found %d recent orders\n", len(recentOrders))

	// 8. Calculate revenue
	fmt.Println("\n8. Calculating revenue for user-123...")
	revenue, _ := orderManager.CalculateRevenue(ctx, "user-123")
	fmt.Printf("   Total revenue: $%.2f (delivered orders only)\n", revenue)

	fmt.Println("\n=== Example Complete ===")
}
