package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"true-hack/internal/chain"
	"true-hack/internal/collector"
	"true-hack/internal/server"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Prometheus struct {
		URL string `yaml:"url"`
	} `yaml:"prometheus"`
	Jaeger struct {
		URL string `yaml:"url"`
	} `yaml:"jaeger"`
	OpenAI struct {
		Model   string `yaml:"model"`
		BaseURL string `yaml:"base_url"`
	} `yaml:"openai"`
}

func main() {
	// Read config
	configFile, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(configFile, &config); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	apiKeyBytes, err := os.ReadFile(".token_key")
	if err != nil {
		log.Fatalf("Failed to read API key file: %v", err)
	}
	apiKey := "Bearer " + strings.TrimSpace(string(apiKeyBytes))

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Initialize collectors
	prometheusCollector, err := collector.NewPrometheusCollector(config.Prometheus.URL, logger)
	if err != nil {
		logger.Fatal("Failed to initialize Prometheus collector", zap.Error(err))
	}

	jaegerCollector, err := collector.NewJaegerCollector(config.Jaeger.URL, logger)
	if err != nil {
		logger.Fatal("Failed to initialize Jaeger collector", zap.Error(err))
	}

	// Initialize OpenAI client with custom base URL
	openaiConfig := openai.DefaultConfig(apiKey)
	openaiConfig.BaseURL = config.OpenAI.BaseURL
	openaiClient := openai.NewClientWithConfig(openaiConfig)

	// Initialize cache
	cache := chain.NewCache(30 * time.Minute)

	// Initialize analyzer config
	analyzerConfig := &chain.Config{
		Model:           config.OpenAI.Model,
		Temperature:     0.7,
		MaxTokens:       2000,
		SystemPrompt:    "You are an experienced SRE/DevOps engineer analyzing system metrics. Provide concise, actionable insights focusing on critical issues and potential improvements. Be direct and technical, avoiding unnecessary explanations. Format: [SEVERITY] Issue: Brief description. Action: Specific recommendation.",
		MetricsTemplate: "Metrics data for time range from {{.StartTime}} to {{.EndTime}}:\n{{.Data}}",
		LogsTemplate:    "Logs for time range from {{.StartTime}} to {{.EndTime}}:\n{{.Data}}",
		TracesTemplate:  "Traces for time range from {{.StartTime}} to {{.EndTime}}:\n{{.Data}}",
	}

	// Initialize analyzer
	analyzer, err := chain.NewAnalyzer(
		openaiClient,
		logger,
		prometheusCollector,
		nil, // Loki collector
		jaegerCollector,
		analyzerConfig,
		cache,
	)
	if err != nil {
		logger.Fatal("Failed to initialize analyzer", zap.Error(err))
	}

	// Initialize server
	server := server.NewServer(analyzer, logger)

	// Start server in a goroutine
	go func() {
		if err := server.Start(config.Server.Port); err != nil {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down server...")
}
