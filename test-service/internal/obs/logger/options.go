package logger

import (
	"io"
	"log"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Level = zapcore.Level
type AtomicLevel = zap.AtomicLevel

func WithConfig(cfg Config) Option {
	return func(x *options) {
		if err := cfg.validate(); err != nil {
			// Logic error
			log.Fatal(err)
		}
		x.cfg = cfg
	}
}

type options struct {
	cfg Config

	zOpts       []zap.Option
	output      zapcore.WriteSyncer
	errorOutput zapcore.WriteSyncer
}

type Option func(*options)

func applyOptions(opts []Option) options {
	x := &options{
		cfg: NewDefaultConfig(),
	}
	for _, opt := range opts {
		opt(x)
	}
	x.output = firstSyncer(x.output, lockSyncer(os.Stdout))
	x.errorOutput = firstSyncer(x.errorOutput, lockSyncer(os.Stderr))
	return *x
}

func lockSyncer(w io.Writer) zapcore.WriteSyncer {
	return zapcore.Lock(zapcore.AddSync(w))
}

func firstSyncer(ws ...zapcore.WriteSyncer) zapcore.WriteSyncer {
	for _, v := range ws {
		if v != nil {
			return v
		}
	}
	return nil
}
