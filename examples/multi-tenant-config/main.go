package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/adrianmcphee/smarterbase"
	"github.com/redis/go-redis/v9"
)

// TenantConfig represents configuration for a tenant
type TenantConfig struct {
	TenantID  string                 `json:"tenant_id"`
	Name      string                 `json:"name"`
	Plan      string                 `json:"plan"` // free, pro, enterprise
	Settings  map[string]interface{} `json:"settings"`
	Features  []string               `json:"features"`
	Limits    ResourceLimits         `json:"limits"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// ResourceLimits defines resource constraints for a tenant
type ResourceLimits struct {
	MaxUsers      int `json:"max_users"`
	MaxStorage    int `json:"max_storage_mb"`
	MaxAPIRequest int `json:"max_api_requests_per_hour"`
}

// ConfigManager handles tenant configuration operations
type ConfigManager struct {
	store        *smarterbase.Store
	indexManager *smarterbase.IndexManager
	redisIndexer *smarterbase.RedisIndexer
	lock         *smarterbase.DistributedLock
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(store *smarterbase.Store, redisClient *redis.Client) *ConfigManager {
	// Create Redis indexer
	redisIndexer := smarterbase.NewRedisIndexer(redisClient)

	// Register indexes
	redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
		Name:        "tenants-by-plan",
		EntityType:  "tenants",
		ExtractFunc: smarterbase.ExtractJSONField("plan"),
	})

	indexManager := smarterbase.NewIndexManager(store).
		WithRedisIndexer(redisIndexer)

	lock := smarterbase.NewDistributedLock(redisClient, "smarterbase")

	return &ConfigManager{
		store:        store,
		indexManager: indexManager,
		redisIndexer: redisIndexer,
		lock:         lock,
	}
}

// CreateTenant creates a new tenant with default configuration
func (m *ConfigManager) CreateTenant(ctx context.Context, tenantID, name, plan string) (*TenantConfig, error) {
	// Get plan limits
	limits := m.getPlanLimits(plan)

	// Get plan features
	features := m.getPlanFeatures(plan)

	config := &TenantConfig{
		TenantID: tenantID,
		Name:     name,
		Plan:     plan,
		Settings: map[string]interface{}{
			"timezone":      "UTC",
			"date_format":   "YYYY-MM-DD",
			"language":      "en",
			"notifications": true,
		},
		Features:  features,
		Limits:    limits,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	key := fmt.Sprintf("tenants/%s/config.json", tenantID)

	err := m.indexManager.Create(ctx, key, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant config: %w", err)
	}

	log.Printf("Created tenant config: %s (%s plan)", tenantID, plan)
	return config, nil
}

// GetTenantConfig retrieves configuration for a tenant
func (m *ConfigManager) GetTenantConfig(ctx context.Context, tenantID string) (*TenantConfig, error) {
	key := fmt.Sprintf("tenants/%s/config.json", tenantID)

	var config TenantConfig
	err := m.store.GetJSON(ctx, key, &config)
	if err != nil {
		if smarterbase.IsNotFound(err) {
			return nil, fmt.Errorf("tenant not found: %s", tenantID)
		}
		return nil, fmt.Errorf("failed to get tenant config: %w", err)
	}

	return &config, nil
}

// UpdateTenantSettings updates specific settings atomically
func (m *ConfigManager) UpdateTenantSettings(ctx context.Context, tenantID string, settings map[string]interface{}) error {
	key := fmt.Sprintf("tenants/%s/config.json", tenantID)

	// Use atomic update to prevent race conditions
	return smarterbase.WithAtomicUpdate(ctx, m.store, m.lock, key, 10*time.Second,
		func(ctx context.Context) error {
			var config TenantConfig
			if err := m.store.GetJSON(ctx, key, &config); err != nil {
				return fmt.Errorf("failed to get config: %w", err)
			}

			// Merge settings
			for k, v := range settings {
				config.Settings[k] = v
			}

			config.UpdatedAt = time.Now()

			return m.store.PutJSON(ctx, key, &config)
		})
}

// UpgradeTenantPlan upgrades a tenant to a new plan
func (m *ConfigManager) UpgradeTenantPlan(ctx context.Context, tenantID, newPlan string) error {
	key := fmt.Sprintf("tenants/%s/config.json", tenantID)

	// Use atomic update for consistency
	return smarterbase.WithAtomicUpdate(ctx, m.store, m.lock, key, 10*time.Second,
		func(ctx context.Context) error {
			var config TenantConfig
			if err := m.store.GetJSON(ctx, key, &config); err != nil {
				return err
			}

			// Validate upgrade path
			if !isValidUpgrade(config.Plan, newPlan) {
				return fmt.Errorf("invalid upgrade: %s -> %s", config.Plan, newPlan)
			}

			// Update plan, limits, and features
			config.Plan = newPlan
			config.Limits = m.getPlanLimits(newPlan)
			config.Features = m.getPlanFeatures(newPlan)
			config.UpdatedAt = time.Now()

			// Update with index coordination
			return m.indexManager.Update(ctx, key, &config)
		})
}

// ListTenantsByPlan returns all tenants on a specific plan
func (m *ConfigManager) ListTenantsByPlan(ctx context.Context, plan string) ([]*TenantConfig, error) {
	// ✅ NEW (ADR-0006): QueryWithFallback handles Redis → fallback + profiling
	return smarterbase.QueryWithFallback[TenantConfig](
		ctx, m.store, m.redisIndexer,
		"tenants", "plan", plan,
		"tenants/",
		func(t *TenantConfig) bool { return t.Plan == plan },
	)
}

// GetPlanStats returns statistics about tenant plans
func (m *ConfigManager) GetPlanStats(ctx context.Context) (map[string]int64, error) {
	plans := []string{"free", "pro", "enterprise"}
	return m.redisIndexer.GetIndexStats(ctx, "tenants", "plan", plans)
}

// Helper functions

func (m *ConfigManager) getPlanLimits(plan string) ResourceLimits {
	limits := map[string]ResourceLimits{
		"free": {
			MaxUsers:      5,
			MaxStorage:    1024, // 1GB
			MaxAPIRequest: 1000,
		},
		"pro": {
			MaxUsers:      50,
			MaxStorage:    10240, // 10GB
			MaxAPIRequest: 10000,
		},
		"enterprise": {
			MaxUsers:      -1, // unlimited
			MaxStorage:    -1, // unlimited
			MaxAPIRequest: -1, // unlimited
		},
	}

	return limits[plan]
}

func (m *ConfigManager) getPlanFeatures(plan string) []string {
	features := map[string][]string{
		"free": {
			"basic_dashboard",
			"email_support",
		},
		"pro": {
			"basic_dashboard",
			"advanced_analytics",
			"priority_support",
			"api_access",
			"custom_branding",
		},
		"enterprise": {
			"basic_dashboard",
			"advanced_analytics",
			"priority_support",
			"api_access",
			"custom_branding",
			"sso",
			"audit_logs",
			"dedicated_support",
			"sla_guarantee",
		},
	}

	return features[plan]
}

func isValidUpgrade(from, to string) bool {
	upgradePaths := map[string][]string{
		"free":       {"pro", "enterprise"},
		"pro":        {"enterprise"},
		"enterprise": {},
	}

	validPaths, ok := upgradePaths[from]
	if !ok {
		return false
	}

	for _, valid := range validPaths {
		if valid == to {
			return true
		}
	}

	return false
}

func main() {
	ctx := context.Background()

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

	// Create config manager
	configManager := NewConfigManager(store, redisClient)

	fmt.Println("\n=== Multi-Tenant SaaS with SmarterBase ===")
	fmt.Println("\n📋 THE CHALLENGE:")
	fmt.Println("Multi-tenant SaaS platforms need:")
	fmt.Println("  • Isolated config per tenant (millions of tenants)")
	fmt.Println("  • Fast plan upgrades without downtime")
	fmt.Println("  • Feature flags that change without migrations")
	fmt.Println("  • Cost-effective storage that scales with growth")
	fmt.Println("\n✨ THE SMARTERBASE SOLUTION:")
	fmt.Println("  ✅ Schema-less - Add features without migrations")
	fmt.Println("  ✅ Infinite scale - Millions of tenants on S3")
	fmt.Println("  ✅ Atomic upgrades - Distributed locks prevent corruption")
	fmt.Println("  ✅ Fast queries - Redis indexes by plan type")
	fmt.Println("  ✅ 85% cost savings - vs. traditional databases")
	fmt.Println()

	fmt.Println("=== Running Example Operations ===")

	// 1. Create tenants
	fmt.Println("1. Creating tenants...")
	acme, _ := configManager.CreateTenant(ctx, "acme-corp", "Acme Corporation", "enterprise")
	fmt.Printf("   Created: %s (%s plan)\n", acme.Name, acme.Plan)

	techstart, _ := configManager.CreateTenant(ctx, "techstart", "TechStart Inc", "pro")
	fmt.Printf("   Created: %s (%s plan)\n", techstart.Name, techstart.Plan)

	smallbiz, _ := configManager.CreateTenant(ctx, "smallbiz", "Small Business LLC", "free")
	fmt.Printf("   Created: %s (%s plan)\n", smallbiz.Name, smallbiz.Plan)

	// 2. Get tenant configuration
	fmt.Println("\n2. Getting tenant configuration...")
	config, _ := configManager.GetTenantConfig(ctx, "acme-corp")
	fmt.Printf("   Tenant: %s\n", config.Name)
	fmt.Printf("   Plan: %s\n", config.Plan)
	fmt.Printf("   Features: %v\n", config.Features)
	fmt.Printf("   Max Users: %d\n", config.Limits.MaxUsers)
	fmt.Printf("   Max Storage: %d MB\n", config.Limits.MaxStorage)

	// 3. Update tenant settings
	fmt.Println("\n3. Updating tenant settings...")
	configManager.UpdateTenantSettings(ctx, "techstart", map[string]interface{}{
		"timezone":      "America/New_York",
		"notifications": false,
		"custom_domain": "techstart.example.com",
	})
	fmt.Println("   Updated TechStart settings")

	// Verify update
	updated, _ := configManager.GetTenantConfig(ctx, "techstart")
	fmt.Printf("   New timezone: %v\n", updated.Settings["timezone"])
	fmt.Printf("   Custom domain: %v\n", updated.Settings["custom_domain"])

	// 4. Upgrade tenant plan
	fmt.Println("\n4. Upgrading tenant plan...")
	err := configManager.UpgradeTenantPlan(ctx, "smallbiz", "pro")
	if err != nil {
		log.Printf("   Error: %v", err)
	} else {
		fmt.Println("   Upgraded Small Business to Pro plan")
	}

	// Verify upgrade
	upgraded, _ := configManager.GetTenantConfig(ctx, "smallbiz")
	fmt.Printf("   New plan: %s\n", upgraded.Plan)
	fmt.Printf("   New features: %v\n", upgraded.Features)
	fmt.Printf("   New max users: %d\n", upgraded.Limits.MaxUsers)

	// 5. List tenants by plan
	fmt.Println("\n5. Listing tenants by plan...")
	proTenants, _ := configManager.ListTenantsByPlan(ctx, "pro")
	fmt.Printf("   Pro plan tenants (%d):\n", len(proTenants))
	for _, t := range proTenants {
		fmt.Printf("   - %s\n", t.Name)
	}

	// 6. Get plan statistics
	fmt.Println("\n6. Plan statistics:")
	stats, _ := configManager.GetPlanStats(ctx)
	for plan, count := range stats {
		fmt.Printf("   %s: %d tenants\n", plan, count)
	}

	// 7. Try invalid upgrade
	fmt.Println("\n7. Testing invalid upgrade...")
	err = configManager.UpgradeTenantPlan(ctx, "acme-corp", "free")
	if err != nil {
		fmt.Printf("   Prevented invalid downgrade: %v\n", err)
	}

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\nKey benefits of this approach:")
	fmt.Println("- Per-tenant isolation (each tenant has own config file)")
	fmt.Println("- Fast plan-based queries with Redis indexes")
	fmt.Println("- Atomic updates prevent race conditions")
	fmt.Println("- No schema migrations needed (JSON flexibility)")
	fmt.Println("- Scales to millions of tenants on S3")
}
