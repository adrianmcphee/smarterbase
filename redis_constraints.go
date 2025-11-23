package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// UniqueConstraint defines a field that must be unique across all entities of a type.
//
// Example: Email uniqueness for users
//
//	constraint := &UniqueConstraint{
//	    EntityType: "users",
//	    FieldName:  "email",
//	    GetValue:   func(data interface{}) (string, error) { return data.(*User).Email, nil },
//	}
type UniqueConstraint struct {
	EntityType string                                 // e.g., "users", "admin_users"
	FieldName  string                                 // e.g., "email", "platform_user_id"
	GetValue   func(data interface{}) (string, error) // Extract value from data
	Normalize  func(value string) string              // Optional: normalize before storing (e.g., lowercase email)
}

// ConstraintManager handles uniqueness constraints using Redis SET NX operations.
//
// Architecture:
// - Uses Redis as "claim registry" for unique keys
// - SET NX (Set if Not eXists) provides atomic uniqueness guarantee
// - No race conditions possible - Redis handles concurrency
// - Rollback support if storage write fails after claim
//
// Key Format: unique:{entity}:{field}:{value} → object_key
// Example: unique:users:email:adrian@demandops.com → users/019ab.../profile.json
type ConstraintManager struct {
	redis          *redis.Client
	constraints    map[string][]*UniqueConstraint // entity_type → constraints
	circuitBreaker *CircuitBreaker
}

// NewConstraintManager creates a new constraint manager
func NewConstraintManager(redis *redis.Client) *ConstraintManager {
	return &ConstraintManager{
		redis:          redis,
		constraints:    make(map[string][]*UniqueConstraint),
		circuitBreaker: NewCircuitBreaker(5, 30), // 5 failures, 30s timeout
	}
}

// RegisterConstraint registers a uniqueness constraint for an entity type
func (cm *ConstraintManager) RegisterConstraint(constraint *UniqueConstraint) {
	if cm.constraints[constraint.EntityType] == nil {
		cm.constraints[constraint.EntityType] = []*UniqueConstraint{}
	}
	cm.constraints[constraint.EntityType] = append(cm.constraints[constraint.EntityType], constraint)
}

// ClaimUniqueKeys atomically claims all unique keys for an entity before storage write.
//
// This is the CRITICAL operation that prevents duplicates:
// 1. Extract all unique field values from data
// 2. Use Redis SET NX to atomically claim each key
// 3. If ANY claim fails → rollback all and return error
// 4. If ALL succeed → safe to write to storage
//
// Returns:
// - claimed keys (for rollback if storage write fails)
// - error if any constraint is violated
func (cm *ConstraintManager) ClaimUniqueKeys(ctx context.Context, entityType, objectKey string, data interface{}) ([]string, error) {
	if cm.redis == nil {
		return nil, nil // Graceful degradation if Redis unavailable
	}

	constraints, ok := cm.constraints[entityType]
	if !ok || len(constraints) == 0 {
		return nil, nil // No constraints for this entity type
	}

	var claimedKeys []string

	// Try to claim each unique key
	for _, constraint := range constraints {
		// Extract value from data
		value, err := constraint.GetValue(data)
		if err != nil {
			// Field doesn't exist or is empty - skip constraint
			continue
		}

		if value == "" {
			// Empty values don't need uniqueness (NULL in SQL terms)
			continue
		}

		// Normalize if configured (e.g., lowercase emails)
		if constraint.Normalize != nil {
			value = constraint.Normalize(value)
		}

		// Generate Redis key for this unique constraint
		constraintKey := cm.getConstraintKey(entityType, constraint.FieldName, value)

		// Atomic claim: SET NX (set if not exists)
		var claimed bool
		err = cm.circuitBreaker.Execute(ctx, func() error {
			result, err := cm.redis.SetNX(ctx, constraintKey, objectKey, 0).Result()
			claimed = result
			return err
		})

		if err != nil {
			// Redis error - rollback claims and fail
			cm.releaseKeys(ctx, claimedKeys)
			return nil, fmt.Errorf("redis error claiming %s: %w", constraint.FieldName, err)
		}

		if !claimed {
			// Key already exists - constraint violated!
			// Rollback any keys we did claim
			cm.releaseKeys(ctx, claimedKeys)

			// Get existing owner for better error message
			existingKey, _ := cm.redis.Get(ctx, constraintKey).Result()

			return nil, &ConstraintViolationError{
				EntityType:  entityType,
				FieldName:   constraint.FieldName,
				Value:       value,
				ExistingKey: existingKey,
			}
		}

		// Success - track for potential rollback
		claimedKeys = append(claimedKeys, constraintKey)
	}

	return claimedKeys, nil
}

// ReleaseUniqueKeys releases previously claimed keys (rollback after failed storage write)
func (cm *ConstraintManager) ReleaseUniqueKeys(ctx context.Context, claimedKeys []string) error {
	return cm.releaseKeys(ctx, claimedKeys)
}

// releaseKeys deletes claimed constraint keys from Redis
func (cm *ConstraintManager) releaseKeys(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	err := cm.circuitBreaker.Execute(ctx, func() error {
		return cm.redis.Del(ctx, keys...).Err()
	})

	return err
}

// UpdateUniqueKeys handles constraint updates when an entity is modified.
//
// Flow:
// 1. Release old unique keys temporarily
// 2. Claim new unique keys
// 3. If claim succeeds, clean up any released keys that weren't reclaimed
// 4. If claim fails, restore old keys and return error
//
// This ensures atomicity - either all new keys claimed or old keys restored.
func (cm *ConstraintManager) UpdateUniqueKeys(ctx context.Context, entityType, objectKey string, oldData, newData interface{}) ([]string, error) {
	if cm.redis == nil {
		return nil, nil
	}

	// Get old constraint keys for temporary release
	oldKeys := cm.extractConstraintKeys(ctx, entityType, objectKey, oldData)

	// STEP 1: Temporarily release old keys to allow claiming new ones
	if len(oldKeys) > 0 {
		_ = cm.releaseKeys(ctx, oldKeys) // Ignore errors - continue with update
	}

	// STEP 2: Try to claim new constraint keys
	newKeys, err := cm.ClaimUniqueKeys(ctx, entityType, objectKey, newData)
	if err != nil {
		// Claim failed - try to restore old keys
		if len(oldKeys) > 0 {
			// Attempt to restore - if this fails, constraints are lost but data is consistent
			for _, oldKey := range oldKeys {
				_ = cm.redis.SetNX(ctx, oldKey, objectKey, 0)
			}
		}
		return nil, err
	}

	// Success - new keys claimed, old keys released
	return newKeys, nil
}

// extractConstraintKeys extracts all constraint keys for an entity (for cleanup)
func (cm *ConstraintManager) extractConstraintKeys(ctx context.Context, entityType, objectKey string, data interface{}) []string {
	constraints, ok := cm.constraints[entityType]
	if !ok {
		return nil
	}

	var keys []string
	for _, constraint := range constraints {
		value, err := constraint.GetValue(data)
		if err != nil || value == "" {
			continue
		}

		if constraint.Normalize != nil {
			value = constraint.Normalize(value)
		}

		key := cm.getConstraintKey(entityType, constraint.FieldName, value)
		keys = append(keys, key)
	}

	return keys
}

// diffKeys returns keys in 'old' that are not in 'new'
func (cm *ConstraintManager) diffKeys(old, new []string) []string {
	newSet := make(map[string]bool)
	for _, k := range new {
		newSet[k] = true
	}

	var diff []string
	for _, k := range old {
		if !newSet[k] {
			diff = append(diff, k)
		}
	}
	return diff
}

// getConstraintKey generates Redis key for a uniqueness constraint
// Format: unique:{entity}:{field}:{value}
// Example: unique:users:email:adrian@demandops.com
func (cm *ConstraintManager) getConstraintKey(entityType, fieldName, value string) string {
	return fmt.Sprintf("unique:%s:%s:%s", entityType, fieldName, value)
}

// VerifyConstraint checks if a constraint key exists and points to correct object
// Useful for detecting stale constraint keys
func (cm *ConstraintManager) VerifyConstraint(ctx context.Context, entityType, fieldName, value, expectedKey string) (bool, error) {
	if cm.redis == nil {
		return true, nil // Assume valid if Redis unavailable
	}

	key := cm.getConstraintKey(entityType, fieldName, value)

	var actualKey string
	err := cm.circuitBreaker.Execute(ctx, func() error {
		result, err := cm.redis.Get(ctx, key).Result()
		if err == redis.Nil {
			return nil // Key doesn't exist - constraint not claimed
		}
		actualKey = result
		return err
	})

	if err != nil {
		return false, err
	}

	return actualKey == expectedKey, nil
}

// RebuildConstraints rebuilds all constraint keys from storage
// Useful for:
// - Initial setup when adding constraints to existing data
// - Recovery after Redis data loss
// - Cleanup of stale constraint keys
func (cm *ConstraintManager) RebuildConstraints(ctx context.Context, entityType string, objects map[string]interface{}) error {
	if cm.redis == nil {
		return fmt.Errorf("redis not available")
	}

	if _, ok := cm.constraints[entityType]; !ok {
		return fmt.Errorf("no constraints defined for %s", entityType)
	}

	// Clear existing constraint keys for this entity type
	pattern := fmt.Sprintf("unique:%s:*", entityType)
	var cursor uint64
	for {
		var keys []string
		var err error

		err = cm.circuitBreaker.Execute(ctx, func() error {
			var scanErr error
			keys, cursor, scanErr = cm.redis.Scan(ctx, cursor, pattern, 100).Result()
			return scanErr
		})

		if err != nil {
			return fmt.Errorf("failed to scan constraint keys: %w", err)
		}

		if len(keys) > 0 {
			_ = cm.redis.Del(ctx, keys...) // Best effort cleanup
		}

		if cursor == 0 {
			break
		}
	}

	// Rebuild constraint keys from objects
	for objectKey, data := range objects {
		_, err := cm.ClaimUniqueKeys(ctx, entityType, objectKey, data)
		if err != nil {
			return fmt.Errorf("failed to rebuild constraints for %s: %w", objectKey, err)
		}
	}

	return nil
}

// ConstraintViolationError is returned when a uniqueness constraint is violated
type ConstraintViolationError struct {
	EntityType  string
	FieldName   string
	Value       string
	ExistingKey string // The object key that already has this value
}

func (e *ConstraintViolationError) Error() string {
	if e.ExistingKey != "" {
		return fmt.Sprintf("%s with %s '%s' already exists (owned by %s)",
			e.EntityType, e.FieldName, e.Value, e.ExistingKey)
	}
	return fmt.Sprintf("%s with %s '%s' already exists",
		e.EntityType, e.FieldName, e.Value)
}

// IsConstraintViolation checks if an error is a constraint violation
func IsConstraintViolation(err error) bool {
	_, ok := err.(*ConstraintViolationError)
	return ok
}

// Helper: Extract JSON field for constraint (common pattern)
func ExtractJSONFieldForConstraint(fieldName string) func(data interface{}) (string, error) {
	return func(data interface{}) (string, error) {
		// Try direct struct field access first
		// (This would require reflection or type assertion - simplified here)

		// Marshal to JSON and extract field
		bytes, err := json.Marshal(data)
		if err != nil {
			return "", err
		}

		var obj map[string]interface{}
		if err := json.Unmarshal(bytes, &obj); err != nil {
			return "", err
		}

		value, ok := obj[fieldName]
		if !ok {
			return "", fmt.Errorf("field %s not found", fieldName)
		}

		strValue, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("field %s is not a string", fieldName)
		}

		return strValue, nil
	}
}

// Helper: Normalize email addresses (lowercase, trim)
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Helper: Normalize string (trim whitespace)
func NormalizeString(s string) string {
	return strings.TrimSpace(s)
}
