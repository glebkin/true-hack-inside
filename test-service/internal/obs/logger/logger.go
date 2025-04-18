package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(opts ...Option) *zap.Logger {
	logOpts := applyOptions(opts)

	zOpts := append([]zap.Option{}, logOpts.zOpts...)
	zOpts = append(zOpts,
		zap.ErrorOutput(logOpts.errorOutput),
		zap.AddStacktrace(logOpts.cfg.StackLevel),
	)

	logger := zap.New(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(newEncoderConfig()),
			logOpts.output,
			logOpts.cfg.Level,
		),
		zOpts...,
	)

	return logger
}

func newEncoderConfig() zapcore.EncoderConfig {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	encoderConfig.MessageKey = "message"
	encoderConfig.TimeKey = "timestamp"
	return encoderConfig
}
