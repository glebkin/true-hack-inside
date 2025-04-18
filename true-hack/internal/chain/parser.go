package chain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type LLMResponse struct {
	Analysis    string   `json:"analysis"`
	Confidence  float32  `json:"confidence"`
	Suggestions []string `json:"suggestions"`
	Metrics     []string `json:"relevant_metrics"`
}

func parseLLMResponse(response string) (*LLMResponse, error) {
	// Try to parse as JSON first
	var llmResp LLMResponse
	if err := json.Unmarshal([]byte(response), &llmResp); err == nil {
		return &llmResp, nil
	}

	// If not JSON, try to parse using regex
	resp := &LLMResponse{
		Analysis:    response,
		Confidence:  0.8, // Default confidence
		Suggestions: []string{},
		Metrics:     []string{},
	}

	// Extract confidence if present
	confidenceRegex := regexp.MustCompile(`confidence:?\s*([0-9.]+)`)
	if matches := confidenceRegex.FindStringSubmatch(response); len(matches) > 1 {
		fmt.Sscanf(matches[1], "%f", &resp.Confidence)
	}

	// Extract suggestions
	suggestionRegex := regexp.MustCompile(`suggestion:?\s*([^\n]+)`)
	resp.Suggestions = suggestionRegex.FindAllString(response, -1)
	for i, s := range resp.Suggestions {
		resp.Suggestions[i] = strings.TrimSpace(strings.TrimPrefix(s, "suggestion:"))
	}

	// Extract metrics
	metricRegex := regexp.MustCompile(`metric:?\s*([^\n]+)`)
	resp.Metrics = metricRegex.FindAllString(response, -1)
	for i, m := range resp.Metrics {
		resp.Metrics[i] = strings.TrimSpace(strings.TrimPrefix(m, "metric:"))
	}

	return resp, nil
}
