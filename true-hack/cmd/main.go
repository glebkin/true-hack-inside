package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
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

	// Read API key from .token_key
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}
	projectRoot := filepath.Dir(wd)
	tokenPath := filepath.Join(projectRoot, ".token_key")

	apiKeyBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		log.Fatalf("Failed to read API key file from %s: %v", tokenPath, err)
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
		SystemPrompt:    "You are a system metrics analyzer. Analyze the provided metrics and provide insights.",
		MetricsTemplate: "Metrics data for time range from {{.StartTime}} to {{.EndTime}}:\n{{.Data}}",
		LogsTemplate:    "Logs for time range from {{.StartTime}} to {{.EndTime}}:\n{{.Data}}",
		TracesTemplate:  "Traces for time range from {{.StartTime}} to {{.EndTime}}:\n{{.Data}}",
	}

	// Initialize analyzer
	analyzer := chain.NewAnalyzer(
		openaiClient,
		logger,
		prometheusCollector,
		nil, // Loki collector
		nil, // Jaeger collector
		analyzerConfig,
		cache,
	)

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
