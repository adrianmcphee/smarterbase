package smarterbase

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestNewZapLogger tests creating a ZapLogger from a standard zap.Logger
func TestNewZapLogger(t *testing.T) {
	core, _ := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	zapLogger := NewZapLogger(logger)
	if zapLogger == nil {
		t.Fatal("expected ZapLogger, got nil")
	}

	// Test that logging works
	zapLogger.Info("test message", "key", "value")
}

// TestNewZapLoggerFromSugar tests creating a ZapLogger from sugared logger
func TestNewZapLoggerFromSugar(t *testing.T) {
	core, _ := observer.New(zapcore.InfoLevel)
	logger := zap.New(core).Sugar()

	zapLogger := NewZapLoggerFromSugar(logger)
	if zapLogger == nil {
		t.Fatal("expected ZapLogger, got nil")
	}

	// Test that logging works
	zapLogger.Info("test message", "key", "value")
}

// TestNewProductionZapLogger tests production logger creation
func TestNewProductionZapLogger(t *testing.T) {
	logger, err := NewProductionZapLogger()
	if err != nil {
		t.Fatalf("failed to create production logger: %v", err)
	}

	if logger == nil {
		t.Fatal("expected logger, got nil")
	}

	// Test all log levels
	logger.Debug("debug message", "key", "value")
	logger.Info("info message", "key", "value")
	logger.Warn("warn message", "key", "value")
	logger.Error("error message", "key", "value")

	// Test Sync
	if err := logger.Sync(); err != nil {
		// Sync can fail on stdout/stderr in tests, that's ok
		t.Logf("sync returned error (expected in tests): %v", err)
	}
}

// TestNewDevelopmentZapLogger tests development logger creation
func TestNewDevelopmentZapLogger(t *testing.T) {
	logger, err := NewDevelopmentZapLogger()
	if err != nil {
		t.Fatalf("failed to create development logger: %v", err)
	}

	if logger == nil {
		t.Fatal("expected logger, got nil")
	}

	// Test all log levels
	logger.Debug("debug message", "key", "value")
	logger.Info("info message", "key", "value")
	logger.Warn("warn message", "key", "value")
	logger.Error("error message", "key", "value")
}

// TestZapLoggerMethods tests all ZapLogger methods with observer
func TestZapLoggerMethods(t *testing.T) {
	// Use observer to verify logs are actually written
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	zapLogger := NewZapLogger(logger)

	// Test Debug
	zapLogger.Debug("debug message", "key", "value")
	if recorded.Len() != 1 {
		t.Errorf("expected 1 log entry, got %d", recorded.Len())
	}

	// Test Info
	zapLogger.Info("info message", "key", "value")
	if recorded.Len() != 2 {
		t.Errorf("expected 2 log entries, got %d", recorded.Len())
	}

	// Test Warn
	zapLogger.Warn("warn message", "key", "value")
	if recorded.Len() != 3 {
		t.Errorf("expected 3 log entries, got %d", recorded.Len())
	}

	// Test Error
	zapLogger.Error("error message", "key", "value")
	if recorded.Len() != 4 {
		t.Errorf("expected 4 log entries, got %d", recorded.Len())
	}

	// Verify log levels
	entries := recorded.All()
	if entries[0].Level != zapcore.DebugLevel {
		t.Errorf("expected Debug level, got %v", entries[0].Level)
	}
	if entries[1].Level != zapcore.InfoLevel {
		t.Errorf("expected Info level, got %v", entries[1].Level)
	}
	if entries[2].Level != zapcore.WarnLevel {
		t.Errorf("expected Warn level, got %v", entries[2].Level)
	}
	if entries[3].Level != zapcore.ErrorLevel {
		t.Errorf("expected Error level, got %v", entries[3].Level)
	}
}

// TestZapLoggerFields tests that fields are properly passed
func TestZapLoggerFields(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	zapLogger := NewZapLogger(logger)

	// Test with various field types
	zapLogger.Info("message",
		"string", "value",
		"int", 42,
		"float", 3.14,
		"bool", true,
	)

	if recorded.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", recorded.Len())
	}

	entry := recorded.All()[0]
	if entry.Message != "message" {
		t.Errorf("expected message 'message', got '%s'", entry.Message)
	}

	// Verify fields were captured
	context := entry.ContextMap()
	if context["string"] != "value" {
		t.Errorf("expected string field 'value', got '%v'", context["string"])
	}
	if context["int"] != int64(42) {
		t.Errorf("expected int field 42, got '%v'", context["int"])
	}
	if context["bool"] != true {
		t.Errorf("expected bool field true, got '%v'", context["bool"])
	}
}

// TestZapLoggerImplementsInterface verifies ZapLogger implements Logger
func TestZapLoggerImplementsInterface(t *testing.T) {
	var _ Logger = &ZapLogger{}
}

// TestZapLoggerSync tests the Sync method
func TestZapLoggerSync(t *testing.T) {
	core, _ := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	zapLogger := NewZapLogger(logger)

	// Sync should not panic
	err := zapLogger.Sync()
	if err != nil {
		t.Logf("sync returned error (can happen with memory logger): %v", err)
	}
}
