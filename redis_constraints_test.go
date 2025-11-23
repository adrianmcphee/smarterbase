package smarterbase

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestConstraintManager_ClaimUniqueKeys tests basic constraint claiming
func TestConstraintManager_ClaimUniqueKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	// Register email constraint
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
		Normalize:  NormalizeEmail,
	})

	ctx := context.Background()

	// Create user data
	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
		"name":  "Alice",
	}

	// Claim constraints - should succeed
	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user)
	if err != nil {
		t.Fatalf("ClaimUniqueKeys failed: %v", err)
	}

	if len(claimedKeys) != 1 {
		t.Fatalf("expected 1 claimed key, got %d", len(claimedKeys))
	}

	expectedKey := "unique:users:email:alice@example.com"
	if claimedKeys[0] != expectedKey {
		t.Errorf("expected key '%s', got '%s'", expectedKey, claimedKeys[0])
	}

	// Verify key exists in Redis
	val, err := redisClient.Get(ctx, expectedKey).Result()
	if err != nil {
		t.Fatalf("expected key to exist in Redis: %v", err)
	}
	if val != "users/user-123" {
		t.Errorf("expected value 'users/user-123', got '%s'", val)
	}
}

// TestConstraintManager_DuplicateDetection tests constraint violations
func TestConstraintManager_DuplicateDetection(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
		Normalize:  NormalizeEmail,
	})

	ctx := context.Background()

	// Create first user
	user1 := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}

	_, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user1)
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}

	// Try to create second user with same email - should fail
	user2 := map[string]interface{}{
		"id":    "user-456",
		"email": "alice@example.com",
	}

	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-456", user2)
	if err == nil {
		t.Fatalf("expected constraint violation, got success")
	}

	if len(claimedKeys) != 0 {
		t.Errorf("expected no claimed keys on failure, got %d", len(claimedKeys))
	}

	// Verify error is ConstraintViolationError
	if !IsConstraintViolation(err) {
		t.Errorf("expected ConstraintViolationError, got %T: %v", err, err)
	}

	constraintErr, ok := err.(*ConstraintViolationError)
	if !ok {
		t.Fatalf("expected *ConstraintViolationError, got %T", err)
	}

	if constraintErr.EntityType != "users" {
		t.Errorf("expected entity_type 'users', got '%s'", constraintErr.EntityType)
	}
	if constraintErr.FieldName != "email" {
		t.Errorf("expected field_name 'email', got '%s'", constraintErr.FieldName)
	}
	if constraintErr.Value != "alice@example.com" {
		t.Errorf("expected value 'alice@example.com', got '%s'", constraintErr.Value)
	}
	if constraintErr.ExistingKey != "users/user-123" {
		t.Errorf("expected existing_key 'users/user-123', got '%s'", constraintErr.ExistingKey)
	}
}

// TestConstraintManager_RollbackOnFailure tests partial claim rollback
func TestConstraintManager_RollbackOnFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	// Register multiple constraints
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "username",
		GetValue:   ExtractJSONFieldForConstraint("username"),
	})

	ctx := context.Background()

	// Claim email for user1
	user1 := map[string]interface{}{
		"id":       "user-123",
		"email":    "alice@example.com",
		"username": "alice",
	}
	_, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user1)
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}

	// Try to claim with different email but same username - should fail and rollback
	user2 := map[string]interface{}{
		"id":       "user-456",
		"email":    "bob@example.com", // Different email (would succeed)
		"username": "alice",           // Same username (will fail)
	}

	_, err = cm.ClaimUniqueKeys(ctx, "users", "users/user-456", user2)
	if err == nil {
		t.Fatalf("expected constraint violation")
	}

	// Verify bob's email constraint was NOT claimed (rollback worked)
	bobEmailKey := "unique:users:email:bob@example.com"
	_, err = redisClient.Get(ctx, bobEmailKey).Result()
	if err != redis.Nil {
		t.Errorf("expected bob's email key to not exist (rollback), but it exists")
	}

	// Verify alice's email is still claimed
	aliceEmailKey := "unique:users:email:alice@example.com"
	val, err := redisClient.Get(ctx, aliceEmailKey).Result()
	if err != nil {
		t.Errorf("alice's email key should still exist: %v", err)
	}
	if val != "users/user-123" {
		t.Errorf("alice's email should point to user-123, got '%s'", val)
	}
}

// TestConstraintManager_ReleaseKeys tests manual key release
func TestConstraintManager_ReleaseKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})

	ctx := context.Background()

	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}

	// Claim constraint
	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	// Release the constraint
	err = cm.ReleaseUniqueKeys(ctx, claimedKeys)
	if err != nil {
		t.Fatalf("release failed: %v", err)
	}

	// Verify key no longer exists
	constraintKey := "unique:users:email:alice@example.com"
	_, err = redisClient.Get(ctx, constraintKey).Result()
	if err != redis.Nil {
		t.Errorf("expected key to be deleted, but it still exists")
	}

	// Verify we can now claim it again
	_, err = cm.ClaimUniqueKeys(ctx, "users", "users/user-456", user)
	if err != nil {
		t.Errorf("expected to claim released key, got error: %v", err)
	}
}

// TestConstraintManager_UpdateUniqueKeys tests updating constraints
func TestConstraintManager_UpdateUniqueKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})

	ctx := context.Background()

	// Create user with initial email
	oldUser := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}

	oldKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", oldUser)
	if err != nil {
		t.Fatalf("initial claim failed: %v", err)
	}

	// Update to new email
	newUser := map[string]interface{}{
		"id":    "user-123",
		"email": "alice.new@example.com",
	}

	newKeys, err := cm.UpdateUniqueKeys(ctx, "users", "users/user-123", oldUser, newUser)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if len(newKeys) != 1 {
		t.Fatalf("expected 1 new key, got %d", len(newKeys))
	}

	// Verify old email is released
	oldEmailKey := "unique:users:email:alice@example.com"
	_, err = redisClient.Get(ctx, oldEmailKey).Result()
	if err != redis.Nil {
		t.Errorf("expected old email key to be released")
	}

	// Verify new email is claimed
	newEmailKey := "unique:users:email:alice.new@example.com"
	val, err := redisClient.Get(ctx, newEmailKey).Result()
	if err != nil {
		t.Errorf("expected new email key to exist: %v", err)
	}
	if val != "users/user-123" {
		t.Errorf("expected new email to point to user-123, got '%s'", val)
	}

	// Verify old keys list is correct
	if len(oldKeys) != 1 || oldKeys[0] != oldEmailKey {
		t.Errorf("old keys mismatch: got %v", oldKeys)
	}
}

// TestConstraintManager_UpdateSameValue tests updating when value doesn't change
func TestConstraintManager_UpdateSameValue(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})

	ctx := context.Background()

	// Create user
	oldUser := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
		"name":  "Alice",
	}

	_, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", oldUser)
	if err != nil {
		t.Fatalf("initial claim failed: %v", err)
	}

	// Update user but keep same email (only change name)
	newUser := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com", // Same email
		"name":  "Alice Smith",       // Different name
	}

	newKeys, err := cm.UpdateUniqueKeys(ctx, "users", "users/user-123", oldUser, newUser)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Should still have the constraint key
	if len(newKeys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(newKeys))
	}

	// Verify email constraint still exists
	emailKey := "unique:users:email:alice@example.com"
	val, err := redisClient.Get(ctx, emailKey).Result()
	if err != nil {
		t.Errorf("expected email key to still exist: %v", err)
	}
	if val != "users/user-123" {
		t.Errorf("expected email to point to user-123, got '%s'", val)
	}
}

// TestConstraintManager_UpdateConflict tests update failing due to conflict
func TestConstraintManager_UpdateConflict(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})

	ctx := context.Background()

	// Create user1
	user1 := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}
	_, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user1)
	if err != nil {
		t.Fatalf("user1 claim failed: %v", err)
	}

	// Create user2 with different email
	user2Old := map[string]interface{}{
		"id":    "user-456",
		"email": "bob@example.com",
	}
	_, err = cm.ClaimUniqueKeys(ctx, "users", "users/user-456", user2Old)
	if err != nil {
		t.Fatalf("user2 claim failed: %v", err)
	}

	// Try to update user2 to use user1's email - should fail
	user2New := map[string]interface{}{
		"id":    "user-456",
		"email": "alice@example.com", // Conflict!
	}

	_, err = cm.UpdateUniqueKeys(ctx, "users", "users/user-456", user2Old, user2New)
	if err == nil {
		t.Fatalf("expected constraint violation on update")
	}

	if !IsConstraintViolation(err) {
		t.Errorf("expected ConstraintViolationError, got %T", err)
	}

	// Verify user2 still has their old email
	bobEmailKey := "unique:users:email:bob@example.com"
	val, err := redisClient.Get(ctx, bobEmailKey).Result()
	if err != nil {
		t.Errorf("user2's old email should still exist: %v", err)
	}
	if val != "users/user-456" {
		t.Errorf("user2's old email should still point to user-456, got '%s'", val)
	}
}

// TestConstraintManager_MultipleConstraints tests multiple constraints per entity
func TestConstraintManager_MultipleConstraints(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	// Register multiple constraints
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "username",
		GetValue:   ExtractJSONFieldForConstraint("username"),
	})
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "referral_code",
		GetValue:   ExtractJSONFieldForConstraint("referral_code"),
	})

	ctx := context.Background()

	user := map[string]interface{}{
		"id":            "user-123",
		"email":         "alice@example.com",
		"username":      "alice",
		"referral_code": "ALICE2025",
	}

	// Claim all constraints
	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if len(claimedKeys) != 3 {
		t.Fatalf("expected 3 claimed keys, got %d", len(claimedKeys))
	}

	// Verify all three constraints exist
	expectedKeys := []string{
		"unique:users:email:alice@example.com",
		"unique:users:username:alice",
		"unique:users:referral_code:ALICE2025",
	}

	for _, expectedKey := range expectedKeys {
		val, err := redisClient.Get(ctx, expectedKey).Result()
		if err != nil {
			t.Errorf("expected key '%s' to exist: %v", expectedKey, err)
		}
		if val != "users/user-123" {
			t.Errorf("key '%s' should point to user-123, got '%s'", expectedKey, val)
		}
	}
}

// TestConstraintManager_EmptyValues tests that empty values don't create constraints
func TestConstraintManager_EmptyValues(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "phone",
		GetValue:   ExtractJSONFieldForConstraint("phone"),
	})

	ctx := context.Background()

	// User with email but no phone
	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
		"phone": "", // Empty value
	}

	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	// Should only claim email, not phone
	if len(claimedKeys) != 1 {
		t.Fatalf("expected 1 claimed key (email only), got %d", len(claimedKeys))
	}

	if claimedKeys[0] != "unique:users:email:alice@example.com" {
		t.Errorf("expected email key, got '%s'", claimedKeys[0])
	}

	// Verify phone constraint was NOT created
	phoneKey := "unique:users:phone:"
	_, err = redisClient.Get(ctx, phoneKey).Result()
	if err != redis.Nil {
		t.Errorf("expected empty phone to not create constraint key")
	}
}

// TestConstraintManager_Normalization tests value normalization
func TestConstraintManager_Normalization(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
		Normalize:  NormalizeEmail, // Lowercase + trim
	})

	ctx := context.Background()

	// Create user with uppercase email
	user1 := map[string]interface{}{
		"id":    "user-123",
		"email": "  ALICE@EXAMPLE.COM  ", // Uppercase with spaces
	}

	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user1)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	// Constraint key should be normalized
	expectedKey := "unique:users:email:alice@example.com"
	if len(claimedKeys) != 1 || claimedKeys[0] != expectedKey {
		t.Fatalf("expected normalized key '%s', got %v", expectedKey, claimedKeys)
	}

	// Try to create another user with same email but different case - should fail
	user2 := map[string]interface{}{
		"id":    "user-456",
		"email": "Alice@Example.Com", // Different case
	}

	_, err = cm.ClaimUniqueKeys(ctx, "users", "users/user-456", user2)
	if err == nil {
		t.Fatalf("expected constraint violation for case-insensitive duplicate")
	}

	if !IsConstraintViolation(err) {
		t.Errorf("expected ConstraintViolationError, got %T", err)
	}
}

// TestConstraintManager_NoRedis tests graceful degradation when Redis is nil
func TestConstraintManager_NoRedis(t *testing.T) {
	cm := NewConstraintManager(nil) // No Redis client

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})

	ctx := context.Background()

	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}

	// Should not fail - graceful degradation
	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user)
	if err != nil {
		t.Errorf("expected graceful degradation when Redis is nil, got error: %v", err)
	}

	if len(claimedKeys) != 0 {
		t.Errorf("expected no claimed keys when Redis is nil, got %d", len(claimedKeys))
	}
}

// TestConstraintManager_NoConstraintsForEntity tests behavior when no constraints registered
func TestConstraintManager_NoConstraintsForEntity(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	// Register constraint for "users" but query for "products"
	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})

	ctx := context.Background()

	product := map[string]interface{}{
		"id":   "product-123",
		"name": "Widget",
	}

	// Should succeed without claiming anything
	claimedKeys, err := cm.ClaimUniqueKeys(ctx, "products", "products/product-123", product)
	if err != nil {
		t.Errorf("expected success when no constraints for entity, got error: %v", err)
	}

	if len(claimedKeys) != 0 {
		t.Errorf("expected no claimed keys, got %d", len(claimedKeys))
	}
}

// TestConstraintManager_VerifyConstraint tests constraint verification
func TestConstraintManager_VerifyConstraint(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	cm := NewConstraintManager(redisClient)

	cm.RegisterConstraint(&UniqueConstraint{
		EntityType: "users",
		FieldName:  "email",
		GetValue:   ExtractJSONFieldForConstraint("email"),
	})

	ctx := context.Background()

	user := map[string]interface{}{
		"id":    "user-123",
		"email": "alice@example.com",
	}

	// Claim constraint
	_, err := cm.ClaimUniqueKeys(ctx, "users", "users/user-123", user)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	// Verify with correct key
	valid, err := cm.VerifyConstraint(ctx, "users", "email", "alice@example.com", "users/user-123")
	if err != nil {
		t.Errorf("verify failed: %v", err)
	}
	if !valid {
		t.Errorf("expected constraint to be valid")
	}

	// Verify with wrong key
	valid, err = cm.VerifyConstraint(ctx, "users", "email", "alice@example.com", "users/user-456")
	if err != nil {
		t.Errorf("verify failed: %v", err)
	}
	if valid {
		t.Errorf("expected constraint to be invalid for wrong key")
	}

	// Verify unclaimed constraint
	valid, err = cm.VerifyConstraint(ctx, "users", "email", "bob@example.com", "users/user-789")
	if err != nil {
		t.Errorf("verify failed: %v", err)
	}
	if valid {
		t.Errorf("expected unclaimed constraint to be invalid")
	}
}

// TestNormalizeEmail tests email normalization helper
func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"alice@example.com", "alice@example.com"},
		{"ALICE@EXAMPLE.COM", "alice@example.com"},
		{"  alice@example.com  ", "alice@example.com"},
		{"Alice@Example.Com", "alice@example.com"},
		{"  ALICE@EXAMPLE.COM  ", "alice@example.com"},
	}

	for _, tt := range tests {
		result := NormalizeEmail(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeEmail(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// TestNormalizeString tests string normalization helper
func TestNormalizeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"  hello  ", "hello"},
		{"  HELLO  ", "HELLO"}, // No lowercase
		{"\t\nhello\n\t", "hello"},
	}

	for _, tt := range tests {
		result := NormalizeString(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeString(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// TestExtractJSONFieldForConstraint tests JSON field extraction
func TestExtractJSONFieldForConstraint(t *testing.T) {
	extractor := ExtractJSONFieldForConstraint("email")

	tests := []struct {
		name      string
		data      interface{}
		expected  string
		expectErr bool
	}{
		{
			name:      "valid email",
			data:      map[string]interface{}{"email": "alice@example.com"},
			expected:  "alice@example.com",
			expectErr: false,
		},
		{
			name:      "missing field",
			data:      map[string]interface{}{"name": "Alice"},
			expected:  "",
			expectErr: true,
		},
		{
			name:      "non-string field",
			data:      map[string]interface{}{"email": 123},
			expected:  "",
			expectErr: true,
		},
		{
			name:      "empty string",
			data:      map[string]interface{}{"email": ""},
			expected:  "",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractor(tt.data)
			if tt.expectErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestConstraintViolationError tests error formatting
func TestConstraintViolationError(t *testing.T) {
	tests := []struct {
		name     string
		err      *ConstraintViolationError
		expected string
	}{
		{
			name: "with existing key",
			err: &ConstraintViolationError{
				EntityType:  "users",
				FieldName:   "email",
				Value:       "alice@example.com",
				ExistingKey: "users/user-123",
			},
			expected: "users with email 'alice@example.com' already exists (owned by users/user-123)",
		},
		{
			name: "without existing key",
			err: &ConstraintViolationError{
				EntityType: "users",
				FieldName:  "email",
				Value:      "alice@example.com",
			},
			expected: "users with email 'alice@example.com' already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestIsConstraintViolation tests error type checking
func TestIsConstraintViolation(t *testing.T) {
	constraintErr := &ConstraintViolationError{
		EntityType: "users",
		FieldName:  "email",
		Value:      "test@example.com",
	}

	if !IsConstraintViolation(constraintErr) {
		t.Errorf("expected IsConstraintViolation to return true for ConstraintViolationError")
	}

	genericErr := fmt.Errorf("some error")
	if IsConstraintViolation(genericErr) {
		t.Errorf("expected IsConstraintViolation to return false for generic error")
	}

	if IsConstraintViolation(nil) {
		t.Errorf("expected IsConstraintViolation to return false for nil")
	}
}
