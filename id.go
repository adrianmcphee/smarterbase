package smarterbase

import (
	"github.com/google/uuid"
)

// NewID generates a UUIDv7 (time-ordered) identifier
// UUIDv7 benefits:
// - Sortable by creation time
// - Database index friendly
// - Distributed system friendly (no coordination needed)
// - Can infer creation time from ID
func NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// Fall back to UUIDv4 if NewV7 fails (extremely rare)
		id = uuid.New()
	}
	return id.String()
}

// ParseID parses a UUID string
func ParseID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// IsValidID checks if a string is a valid UUID
func IsValidID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
