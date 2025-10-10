package smarterbase

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewID(t *testing.T) {
	// Generate multiple IDs
	id1 := NewID()
	time.Sleep(1 * time.Millisecond)
	id2 := NewID()

	// Both should be valid UUIDs
	if !IsValidID(id1) {
		t.Errorf("NewID() generated invalid ID: %s", id1)
	}

	if !IsValidID(id2) {
		t.Errorf("NewID() generated invalid ID: %s", id2)
	}

	// IDs should be different
	if id1 == id2 {
		t.Error("NewID() generated duplicate IDs")
	}

	// Parse as UUIDv7 and verify time-ordering
	uuid1, _ := ParseID(id1)
	uuid2, _ := ParseID(id2)

	// UUIDv7 should be lexicographically sortable by time
	if id1 > id2 {
		t.Error("UUIDv7 not time-ordered: id1 should be < id2")
	}

	// Verify they're version 7
	if uuid1.Version() != 7 {
		t.Errorf("Expected UUIDv7, got version %d", uuid1.Version())
	}

	if uuid2.Version() != 7 {
		t.Errorf("Expected UUIDv7, got version %d", uuid2.Version())
	}
}

func TestParseID(t *testing.T) {
	id := NewID()

	parsed, err := ParseID(id)
	if err != nil {
		t.Fatalf("ParseID failed: %v", err)
	}

	if parsed.String() != id {
		t.Errorf("Parsed ID doesn't match: %s != %s", parsed.String(), id)
	}

	// Test invalid ID
	_, err = ParseID("invalid-uuid")
	if err == nil {
		t.Error("Expected error when parsing invalid UUID")
	}
}

func TestIsValidID(t *testing.T) {
	testCases := []struct {
		id    string
		valid bool
	}{
		{NewID(), true},
		{uuid.New().String(), true},
		{"invalid", false},
		{"", false},
		{"123", false},
		{"00000000-0000-0000-0000-000000000000", true}, // Nil UUID is technically valid
	}

	for _, tc := range testCases {
		result := IsValidID(tc.id)
		if result != tc.valid {
			t.Errorf("IsValidID(%q) = %v, want %v", tc.id, result, tc.valid)
		}
	}
}

func BenchmarkNewID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewID()
	}
}
