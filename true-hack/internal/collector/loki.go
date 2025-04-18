package collector

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type LokiCollector struct {
	logger *zap.Logger
}

func NewLokiCollector(url string, logger *zap.Logger) (*LokiCollector, error) {
	return &LokiCollector{
		logger: logger,
	}, nil
}

func (c *LokiCollector) Collect(ctx context.Context, start, end time.Time) (string, error) {
	// TODO: Implement Loki collector
	return "", nil
}
