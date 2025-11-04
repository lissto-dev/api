package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

// InitLogger initializes the structured logger
func InitLogger(level string, format string) error {
	var config zap.Config

	if format == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	// Disable caller and stack trace for cleaner logs
	config.DisableCaller = true
	config.DisableStacktrace = true

	var err error
	Logger, err = config.Build()
	if err != nil {
		return err
	}

	return nil
}

// LogDenied logs a denied request with minimal structured data
func LogDenied(reason, user, endpoint string) {
	Logger.Info("denied",
		zap.String("reason", reason),
		zap.String("user", user),
		zap.String("endpoint", endpoint),
	)
}

// LogDeniedWithIP logs a denied request with IP address
func LogDeniedWithIP(reason, user, endpoint, ip string) {
	Logger.Info("denied",
		zap.String("reason", reason),
		zap.String("user", user),
		zap.String("endpoint", endpoint),
		zap.String("ip", ip),
	)
}
