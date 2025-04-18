package logger

import (
	"errors"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	DefaultLogLevel   = zapcore.DebugLevel
	DefaultStackLevel = zapcore.ErrorLevel
)

// NewDefaultConfig creates initial config to be used before the actual config is loaded
func NewDefaultConfig() Config {
	return Config{
		Level:      zap.NewAtomicLevelAt(DefaultLogLevel),
		StackLevel: zap.NewAtomicLevelAt(DefaultStackLevel),
	}
}

type Config struct {
	Level      AtomicLevel `json:"level" yaml:"level" env:"LOG_LEVEL"`
	StackLevel AtomicLevel `json:"stack" yaml:"stack" env:"LOG_STACK"`
}

var (
	errLevelNotInitializedError      = errors.New("logic error: logger atomic level not initialized")
	errStackLevelNotInitializedError = errors.New("logic error: logger atomic stack level not initialized")
)

func (cfg Config) validate() error {
	if IsAtomicLevelEmpty(cfg.Level) {
		return errLevelNotInitializedError
	}
	if IsAtomicLevelEmpty(cfg.StackLevel) {
		return errStackLevelNotInitializedError
	}
	return nil
}

func IsAtomicLevelEmpty(x AtomicLevel) bool {
	return x == AtomicLevel{}
}
