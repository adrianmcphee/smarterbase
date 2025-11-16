package smarterbase

import (
	"context"
	"testing"
)

// Test domain models (prefixed to avoid conflicts)
type AutoIndexTestUser struct {
	ID             string `json:"id"`
	Email          string `json:"email" sb:"index,unique"`
	PlatformUserID string `json:"platform_user_id" sb:"index:unique"`
	ReferralCode   string `json:"referral_code,omitempty" sb:"index,unique,optional"`
	Username       string `json:"username" sb:"index,unique,name:custom-username-index"`
}

type AutoIndexTestSession struct {
	ID     string `json:"id"`
	Token  string `json:"token" sb:"index,unique"`
	UserID string `json:"user_id" sb:"index,multi"`
}

type AutoIndexTestProduct struct {
	ID       string `json:"id"`
	SKU      string `json:"sku" sb:"index,unique"`
	Category string `json:"category" sb:"index,multi"`
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
			name:     "unique with colon",
			tag:      "index:unique",
			wantType: "unique",
			wantOk:   true,
		},
		{
			name:     "unique with comma",
			tag:      "index,unique",
			wantType: "unique",
			wantOk:   true,
		},
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
			name:     "optional unique",
			tag:      "index,unique,optional",
			wantType: "unique",
			wantOk:   true,
			optional: true,
		},
		{
			name:     "just index defaults to multi",
			tag:      "index",
			wantType: "multi",
			wantOk:   true,
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
	tag, ok := ParseIndexTag("index,unique,name:custom-index-name")
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
	indexer := NewIndexer(store)

	// Test auto-registration
	err := AutoRegisterIndexes(indexer, nil, "users", &AutoIndexTestUser{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes() error = %v", err)
	}

	// Verify indexes were registered by checking specs
	if len(indexer.specs) != 4 { // email, platform_user_id, referral_code, username
		t.Errorf("Expected 4 indexes registered, got %d", len(indexer.specs))
	}

	// Verify specific index names
	expectedIndexes := []string{
		"users-by-email",
		"users-by-platform-user-id",
		"users-by-referral-code",
		"custom-username-index",
	}

	for _, name := range expectedIndexes {
		if _, exists := indexer.specs[name]; !exists {
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
	err = indexer.UpdateIndexes(ctx, userKey, userJSON)
	if err != nil {
		t.Fatalf("UpdateIndexes() error = %v", err)
	}

	// Verify email index was created
	objectKey, err := indexer.QueryIndex(ctx, "users-by-email", "test@example.com")
	if err != nil {
		t.Errorf("Failed to query email index: %v", err)
	}
	if objectKey != userKey {
		t.Errorf("Email index returned wrong key: got %s, want %s", objectKey, userKey)
	}
}

func TestAutoRegisterIndexesOptional(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	indexer := NewIndexer(store)

	err := AutoRegisterIndexes(indexer, nil, "users", &AutoIndexTestUser{})
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
	err = indexer.UpdateIndexes(context.Background(), userKey, userJSON)
	if err != nil {
		t.Errorf("UpdateIndexes() with optional empty field should not error: %v", err)
	}
}

func TestAutoRegisterIndexesWithRedis(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	indexer := NewIndexer(store)

	// Skip Redis and just verify file indexes work
	err := AutoRegisterIndexes(indexer, nil, "sessions", &AutoIndexTestSession{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes() error = %v", err)
	}

	// Verify unique index (token) was registered
	if _, exists := indexer.specs["sessions-by-token"]; !exists {
		t.Error("Expected sessions-by-token index to be registered")
	}

	// Multi-indexes should be skipped when Redis is nil (graceful degradation)
	if _, exists := indexer.specs["sessions-by-user-id"]; exists {
		t.Error("Multi-index should be skipped when Redis is nil")
	}
}

func TestAutoRegisterIndexesInvalidType(t *testing.T) {
	indexer := NewIndexer(NewStore(NewFilesystemBackend(t.TempDir())))

	// Test with non-struct
	err := AutoRegisterIndexes(indexer, nil, "strings", "not a struct")
	if err == nil {
		t.Error("AutoRegisterIndexes() should error with non-struct type")
	}
}

func TestAutoRegisterIndexesMultipleTypes(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	indexer := NewIndexer(store)

	// Register indexes for multiple types
	err := AutoRegisterIndexes(indexer, nil, "users", &AutoIndexTestUser{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes(users) error = %v", err)
	}

	err = AutoRegisterIndexes(indexer, nil, "products", &AutoIndexTestProduct{})
	if err != nil {
		t.Fatalf("AutoRegisterIndexes(products) error = %v", err)
	}

	// Verify expected number of indexes
	// Users: email, platform_user_id, referral_code, username = 4
	// Products: sku = 1 (category is multi, skipped without Redis)
	expectedCount := 5
	if len(indexer.specs) != expectedCount {
		t.Errorf("Expected %d indexes, got %d", expectedCount, len(indexer.specs))
	}
}

func TestRegisterIndexesForType(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	indexer := NewIndexer(store)

	// Test combined auto + manual registration
	manualCalled := false
	err := RegisterIndexesForType(
		IndexConfig{
			EntityType:  "users",
			FileIndexer: indexer,
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
	if _, exists := indexer.specs["users-by-email"]; !exists {
		t.Error("Auto-registered index should exist after RegisterIndexesForType")
	}
}
