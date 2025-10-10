package smarterbase

import "fmt"

// Logger provides structured logging for Smarterbase operations
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NoOpLogger is a logger that does nothing
type NoOpLogger struct{}

func (l *NoOpLogger) Debug(msg string, fields ...interface{}) {}
func (l *NoOpLogger) Info(msg string, fields ...interface{})  {}
func (l *NoOpLogger) Warn(msg string, fields ...interface{})  {}
func (l *NoOpLogger) Error(msg string, fields ...interface{}) {}

// StdLogger uses standard library log package
// This is a simple implementation for development
type StdLogger struct {
	prefix string
}

func NewStdLogger(prefix string) *StdLogger {
	return &StdLogger{prefix: prefix}
}

func (l *StdLogger) Debug(msg string, fields ...interface{}) {
	l.log("DEBUG", msg, fields...)
}

func (l *StdLogger) Info(msg string, fields ...interface{}) {
	l.log("INFO", msg, fields...)
}

func (l *StdLogger) Warn(msg string, fields ...interface{}) {
	l.log("WARN", msg, fields...)
}

func (l *StdLogger) Error(msg string, fields ...interface{}) {
	l.log("ERROR", msg, fields...)
}

func (l *StdLogger) log(level string, msg string, fields ...interface{}) {
	// Simple key-value formatting
	fieldStr := ""
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			fieldStr += " " + toString(fields[i]) + "=" + toString(fields[i+1])
		}
	}
	println(l.prefix + " [" + level + "] " + msg + fieldStr)
}

func toString(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// Production integrations:
//
// For go.uber.org/zap:
//   type ZapLogger struct { logger *zap.SugaredLogger }
//   func (l *ZapLogger) Debug(msg string, fields ...interface{}) {
//       l.logger.Debugw(msg, fields...)
//   }
//
// For logrus:
//   type LogrusLogger struct { logger *logrus.Logger }
//   func (l *LogrusLogger) Debug(msg string, fields ...interface{}) {
//       l.logger.WithFields(toLogrusFields(fields)).Debug(msg)
//   }
