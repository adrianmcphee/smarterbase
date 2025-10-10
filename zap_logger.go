package smarterbase

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ZapLogger adapts go.uber.org/zap logger to the Smarterbase Logger interface
type ZapLogger struct {
	logger *zap.SugaredLogger
}

// NewZapLogger creates a new Zap logger adapter
func NewZapLogger(logger *zap.Logger) *ZapLogger {
	return &ZapLogger{
		logger: logger.Sugar(),
	}
}

// NewZapLoggerFromSugar creates a logger from an existing sugared logger
func NewZapLoggerFromSugar(logger *zap.SugaredLogger) *ZapLogger {
	return &ZapLogger{
		logger: logger,
	}
}

// NewProductionZapLogger creates a production-ready Zap logger
// This is a convenience function for common use cases
func NewProductionZapLogger() (*ZapLogger, error) {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return NewZapLogger(logger), nil
}

// NewDevelopmentZapLogger creates a development Zap logger
// This is optimized for human-readable console output
func NewDevelopmentZapLogger() (*ZapLogger, error) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, err
	}

	return NewZapLogger(logger), nil
}

func (l *ZapLogger) Debug(msg string, fields ...interface{}) {
	l.logger.Debugw(msg, fields...)
}

func (l *ZapLogger) Info(msg string, fields ...interface{}) {
	l.logger.Infow(msg, fields...)
}

func (l *ZapLogger) Warn(msg string, fields ...interface{}) {
	l.logger.Warnw(msg, fields...)
}

func (l *ZapLogger) Error(msg string, fields ...interface{}) {
	l.logger.Errorw(msg, fields...)
}

// Sync flushes any buffered log entries
// Should be called before application exit
func (l *ZapLogger) Sync() error {
	return l.logger.Sync()
}

// Example usage:
//
//	// Production logging
//	logger, err := smarterbase.NewProductionZapLogger()
//	if err != nil {
//	    panic(err)
//	}
//	defer logger.Sync()
//
//	store := smarterbase.NewStoreWithLogger(backend, logger)
//
//	// Or with custom Zap config
//	config := zap.NewProductionConfig()
//	config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
//	zapLogger, _ := config.Build()
//
//	logger := smarterbase.NewZapLogger(zapLogger)
//	store := smarterbase.NewStoreWithLogger(backend, logger)
