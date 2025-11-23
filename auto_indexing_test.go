package smarterbase

import (
	"context"
	"testing"
)

// Test domain models (Redis-only indexes)
type AutoIndexTestUser struct {
	ID             string `json:"id"`
	Email          string `json:"email" sb:"index"`
	PlatformUserID string `json:"platform_user_id" sb:"index"`
	ReferralCode   string `json:"referral_code,omitempty" sb:"index,optional"`
	Username       string `json:"username" sb:"index,name:custom-username-index"`
}

type AutoIndexTestSession struct {
	ID     string `json:"id"`
	Token  string `json:"token" sb:"index"`
	UserID string `json:"user_id" sb:"index"`
}

type AutoIndexTestProduct struct {
	ID       string `json:"id"`
	SKU      string `json:"sku" sb:"index"`
	Category string `json:"category" sb:"index"`
}

func TestParseIndexTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		wantType string
		wantOk   bool
		optional bool
	}{
		{
			name:     "multi with colon",
			tag:      "index:multi",
			wantType: "multi",
			wantOk:   true,
		},
		{
			name:     "multi with comma",
			tag:      "index,multi",
			wantType: "multi",
			wantOk:   true,
		},
		{
			name:     "just index defaults to multi",
			tag:      "index",
			wantType: "multi",
			wantOk:   true,
		},
		{
			name:     "optional multi",
			tag:      "index,optional",
			wantType: "multi",
			wantOk:   true,
			optional: true,
		},
		{
			name:   "unique tag is rejected",
			tag:    "index,unique",
			wantOk: false,
		},
		{
			name:   "no index tag",
			tag:    "json:email",
			wantOk: false,
		},
		{
			name:   "empty tag",
			tag:    "",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag, ok := ParseIndexTag(tt.tag)
			if ok != tt.wantOk {
				t.Errorf("ParseIndexTag() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !ok {
				return
			}
			if tag.Type != tt.wantType {
				t.Errorf("ParseIndexTag() type = %v, want %v", tag.Type, tt.wantType)
			}
			if tag.Optional != tt.optional {
				t.Errorf("ParseIndexTag() optional = %v, want %v", tag.Optional, tt.optional)
			}
		})
	}
}

func TestParseIndexTagCustomName(t *testing.T) {
	tag, ok := ParseIndexTag("index,name:custom-index-name")
	if !ok {
		t.Fatal("ParseIndexTag() failed")
	}
	if tag.Name != "custom-index-name" {
		t.Errorf("ParseIndexTag() name = %v, want %v", tag.Name, "custom-index-name")
	}
}

func TestAutoRegisterIndexes(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Create Redis indexer
	redis := setupTestRedis(t)
	if redis == nil {
		t.Skip("Redis not available")
	}
	redisIndexer := NewRedisIndexer(redis)

	// Test auto-registration
	err := AutoRegisterIndexes(redisIndexer, "users", &AutoIndexTestUser{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes() error = %v", err)
	}

	// Verify indexes were registered
	if len(redisIndexer.specs) != 4 { // email, platform_user_id, referral_code, username
		t.Errorf("Expected 4 indexes registered, got %d", len(redisIndexer.specs))
	}

	// Verify specific index names
	expectedIndexes := []string{
		"users-by-email",
		"users-by-platform-user-id",
		"users-by-referral-code",
		"custom-username-index",
	}

	for _, name := range expectedIndexes {
		if _, exists := redisIndexer.specs[name]; !exists {
			t.Errorf("Expected index %s to be registered", name)
		}
	}

	// Test that indexes can be used
	user := &AutoIndexTestUser{
		ID:             "user1",
		Email:          "test@example.com",
		PlatformUserID: "platform123",
		Username:       "testuser",
	}

	userKey := "users/user1.json"
	userJSON, _ := store.MarshalObject(user)

	// Update indexes
	err = redisIndexer.UpdateIndexes(ctx, userKey, userJSON)
	if err != nil {
		t.Fatalf("UpdateIndexes() error = %v", err)
	}

	// Verify email index was created
	keys, err := redisIndexer.Query(ctx, "users", "email", "test@example.com")
	if err != nil {
		t.Errorf("Failed to query email index: %v", err)
	}
	if len(keys) != 1 || keys[0] != userKey {
		t.Errorf("Email index returned wrong keys: got %v, want [%s]", keys, userKey)
	}
}

func TestAutoRegisterIndexesOptional(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	redis := setupTestRedis(t)
	if redis == nil {
		t.Skip("Redis not available")
	}
	redisIndexer := NewRedisIndexer(redis)

	err := AutoRegisterIndexes(redisIndexer, "users", &AutoIndexTestUser{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes() error = %v", err)
	}

	// Create user without referral code (optional field)
	user := &AutoIndexTestUser{
		ID:             "user1",
		Email:          "test@example.com",
		PlatformUserID: "platform123",
		// ReferralCode is empty and optional
	}

	userKey := "users/user1.json"
	userJSON, _ := store.MarshalObject(user)

	// This should not error even though referral code is empty
	err = redisIndexer.UpdateIndexes(context.Background(), userKey, userJSON)
	if err != nil {
		t.Errorf("UpdateIndexes() with optional empty field should not error: %v", err)
	}
}

func TestAutoRegisterIndexesRequiresRedis(t *testing.T) {
	// Test that AutoRegisterIndexes requires Redis (tests should use miniredis)
	err := AutoRegisterIndexes(nil, "users", &AutoIndexTestUser{})
	if err == nil {
		t.Error("AutoRegisterIndexes() should error when Redis indexer is nil - tests must use miniredis")
	}
}

func TestAutoRegisterIndexesInvalidType(t *testing.T) {
	redis := setupTestRedis(t)
	if redis == nil {
		t.Skip("Redis not available")
	}
	redisIndexer := NewRedisIndexer(redis)

	// Test with non-struct
	err := AutoRegisterIndexes(redisIndexer, "strings", "not a struct")
	if err == nil {
		t.Error("AutoRegisterIndexes() should error with non-struct type")
	}
}

func TestAutoRegisterIndexesMultipleTypes(t *testing.T) {
	redis := setupTestRedis(t)
	if redis == nil {
		t.Skip("Redis not available")
	}
	redisIndexer := NewRedisIndexer(redis)

	// Register indexes for multiple types
	err := AutoRegisterIndexes(redisIndexer, "users", &AutoIndexTestUser{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes(users) error = %v", err)
	}

	err = AutoRegisterIndexes(redisIndexer, "products", &AutoIndexTestProduct{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes(products) error = %v", err)
	}

	// Verify expected number of indexes
	// Users: email, platform_user_id, referral_code, username = 4
	// Products: sku, category = 2
	expectedCount := 6
	if len(redisIndexer.specs) != expectedCount {
		t.Errorf("Expected %d indexes, got %d", expectedCount, len(redisIndexer.specs))
	}
}

func TestRegisterIndexesForType(t *testing.T) {
	redis := setupTestRedis(t)
	if redis == nil {
		t.Skip("Redis not available")
	}
	redisIndexer := NewRedisIndexer(redis)

	// Test combined auto + manual registration
	manualCalled := false
	err := RegisterIndexesForType(
		IndexConfig{
			EntityType:   "users",
			RedisIndexer: redisIndexer,
		},
		&AutoIndexTestUser{},
		func() {
			// Manual registration for complex index
			manualCalled = true
		},
	)

	if err != nil {
		t.Fatalf("RegisterIndexesForType() error = %v", err)
	}

	if !manualCalled {
		t.Error("Manual index registration callback was not called")
	}

	// Verify auto-registered indexes exist
	if _, exists := redisIndexer.specs["users-by-email"]; !exists {
		t.Error("Auto-registered index should exist after RegisterIndexesForType")
	}
}
