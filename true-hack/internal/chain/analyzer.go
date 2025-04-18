package chain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"true-hack/internal/collector"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

type Analyzer struct {
	client     *openai.Client
	logger     *zap.Logger
	prometheus *collector.PrometheusCollector
	loki       *collector.LokiCollector
	jaeger     *collector.JaegerCollector
	config     *Config
	cache      *Cache
}

type Config struct {
	Model           string
	Temperature     float32
	MaxTokens       int
	SystemPrompt    string
	MetricsTemplate string
	LogsTemplate    string
	TracesTemplate  string
}

func NewAnalyzer(
	client *openai.Client,
	logger *zap.Logger,
	prometheus *collector.PrometheusCollector,
	loki *collector.LokiCollector,
	jaeger *collector.JaegerCollector,
	config *Config,
	cache *Cache,
) *Analyzer {
	return &Analyzer{
		client:     client,
		logger:     logger,
		prometheus: prometheus,
		loki:       loki,
		jaeger:     jaeger,
		config:     config,
		cache:      cache,
	}
}

type AnalysisRequest struct {
	Query     string
	TimeRange struct {
		Start time.Time
		End   time.Time
	}
	Metrics []string
}

type AnalysisResponse struct {
	Analysis        string
	RelevantMetrics []string
	Confidence      float32
	Suggestions     []string
}

func (a *Analyzer) Analyze(question string, startTime, endTime time.Time, metrics []string) (*LLMResponse, error) {
	// Check cache first
	cacheKey := CacheKey{
		Question:  question,
		StartTime: startTime,
		EndTime:   endTime,
		Metrics:   metrics,
	}
	if cached, ok := a.cache.Get(cacheKey); ok {
		return cached, nil
	}

	// If no metrics specified, get all available metrics
	if len(metrics) == 0 {
		allMetrics, err := a.prometheus.GetAllMetrics()
		if err != nil {
			return nil, fmt.Errorf("failed to get all metrics: %v", err)
		}
		metrics = allMetrics
		a.logger.Info("Using all available metrics", zap.Int("count", len(metrics)))
	}

	// Collect data from Prometheus
	prometheusData, err := a.collectPrometheusData(startTime, endTime, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to collect Prometheus data: %v", err)
	}

	// Create messages for chat completion
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a system metrics analyzer. Analyze the provided metrics and provide insights.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("Question: %s\n\nMetrics data:\n%s", question, prometheusData),
		},
	}

	// Send request to OpenAI
	resp, err := a.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    a.config.Model,
			Messages: messages,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat completion: %v", err)
	}

	// Parse the response
	result, err := parseLLMResponse(resp.Choices[0].Message.Content)
	if err != nil {
		a.logger.Warn("Failed to parse LLM response", zap.Error(err))
		// Fallback to simple response
		result = &LLMResponse{
			Analysis: resp.Choices[0].Message.Content,
		}
	}

	// Cache the result
	a.cache.Set(cacheKey, result)

	return result, nil
}

func (a *Analyzer) collectPrometheusData(startTime, endTime time.Time, metrics []string) (string, error) {
	// If no specific metrics are requested, get all available metrics
	if len(metrics) == 0 {
		allMetrics, err := a.prometheus.GetAllMetrics()
		if err != nil {
			return "", fmt.Errorf("failed to get all metrics: %v", err)
		}
		metrics = allMetrics
	}

	// Collect data for each metric
	var result strings.Builder
	for _, metric := range metrics {
		data, err := a.prometheus.GetMetricData(metric, startTime, endTime)
		if err != nil {
			a.logger.Warn("Failed to get metric data",
				zap.String("metric", metric),
				zap.Error(err))
			continue
		}
		result.WriteString(fmt.Sprintf("Metric: %s\n", metric))
		result.WriteString(data)
		result.WriteString("\n\n")
	}

	return result.String(), nil
}
