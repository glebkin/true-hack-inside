package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"true-hack/internal/chain"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type Server struct {
	analyzer *chain.Analyzer
	logger   *zap.Logger
	router   *mux.Router
}

type AnalyzeRequest struct {
	Question  string   `json:"question"`
	StartTime string   `json:"start_time"`
	EndTime   string   `json:"end_time"`
	Metrics   []string `json:"metrics"`
}

func NewServer(analyzer *chain.Analyzer, logger *zap.Logger) *Server {
	s := &Server{
		analyzer: analyzer,
		logger:   logger,
		router:   mux.NewRouter(),
	}

	s.router.HandleFunc("/api/v1/analyze", s.handleAnalyze).Methods("POST")
	s.router.HandleFunc("/api/v1/metrics", s.handleMetrics).Methods("GET")
	s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("static")))

	return s
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("Failed to decode request", zap.Error(err))
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		http.Error(w, "Invalid start time format", http.StatusBadRequest)
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		http.Error(w, "Invalid end time format", http.StatusBadRequest)
		return
	}

	result, err := s.analyzer.Analyze(r.Context(), req.Question, startTime, endTime, req.Metrics)
	if err != nil {
		s.logger.Error("Failed to analyze", zap.Error(err))
		http.Error(w, fmt.Sprintf("Analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement metrics list endpoint
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics": []string{
			"process_cpu_seconds_total",
			"process_resident_memory_bytes",
			"http_requests_total",
		},
	})
}

func (s *Server) Start(port int) error {
	s.logger.Info("Starting server", zap.Int("port", port))
	return http.ListenAndServe(":"+strconv.Itoa(port), s.router)
}
