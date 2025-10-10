package smarterbase

import (
	"bytes"
	"testing"
)

func TestNoOpLogger(t *testing.T) {
	// NoOpLogger should not panic or produce output
	logger := &NoOpLogger{}

	// These should all be safe to call
	logger.Debug("test message", "key", "value")
	logger.Info("test message", "key", "value")
	logger.Warn("test message", "key", "value")
	logger.Error("test message", "key", "value")

	// If we get here without panicking, test passes
}

func TestStdLogger(t *testing.T) {
	// Capture output by temporarily redirecting
	var buf bytes.Buffer
	logger := &StdLogger{}

	// We can't easily intercept stdout in tests without more complex setup,
	// but we can at least verify the logger doesn't panic
	logger.Debug("debug message", "key", "value")
	logger.Info("info message", "key", "value")
	logger.Warn("warn message", "key", "value")
	logger.Error("error message", "key", "value")

	// Test logger accepts various field types
	logger.Info("test",
		"string", "value",
		"int", 42,
		"float", 3.14,
		"bool", true,
		"nil", nil,
	)

	// Verify output buffer (would need to redirect stdout to test properly)
	_ = buf
}

func TestLoggerInterface(t *testing.T) {
	// Verify both loggers implement the Logger interface
	var _ Logger = &NoOpLogger{}
	var _ Logger = &StdLogger{}
}

func TestStdLoggerFormatting(t *testing.T) {
	logger := &StdLogger{}

	// These calls should not panic with various field combinations
	testCases := []struct {
		name   string
		msg    string
		fields []interface{}
	}{
		{"no fields", "simple message", nil},
		{"one pair", "message", []interface{}{"key", "value"}},
		{"multiple pairs", "message", []interface{}{"k1", "v1", "k2", "v2"}},
		{"odd fields", "message", []interface{}{"k1", "v1", "k2"}}, // Missing value
		{"mixed types", "message", []interface{}{
			"string", "value",
			"int", 123,
			"float", 45.67,
			"bool", true,
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			logger.Info(tc.msg, tc.fields...)
			logger.Debug(tc.msg, tc.fields...)
			logger.Warn(tc.msg, tc.fields...)
			logger.Error(tc.msg, tc.fields...)
		})
	}
}

// Note: MockLogger is defined in index_manager_test.go to avoid duplication
