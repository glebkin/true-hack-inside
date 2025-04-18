package collector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"
)

type PrometheusCollector struct {
	client v1.API
	logger *zap.Logger
}

func NewPrometheusCollector(url string, logger *zap.Logger) (*PrometheusCollector, error) {
	client, err := api.NewClient(api.Config{
		Address: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	return &PrometheusCollector{
		client: v1.NewAPI(client),
		logger: logger,
	}, nil
}

func (p *PrometheusCollector) GetAllMetrics() ([]string, error) {
	// Get all metric names
	ctx := context.Background()
	names, warnings, err := p.client.LabelValues(ctx, "__name__", nil, time.Time{}, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to get metric names: %v", err)
	}
	if len(warnings) > 0 {
		p.logger.Warn("Got warnings while fetching metrics", zap.Strings("warnings", warnings))
	}

	metrics := make([]string, 0, len(names))
	for _, name := range names {
		metrics = append(metrics, string(name))
	}

	return metrics, nil
}

func (p *PrometheusCollector) GetMetricData(metric string, startTime, endTime time.Time) (string, error) {
	// Escape dots in metric name with underscores for Prometheus query
	escapedMetric := strings.ReplaceAll(metric, ".", "_")

	// Execute query using the official API client
	r := v1.Range{
		Start: startTime,
		End:   endTime,
		Step:  15 * time.Second,
	}

	value, _, err := p.client.QueryRange(context.Background(), escapedMetric, r)
	if err != nil {
		return "", fmt.Errorf("failed to query metric: %v", err)
	}

	// Format the result
	var result strings.Builder
	switch v := value.(type) {
	case model.Vector:
		for _, sample := range v {
			result.WriteString(fmt.Sprintf("%s: %v\n", metric, sample.Value))
		}
	case model.Matrix:
		for _, stream := range v {
			result.WriteString(fmt.Sprintf("%s:\n", metric))
			for _, point := range stream.Values {
				result.WriteString(fmt.Sprintf("  %s: %v\n",
					point.Timestamp.Time().Format(time.RFC3339),
					point.Value))
			}
		}
	}

	return result.String(), nil
}

func (p *PrometheusCollector) Collect(ctx context.Context, metrics []string, start, end time.Time) (string, error) {
	var result strings.Builder

	for _, metric := range metrics {
		data, err := p.GetMetricData(metric, start, end)
		if err != nil {
			p.logger.Warn("Failed to get metric data",
				zap.String("metric", metric),
				zap.Error(err))
			continue
		}
		result.WriteString(data)
		result.WriteString("\n")
	}

	return result.String(), nil
}
