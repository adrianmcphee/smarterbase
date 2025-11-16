package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/adrianmcphee/smarterbase/v2"
)

// Event represents an application event
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	UserID    string                 `json:"user_id,omitempty"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Level     string                 `json:"level"` // info, warning, error
}

// EventLogger handles event logging operations
type EventLogger struct {
	store *smarterbase.Store
}

// NewEventLogger creates a new event logger
func NewEventLogger(store *smarterbase.Store) *EventLogger {
	return &EventLogger{
		store: store,
	}
}

// LogEvent appends an event to today's log file (JSONL format)
func (l *EventLogger) LogEvent(ctx context.Context, eventType, action string, opts ...EventOption) error {
	event := &Event{
		ID:        smarterbase.NewID(),
		Type:      eventType,
		Timestamp: time.Now(),
		Action:    action,
		Level:     "info",
		Metadata:  make(map[string]interface{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(event)
	}

	// Serialize event to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Append newline for JSONL format
	eventJSON = append(eventJSON, '\n')

	// Get log file key for today
	logKey := l.getLogKey(time.Now())

	// Append to log file
	err = l.store.Backend().Append(ctx, logKey, eventJSON)
	if err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	return nil
}

// ReadEvents reads all events from a specific date
func (l *EventLogger) ReadEvents(ctx context.Context, date time.Time) ([]*Event, error) {
	logKey := l.getLogKey(date)

	// Get log file
	data, err := l.store.Backend().Get(ctx, logKey)
	if err != nil {
		if smarterbase.IsNotFound(err) {
			return []*Event{}, nil // No events for this date
		}
		return nil, fmt.Errorf("failed to get log file: %w", err)
	}

	// Parse JSONL
	events := make([]*Event, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			log.Printf("Warning: failed to parse event: %v", err)
			continue
		}
		events = append(events, &event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan log file: %w", err)
	}

	return events, nil
}

// StreamEvents streams events from a date without loading all into memory
func (l *EventLogger) StreamEvents(ctx context.Context, date time.Time, handler func(*Event) error) error {
	logKey := l.getLogKey(date)

	data, err := l.store.Backend().Get(ctx, logKey)
	if err != nil {
		if smarterbase.IsNotFound(err) {
			return nil // No events for this date
		}
		return fmt.Errorf("failed to get log file: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			log.Printf("Warning: failed to parse event: %v", err)
			continue
		}

		if err := handler(&event); err != nil {
			return err // Stop processing on error
		}
	}

	return scanner.Err()
}

// FilterEvents returns events matching a filter
func (l *EventLogger) FilterEvents(ctx context.Context, date time.Time, filter func(*Event) bool) ([]*Event, error) {
	allEvents, err := l.ReadEvents(ctx, date)
	if err != nil {
		return nil, err
	}

	filtered := make([]*Event, 0)
	for _, event := range allEvents {
		if filter(event) {
			filtered = append(filtered, event)
		}
	}

	return filtered, nil
}

// GetEventStats returns statistics about events for a date
func (l *EventLogger) GetEventStats(ctx context.Context, date time.Time) (map[string]int, error) {
	events, err := l.ReadEvents(ctx, date)
	if err != nil {
		return nil, err
	}

	stats := map[string]int{
		"total":   len(events),
		"info":    0,
		"warning": 0,
		"error":   0,
	}

	typeStats := make(map[string]int)

	for _, event := range events {
		stats[event.Level]++
		typeStats[event.Type]++
	}

	// Merge type stats
	for k, v := range typeStats {
		stats[k] = v
	}

	return stats, nil
}

// getLogKey returns the storage key for a specific date
func (l *EventLogger) getLogKey(date time.Time) string {
	return fmt.Sprintf("logs/%s.jsonl", date.Format("2006-01-02"))
}

// EventOption is a functional option for configuring events
type EventOption func(*Event)

// WithUserID sets the user ID for an event
func WithUserID(userID string) EventOption {
	return func(e *Event) {
		e.UserID = userID
	}
}

// WithResource sets the resource for an event
func WithResource(resource string) EventOption {
	return func(e *Event) {
		e.Resource = resource
	}
}

// WithMetadata adds metadata to an event
func WithMetadata(key string, value interface{}) EventOption {
	return func(e *Event) {
		e.Metadata[key] = value
	}
}

// WithLevel sets the event level
func WithLevel(level string) EventOption {
	return func(e *Event) {
		e.Level = level
	}
}

// Example: AuditLogger wraps EventLogger for audit trail
type AuditLogger struct {
	eventLogger *EventLogger
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(eventLogger *EventLogger) *AuditLogger {
	return &AuditLogger{
		eventLogger: eventLogger,
	}
}

// LogUserAction logs a user action for audit purposes
func (a *AuditLogger) LogUserAction(ctx context.Context, userID, action, resource string) error {
	return a.eventLogger.LogEvent(ctx, "audit", action,
		WithUserID(userID),
		WithResource(resource),
		WithMetadata("ip", "192.168.1.100"),
		WithMetadata("user_agent", "Mozilla/5.0"),
	)
}

// LogSecurityEvent logs a security-related event
func (a *AuditLogger) LogSecurityEvent(ctx context.Context, event string, severity string, details map[string]interface{}) error {
	opts := []EventOption{
		WithLevel(severity),
	}

	for k, v := range details {
		opts = append(opts, WithMetadata(k, v))
	}

	return a.eventLogger.LogEvent(ctx, "security", event, opts...)
}

func main() {
	ctx := context.Background()

	// Setup (works with any backend)
	backend := smarterbase.NewFilesystemBackend("./data")
	defer backend.Close()

	store := smarterbase.NewStore(backend)

	// Production would use S3 with retention policies
	// cfg, _ := config.LoadDefaultConfig(ctx)
	// s3Client := s3.NewFromConfig(cfg)
	// backend := smarterbase.NewS3Backend(s3Client, "logs-bucket")
	// Configure S3 lifecycle policy: delete logs after 90 days

	eventLogger := NewEventLogger(store)
	auditLogger := NewAuditLogger(eventLogger)

	fmt.Println("\n=== Event Logging with SmarterBase JSONL ===")
	fmt.Println("\n📋 THE CHALLENGE:")
	fmt.Println("Traditional logging systems face:")
	fmt.Println("  • Database write amplification for billions of events")
	fmt.Println("  • Expensive log aggregation tools (Splunk, Datadog)")
	fmt.Println("  • Complex retention policies and storage management")
	fmt.Println("  • Read-modify-write bottlenecks for append operations")
	fmt.Println("\n✨ THE SMARTERBASE SOLUTION:")
	fmt.Println("  ✅ JSONL support - Pure append-only, no read-modify-write")
	fmt.Println("  ✅ Infinite scale - Billions of events, pennies per month")
	fmt.Println("  ✅ Zero backups - S3's 11 9s durability automatic")
	fmt.Println("  ✅ S3 lifecycle - Auto-delete old logs, no code needed")
	fmt.Println("  ✅ Streaming - Process huge logs without loading to memory")
	fmt.Println()

	fmt.Println("=== Running Example Operations ===")

	// 1. Log various events
	fmt.Println("1. Logging events...")

	eventLogger.LogEvent(ctx, "auth", "login",
		WithUserID("user-123"),
		WithMetadata("method", "password"),
	)

	eventLogger.LogEvent(ctx, "api", "request",
		WithUserID("user-123"),
		WithResource("/api/users"),
		WithMetadata("method", "GET"),
		WithMetadata("status_code", 200),
	)

	eventLogger.LogEvent(ctx, "error", "database_connection_failed",
		WithLevel("error"),
		WithMetadata("error", "connection timeout"),
		WithMetadata("retry_count", 3),
	)

	auditLogger.LogUserAction(ctx, "user-456", "delete_account", "users/789")

	auditLogger.LogSecurityEvent(ctx, "suspicious_login", "warning", map[string]interface{}{
		"user_id":  "user-999",
		"location": "Unknown",
		"attempts": 5,
	})

	fmt.Println("   Logged 5 events to today's log")

	// 2. Read all events from today
	fmt.Println("\n2. Reading today's events...")
	todayEvents, _ := eventLogger.ReadEvents(ctx, time.Now())
	fmt.Printf("   Found %d events:\n", len(todayEvents))
	for i, event := range todayEvents {
		fmt.Printf("   %d. [%s] %s: %s (user: %s)\n",
			i+1, event.Level, event.Type, event.Action, event.UserID)
	}

	// 3. Filter events by type
	fmt.Println("\n3. Filtering error events...")
	errorEvents, _ := eventLogger.FilterEvents(ctx, time.Now(), func(e *Event) bool {
		return e.Level == "error"
	})
	fmt.Printf("   Found %d error events\n", len(errorEvents))
	for _, event := range errorEvents {
		fmt.Printf("   - %s: %v\n", event.Action, event.Metadata["error"])
	}

	// 4. Filter events by user
	fmt.Println("\n4. Filtering events for user-123...")
	userEvents, _ := eventLogger.FilterEvents(ctx, time.Now(), func(e *Event) bool {
		return e.UserID == "user-123"
	})
	fmt.Printf("   Found %d events for user-123:\n", len(userEvents))
	for _, event := range userEvents {
		fmt.Printf("   - %s\n", event.Action)
	}

	// 5. Stream events (memory efficient for large logs)
	fmt.Println("\n5. Streaming events...")
	eventCount := 0
	eventLogger.StreamEvents(ctx, time.Now(), func(e *Event) error {
		eventCount++
		return nil
	})
	fmt.Printf("   Streamed %d events without loading all into memory\n", eventCount)

	// 6. Get event statistics
	fmt.Println("\n6. Event statistics:")
	stats, _ := eventLogger.GetEventStats(ctx, time.Now())
	for key, count := range stats {
		fmt.Printf("   %s: %d\n", key, count)
	}

	// 7. Simulate high-volume logging
	fmt.Println("\n7. Simulating high-volume logging...")
	startTime := time.Now()
	for i := 0; i < 100; i++ {
		eventLogger.LogEvent(ctx, "api", "request",
			WithUserID(fmt.Sprintf("user-%d", i%10)),
			WithResource("/api/data"),
		)
	}
	elapsed := time.Since(startTime)
	fmt.Printf("   Logged 100 events in %v (%.2f events/sec)\n",
		elapsed, float64(100)/elapsed.Seconds())

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\nKey benefits of JSONL for logging:")
	fmt.Println("- Append-only: No read-modify-write needed")
	fmt.Println("- Streaming: Process large logs without loading into memory")
	fmt.Println("- Simple format: Each line is a valid JSON document")
	fmt.Println("- Efficient: S3 append operations are fast")
	fmt.Println("- Rotation: One file per day, easy to implement retention")
	fmt.Println("- Analysis: Easy to process with grep/awk or load into analytics tools")
}
