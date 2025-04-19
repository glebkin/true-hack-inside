package collector

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type JaegerCollector struct {
	logger *zap.Logger
}

func NewJaegerCollector(url string, logger *zap.Logger) (*JaegerCollector, error) {
	return &JaegerCollector{
		logger: logger,
	}, nil
}

func (c *JaegerCollector) Collect(ctx context.Context, start, end time.Time) (string, error) {
	// TODO: Implement Jaeger collector
	return "", nil
}
