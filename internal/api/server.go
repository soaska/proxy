package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/soaska/proxy/internal/speedtest"
	"github.com/soaska/proxy/internal/stats"
)

// Server represents the HTTP API server
type Server struct {
	collector   *stats.StatsCollector
	speedtest   *speedtest.Service
	apiKey      string
	corsOrigins []string
	mux         *http.ServeMux
}

// NewServer creates a new API server
func NewServer(collector *stats.StatsCollector, st *speedtest.Service, apiKey string, corsOrigins []string) *Server {
	s := &Server{
		collector:   collector,
		speedtest:   st,
		apiKey:      apiKey,
		corsOrigins: corsOrigins,
		mux:         http.NewServeMux(),
	}

	// Public endpoints
	s.mux.HandleFunc("/api/stats/public", s.corsMiddleware(s.handlePublicStats))
	s.mux.HandleFunc("/api/speedtest/latest", s.corsMiddleware(s.handleLatestSpeedtest))
	s.mux.HandleFunc("/api/speedtest/history", s.corsMiddleware(s.handleSpeedtestHistory))

	// Speedtest trigger endpoint
	s.mux.HandleFunc("/api/speedtest/trigger", s.corsMiddleware(s.handleTriggerSpeedtest))

	// Private endpoints (requires API key)
	s.mux.HandleFunc("/api/admin/connections", s.corsMiddleware(s.authMiddleware(s.handleConnectionHistory)))

	log.Println("[API] API routes configured")
	return s
}

// Start starts the HTTP API server
func (s *Server) Start(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[API] Starting HTTP API server on %s", addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	return server.ListenAndServe()
}

// handlePublicStats returns public statistics
func (s *Server) handlePublicStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := s.collector.GetPublicStats(r.Context())
	if err != nil {
		log.Printf("[API] Failed to get public stats: %v", err)
		http.Error(w, "Failed to get statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleLatestSpeedtest returns the latest speedtest result
func (s *Server) handleLatestSpeedtest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result, err := s.speedtest.GetLatestResult(r.Context())
	if err != nil {
		log.Printf("[API] Failed to get latest speedtest: %v", err)
		http.Error(w, "Failed to get speedtest result", http.StatusInternalServerError)
		return
	}

	if result == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"result": nil})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleSpeedtestHistory returns speedtest history
func (s *Server) handleSpeedtestHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	results, err := s.speedtest.GetHistory(r.Context(), 10)
	if err != nil {
		log.Printf("[API] Failed to get speedtest history: %v", err)
		http.Error(w, "Failed to get speedtest history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
}

// handleTriggerSpeedtest triggers a new speedtest
func (s *Server) handleTriggerSpeedtest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get client info for notification
	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Forwarded-For")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	triggeredBy := r.URL.Query().Get("source")
	if triggeredBy == "" {
		triggeredBy = "web"
	}

	result, err := s.speedtest.RunSpeedtest(r.Context(), triggeredBy, clientIP)
	if err != nil {
		log.Printf("[API] Speedtest failed: %v", err)
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleConnectionHistory returns connection history (private endpoint)
func (s *Server) handleConnectionHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: implement pagination and filtering
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Connection history endpoint - to be implemented",
	})
}

// authMiddleware checks API key authorization
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("Authorization")
		if apiKey != "Bearer "+s.apiKey && apiKey != s.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range s.corsOrigins {
			if origin == allowedOrigin || allowedOrigin == "*" {
				allowed = true
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}

		if !allowed && len(s.corsOrigins) > 0 {
			// Default to first origin if none match
			w.Header().Set("Access-Control-Allow-Origin", s.corsOrigins[0])
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		// ALLOW ANYTHING
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
