package chain

import (
	"context"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"time"

	"true-hack/internal/collector"

	"github.com/lithammer/fuzzysearch/fuzzy"
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
	gitInfo    *GitInfo
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

type GitInfo struct {
	LastCommitHash string
	LastCommitDiff string
}

func NewAnalyzer(
	client *openai.Client,
	logger *zap.Logger,
	prometheus *collector.PrometheusCollector,
	loki *collector.LokiCollector,
	jaeger *collector.JaegerCollector,
	config *Config,
	cache *Cache,
) (*Analyzer, error) {
	gitInfo, err := getGitInfo()
	if err != nil {
		logger.Warn("Failed to get git information", zap.Error(err))
		gitInfo = &GitInfo{} // Initialize with empty values
	}

	return &Analyzer{
		client:     client,
		logger:     logger,
		prometheus: prometheus,
		loki:       loki,
		jaeger:     jaeger,
		config:     config,
		cache:      cache,
		gitInfo:    gitInfo,
	}, nil
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

func (a *Analyzer) Analyze(ctx context.Context, question string, startTime, endTime time.Time, metrics []string) (*LLMResponse, error) {
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

	// Collect data from Prometheus
	prometheusData, err := a.collectPrometheusData(startTime, endTime, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to collect Prometheus data: %v", err)
	}

	// Create a more concise prompt
	userPrompt := fmt.Sprintf("Question: %s\n\nMetrics data:\n%s\n\nRecent changes:\n%s\n%s",
		question,
		strings.Join(prometheusData, ""),
		a.gitInfo.LastCommitHash,
		a.gitInfo.LastCommitDiff)

	// Create messages for chat completion
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a system metrics analyzer. Analyze the provided metrics and provide insights. Be concise and focus on key findings. Consider recent code changes when analyzing the metrics.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userPrompt,
		},
	}

	// Send request to OpenAI
	resp, err := a.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:     a.config.Model,
			Messages:  messages,
			MaxTokens: a.config.MaxTokens,
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

// estimateTokens приблизительно оценивает количество токенов в строке
// В среднем 1 токен ~ 4 символа для английского текста
func estimateTokens(text string) int {
	// Базовое количество токенов для системного промпта и вопроса
	baseTokens := 100

	// Оцениваем количество токенов в тексте
	// Учитываем, что метрики содержат много чисел и специальных символов
	tokenCount := len(text) / 3 // Более консервативная оценка

	return baseTokens + tokenCount
}

func (a *Analyzer) collectPrometheusData(startTime, endTime time.Time, metrics []string) ([]string, error) {
	// If no specific metrics are requested, get all available metrics
	if len(metrics) == 0 {
		allMetrics, err := a.prometheus.GetAllMetrics()
		if err != nil {
			return nil, fmt.Errorf("failed to get all metrics: %v", err)
		}
		metrics = allMetrics
	}

	// Filter important metrics
	importantMetrics := []string{
		"machine_cpu_cores",
		"machine_cpu_physical_cores",
		"machine_memory_bytes",
		// "process_cpu_seconds_total",
		// "process_resident_memory_bytes",
		// "container_cpu_usage_seconds_total",
		// "container_memory_usage_bytes",
		"grpc_server_handled_total",
	}

	// Create a map of important metrics for quick lookup
	importantMap := make(map[string]bool)
	for _, m := range importantMetrics {
		importantMap[m] = true
	}

	// Максимальное количество токенов для входных данных
	// Оставляем место для системного промпта и ответа
	maxInputTokens := 20000

	// Collect data for each metric, prioritizing important ones
	var result []string
	var totalTokens int

	for _, metric := range metrics {
		// Пропускаем неважные метрики, если уже набрали достаточно данных
		if totalTokens >= maxInputTokens && !importantMap[metric] {
			continue
		}

		data, err := a.prometheus.GetMetricData(metric, startTime, endTime)
		if err != nil {
			a.logger.Warn("Failed to get metric data",
				zap.String("metric", metric),
				zap.Error(err))
			continue
		}
		if data == "" {
			continue
		}

		// Оцениваем количество токенов для новой метрики
		metricData := fmt.Sprintf("Metric: %s\n", data)
		metricTokens := estimateTokens(metricData)

		// Если добавление этой метрики превысит лимит, пропускаем её
		if totalTokens+metricTokens > maxInputTokens && !importantMap[metric] {
			continue
		}

		// Для важных метрик добавляем в начало
		if importantMap[metric] {
			result = append([]string{metricData}, result...)
		} else {
			result = append(result, metricData)
		}

		totalTokens += metricTokens
	}

	a.logger.Debug("Collected metrics data",
		zap.Int("total_metrics", len(result)),
		zap.Int("estimated_tokens", totalTokens))

	return result, nil
}

func findBestMatch(userQuery string, data []string) []string {
	matches := map[int][]string{}
	for _, target := range data {
		rank := fuzzy.RankMatchNormalizedFold(userQuery, target)
		matches[rank] = append(matches[rank], target)
	}

	ranks := slices.Collect(maps.Keys(matches))
	slices.Sort(ranks)

	var result []string
	for _, rank := range slices.Backward(ranks) {
		result = append(result, matches[rank]...)
	}

	return result
}

func getGitInfo() (*GitInfo, error) {
	// Get last commit hash and message
	cmd := exec.Command("git", "log", "-1", "--pretty=format:%H %s")
	commitBytes, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git commit info: %v", err)
	}
	commitInfo := strings.TrimSpace(string(commitBytes))

	// Get last commit diff, but only for relevant files
	cmd = exec.Command("git", "diff", "HEAD~1", "--", "*.go", "*.yaml", "*.json", "*.md")
	diffBytes, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git diff: %v", err)
	}
	diff := string(diffBytes)

	// If diff is too long, truncate it
	if len(diff) > 2000 {
		diff = diff[:2000] + "\n... (truncated)"
	}

	return &GitInfo{
		LastCommitHash: commitInfo,
		LastCommitDiff: diff,
	}, nil
}
