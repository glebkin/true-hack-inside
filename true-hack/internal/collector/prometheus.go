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
		metric := string(name)
		metrics = append(metrics, metric)
		p.logger.Debug("Found metric", zap.String("metric", metric))
	}

	p.logger.Info("Found metrics",
		zap.Int("count", len(metrics)),
		zap.Strings("metrics", metrics))
	return metrics, nil
}

func (p *PrometheusCollector) GetMetricData(metric string, startTime, endTime time.Time) (string, error) {
	// Escape dots in metric name with underscores for Prometheus query
	escapedMetric := strings.ReplaceAll(metric, ".", "_")

	p.logger.Debug("Querying metric",
		zap.String("metric", metric),
		zap.String("escaped", escapedMetric),
		zap.Time("start", startTime),
		zap.Time("end", endTime))

	// For gauge metrics, we can use Query instead of QueryRange
	value, warnings, err := p.client.Query(context.Background(), escapedMetric, time.Now())
	if err != nil {
		p.logger.Error("Failed to query metric",
			zap.String("metric", metric),
			zap.String("escaped", escapedMetric),
			zap.Error(err))
		return "", fmt.Errorf("failed to query metric: %v", err)
	}
	if len(warnings) > 0 {
		p.logger.Warn("Got warnings while querying metric",
			zap.String("metric", metric),
			zap.Strings("warnings", warnings))
	}

	p.logger.Debug("Got metric response",
		zap.String("metric", metric),
		zap.String("type", fmt.Sprintf("%T", value)))

	// Format the result
	var result strings.Builder
	switch v := value.(type) {
	case model.Vector:
		p.logger.Debug("Got vector response",
			zap.String("metric", metric),
			zap.Int("samples", len(v)))
		for _, sample := range v {
			// Format labels
			labels := make([]string, 0, len(sample.Metric))
			for name, value := range sample.Metric {
				if name != "__name__" { // Skip metric name as it's already in the output
					labels = append(labels, fmt.Sprintf("%s=%s", name, value))
				}
			}
			labelStr := strings.Join(labels, ", ")
			if labelStr != "" {
				labelStr = "{" + labelStr + "}"
			}

			result.WriteString(fmt.Sprintf("%s%s: %v\n", metric, labelStr, sample.Value))
		}
	case model.Matrix:
		p.logger.Debug("Got matrix response",
			zap.String("metric", metric),
			zap.Int("streams", len(v)))
		for _, stream := range v {
			// Format labels
			labels := make([]string, 0, len(stream.Metric))
			for name, value := range stream.Metric {
				if name != "__name__" {
					labels = append(labels, fmt.Sprintf("%s=%s", name, value))
				}
			}
			labelStr := strings.Join(labels, ", ")
			if labelStr != "" {
				labelStr = "{" + labelStr + "}"
			}

			result.WriteString(fmt.Sprintf("%s%s:\n", metric, labelStr))
			for _, point := range stream.Values {
				result.WriteString(fmt.Sprintf("  %s: %v\n",
					point.Timestamp.Time().Format(time.RFC3339),
					point.Value))
			}
		}
	default:
		p.logger.Warn("Unexpected response type",
			zap.String("metric", metric),
			zap.String("type", fmt.Sprintf("%T", value)))
	}

	if result.Len() == 0 {
		p.logger.Warn("Empty result for metric",
			zap.String("metric", metric),
			zap.String("type", fmt.Sprintf("%T", value)))
	}

	return result.String(), nil
}

func (p *PrometheusCollector) Collect(ctx context.Context, metrics []string, start, end time.Time) (string, error) {
	var result strings.Builder

	p.logger.Info("Starting metrics collection",
		zap.Int("metrics_count", len(metrics)),
		zap.Time("start", start),
		zap.Time("end", end))

	for _, metric := range metrics {
		data, err := p.GetMetricData(metric, start, end)
		if err != nil {
			p.logger.Warn("Failed to get metric data",
				zap.String("metric", metric),
				zap.Error(err))
			continue
		}
		if data != "" {
			result.WriteString(data)
			result.WriteString("\n")
		}
	}

	if result.Len() == 0 {
		p.logger.Warn("No metrics data collected")
	}

	return result.String(), nil
}
