package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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

type TrafficStatsResponse struct {
	TotalTrafficGB         float64 `json:"total_traffic_gb"`
	DownloadGB             float64 `json:"download_gb"`
	UploadGB               float64 `json:"upload_gb"`
	DownloadPercent        float64 `json:"download_percent"`
	UploadPercent          float64 `json:"upload_percent"`
	UptimeSeconds          int64   `json:"uptime_seconds"`
	TotalConnections       int64   `json:"total_connections"`
	ActiveConnections      int32   `json:"active_connections"`
	TrafficPerHourGB       float64 `json:"traffic_per_hour_gb"`
	TrafficPerDayGB        float64 `json:"traffic_per_day_gb"`
	TrafficPerConnectionMB float64 `json:"traffic_per_connection_mb"`
	AvgDurationSeconds     float64 `json:"avg_duration_seconds"`
}

type CountryUsage struct {
	Country     string  `json:"country"`
	CountryName string  `json:"country_name"`
	Connections int64   `json:"connections"`
	TotalBytes  int64   `json:"total_bytes"`
	Percentage  float64 `json:"percentage"`
}

type CountriesResponse struct {
	Countries        []CountryUsage `json:"countries"`
	TotalConnections int64          `json:"total_connections"`
}

type RecentConnection struct {
	Country         string    `json:"country"`
	CountryName     string    `json:"country_name"`
	City            string    `json:"city"`
	ConnectedAt     time.Time `json:"connected_at"`
	BytesIn         int64     `json:"bytes_in"`
	BytesOut        int64     `json:"bytes_out"`
	DurationSeconds int64     `json:"duration_seconds"`
}

type RecentConnectionsResponse struct {
	Connections []RecentConnection `json:"connections"`
}

type HourlyStat struct {
	Hour        string `json:"hour"`
	Connections int64  `json:"connections"`
}

type TodayStatsResponse struct {
	TotalConnections int64        `json:"total_connections"`
	TotalBytes       int64        `json:"total_bytes"`
	Hourly           []HourlyStat `json:"hourly"`
}

type DailyStat struct {
	Day         string `json:"day"`
	Connections int64  `json:"connections"`
	TotalBytes  int64  `json:"total_bytes"`
}

type WeekStatsResponse struct {
	TotalConnections int64       `json:"total_connections"`
	TotalBytes       int64       `json:"total_bytes"`
	AveragePerDay    float64     `json:"average_per_day"`
	Daily            []DailyStat `json:"daily"`
}

type PeakUsageResponse struct {
	PeakHour               string `json:"peak_hour"`
	PeakHourConnections    int64  `json:"peak_hour_connections"`
	PeakDay                string `json:"peak_day"`
	PeakDayConnections     int64  `json:"peak_day_connections"`
	BusiestCountry         string `json:"busiest_country"`
	BusiestCountryName     string `json:"busiest_country_name"`
	BusiestCountrySessions int64  `json:"busiest_country_sessions"`
}

type CompareResponse struct {
	TodayConnections     int64 `json:"today_connections"`
	TodayBytes           int64 `json:"today_bytes"`
	YesterdayConnections int64 `json:"yesterday_connections"`
	YesterdayBytes       int64 `json:"yesterday_bytes"`
	ThisWeekConnections  int64 `json:"this_week_connections"`
	ThisWeekBytes        int64 `json:"this_week_bytes"`
	LastWeekConnections  int64 `json:"last_week_connections"`
	LastWeekBytes        int64 `json:"last_week_bytes"`
}

type SearchResponse struct {
	Country          string             `json:"country"`
	CountryName      string             `json:"country_name"`
	TotalConnections int64              `json:"total_connections"`
	TotalBytes       int64              `json:"total_bytes"`
	Recent           []RecentConnection `json:"recent"`
}

type ExportResponse struct {
	Timestamp    time.Time                  `json:"timestamp"`
	Stats        *stats.PublicStatsResponse `json:"stats"`
	TopCountries []CountryUsage             `json:"top_countries"`
}

type InfoResponse struct {
	UptimeSeconds     int64         `json:"uptime_seconds"`
	ActiveConnections int32         `json:"active_connections"`
	TotalConnections  int64         `json:"total_connections"`
	TotalTrafficGB    float64       `json:"total_traffic_gb"`
	DownloadGB        float64       `json:"download_gb"`
	UploadGB          float64       `json:"upload_gb"`
	DatabaseSizeBytes int64         `json:"database_size_bytes"`
	CountriesServed   int64         `json:"countries_served"`
	TopCountry        *CountryUsage `json:"top_country,omitempty"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type ConnectionHistoryEntry struct {
	ID              int64      `json:"id"`
	ClientIP        string     `json:"client_ip"`
	TargetAddr      string     `json:"target_addr"`
	Country         string     `json:"country"`
	CountryName     string     `json:"country_name"`
	City            string     `json:"city"`
	BytesIn         int64      `json:"bytes_in"`
	BytesOut        int64      `json:"bytes_out"`
	BytesTotal      int64      `json:"bytes_total"`
	ConnectedAt     time.Time  `json:"connected_at"`
	DisconnectedAt  *time.Time `json:"disconnected_at,omitempty"`
	DurationSeconds int64      `json:"duration_seconds"`
	IsActive        bool       `json:"is_active"`
}

type ConnectionHistorySummary struct {
	TotalConnections       int64   `json:"total_connections"`
	TotalDownloadBytes     int64   `json:"total_download_bytes"`
	TotalUploadBytes       int64   `json:"total_upload_bytes"`
	TotalBytes             int64   `json:"total_bytes"`
	AverageDurationSeconds float64 `json:"average_duration_seconds"`
}

type ConnectionHistoryResponse struct {
	Connections []ConnectionHistoryEntry `json:"connections"`
	Summary     ConnectionHistorySummary `json:"summary"`
	Total       int64                    `json:"total"`
	Limit       int                      `json:"limit"`
	Offset      int                      `json:"offset"`
	HasMore     bool                     `json:"has_more"`
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
	s.mux.HandleFunc("/api/admin/stats/traffic", s.corsMiddleware(s.authMiddleware(s.handleTrafficStats)))
	s.mux.HandleFunc("/api/admin/stats/countries", s.corsMiddleware(s.authMiddleware(s.handleCountryStats)))
	s.mux.HandleFunc("/api/admin/stats/recent", s.corsMiddleware(s.authMiddleware(s.handleRecentConnections)))
	s.mux.HandleFunc("/api/admin/stats/today", s.corsMiddleware(s.authMiddleware(s.handleTodayStats)))
	s.mux.HandleFunc("/api/admin/stats/week", s.corsMiddleware(s.authMiddleware(s.handleWeekStats)))
	s.mux.HandleFunc("/api/admin/stats/peak", s.corsMiddleware(s.authMiddleware(s.handlePeakUsage)))
	s.mux.HandleFunc("/api/admin/stats/compare", s.corsMiddleware(s.authMiddleware(s.handleCompareStats)))
	s.mux.HandleFunc("/api/admin/stats/search", s.corsMiddleware(s.authMiddleware(s.handleSearchStats)))
	s.mux.HandleFunc("/api/admin/stats/export", s.corsMiddleware(s.authMiddleware(s.handleExportStats)))
	s.mux.HandleFunc("/api/admin/stats/info", s.corsMiddleware(s.authMiddleware(s.handleInfo)))

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
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	statsPayload, err := s.collector.GetPublicStats(r.Context())
	if err != nil {
		log.Printf("[API] Failed to get public stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get statistics")
		return
	}

	writeJSON(w, statsPayload)
}

// handleLatestSpeedtest returns the latest speedtest result
func (s *Server) handleLatestSpeedtest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result, err := s.speedtest.GetLatestResult(r.Context())
	if err != nil {
		log.Printf("[API] Failed to get latest speedtest: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get speedtest result")
		return
	}

	if result == nil {
		writeJSON(w, map[string]interface{}{"result": nil})
		return
	}

	writeJSON(w, result)
}

// handleSpeedtestHistory returns speedtest history
func (s *Server) handleSpeedtestHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	results, err := s.speedtest.GetHistory(r.Context(), 10)
	if err != nil {
		log.Printf("[API] Failed to get speedtest history: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get speedtest history")
		return
	}

	writeJSON(w, map[string]interface{}{"results": results})
}

// handleTriggerSpeedtest triggers a new speedtest
func (s *Server) handleTriggerSpeedtest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
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

	// Run speedtest asynchronously to avoid timeout
	go func() {
		ctx := context.Background()
		_, err := s.speedtest.RunSpeedtest(ctx, triggeredBy, clientIP)
		if err != nil {
			log.Printf("[API] Speedtest failed: %v", err)
		}
	}()

	writeJSON(w, map[string]interface{}{
		"status":  "accepted",
		"message": "Speed test started. Results will be available in about 30-60 seconds. Use the refresh button to see results.",
	})
}

// handleConnectionHistory returns connection history (placeholder)
func (s *Server) handleConnectionHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	db := s.collector.GetDB()
	queryParams := r.URL.Query()

	limit := parseLimit(queryParams.Get("limit"), 50, 200)
	offset := parseOffset(queryParams.Get("offset"))

	country := strings.ToUpper(strings.TrimSpace(queryParams.Get("country")))
	clientIP := strings.TrimSpace(queryParams.Get("client_ip"))
	target := strings.TrimSpace(queryParams.Get("target"))
	sinceParam := strings.TrimSpace(queryParams.Get("since"))
	untilParam := strings.TrimSpace(queryParams.Get("until"))

	var filters []string
	var args []interface{}

	filters = append(filters, "1=1")

	if country != "" {
		filters = append(filters, "UPPER(c.country) = ?")
		args = append(args, country)
	}

	if clientIP != "" {
		filters = append(filters, "c.client_ip LIKE ?")
		args = append(args, "%"+clientIP+"%")
	}

	if target != "" {
		filters = append(filters, "c.target_addr LIKE ?")
		args = append(args, "%"+target+"%")
	}

	if sinceParam != "" {
		since, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid since parameter: %v", err))
			return
		}
		filters = append(filters, "c.connected_at >= ?")
		args = append(args, since)
	}

	if untilParam != "" {
		until, err := time.Parse(time.RFC3339, untilParam)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid until parameter: %v", err))
			return
		}
		filters = append(filters, "c.connected_at <= ?")
		args = append(args, until)
	}

	whereClause := strings.Join(filters, " AND ")
	filterArgs := append([]interface{}(nil), args...)

	summaryQuery := fmt.Sprintf(
		`SELECT COUNT(*), COALESCE(SUM(bytes_in), 0), COALESCE(SUM(bytes_out), 0), COALESCE(AVG(duration), 0)
		 FROM connections c
		 WHERE %s`, whereClause)

	var totalConnections int64
	var totalDownload sql.NullInt64
	var totalUpload sql.NullInt64
	var avgDuration sql.NullFloat64

	if err := db.QueryRowContext(ctx, summaryQuery, filterArgs...).Scan(&totalConnections, &totalDownload, &totalUpload, &avgDuration); err != nil {
		log.Printf("[API] Failed to summarize connection history: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to fetch connection summary")
		return
	}

	summary := ConnectionHistorySummary{
		TotalConnections:   totalConnections,
		TotalDownloadBytes: totalDownload.Int64,
		TotalUploadBytes:   totalUpload.Int64,
		TotalBytes:         totalDownload.Int64 + totalUpload.Int64,
	}
	if avgDuration.Valid {
		summary.AverageDurationSeconds = avgDuration.Float64
	}

	query := fmt.Sprintf(
		`SELECT c.id,
		        c.client_ip,
		        c.target_addr,
		        c.country,
		        COALESCE(gs.country_name, c.country) AS country_name,
		        COALESCE(c.city, '') AS city,
		        c.bytes_in,
		        c.bytes_out,
		        c.connected_at,
		        c.disconnected_at,
		        c.duration
		   FROM connections c
		   LEFT JOIN geo_stats gs ON gs.country = c.country
		   WHERE %s
		   ORDER BY c.connected_at DESC
		   LIMIT ? OFFSET ?`, whereClause)

	argsWithPagination := append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, argsWithPagination...)
	if err != nil {
		log.Printf("[API] Failed to query connection history: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to fetch connection history")
		return
	}
	defer rows.Close()

	var connections []ConnectionHistoryEntry
	for rows.Next() {
		var entry ConnectionHistoryEntry
		var disconnectedAt sql.NullTime
		var duration sql.NullInt64

		if err := rows.Scan(
			&entry.ID,
			&entry.ClientIP,
			&entry.TargetAddr,
			&entry.Country,
			&entry.CountryName,
			&entry.City,
			&entry.BytesIn,
			&entry.BytesOut,
			&entry.ConnectedAt,
			&disconnectedAt,
			&duration,
		); err != nil {
			log.Printf("[API] Failed to scan connection history row: %v", err)
			respondError(w, http.StatusInternalServerError, "failed to parse connection history")
			return
		}

		if disconnectedAt.Valid {
			entry.DisconnectedAt = &disconnectedAt.Time
		} else {
			entry.IsActive = true
		}

		if duration.Valid {
			entry.DurationSeconds = duration.Int64
		} else if entry.IsActive {
			entry.DurationSeconds = int64(time.Since(entry.ConnectedAt).Seconds())
		}

		entry.BytesTotal = entry.BytesIn + entry.BytesOut

		connections = append(connections, entry)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[API] Row iteration error for connection history: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to read connection history")
		return
	}

	hasMore := summary.TotalConnections > int64(offset+len(connections))

	writeJSON(w, ConnectionHistoryResponse{
		Connections: connections,
		Summary:     summary,
		Total:       summary.TotalConnections,
		Limit:       limit,
		Offset:      offset,
		HasMore:     hasMore,
	})
}

func (s *Server) handleTrafficStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()

	publicStats, err := s.collector.GetPublicStats(ctx)
	if err != nil {
		log.Printf("[API] Failed to get public stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get traffic statistics")
		return
	}

	downloadBytes, uploadBytes, err := s.fetchServerTotals(ctx)
	if err != nil {
		log.Printf("[API] Failed to get server totals: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get traffic statistics")
		return
	}

	totalBytes := downloadBytes + uploadBytes
	totalTrafficGB := bytesToGB(totalBytes)
	downloadGB := bytesToGB(downloadBytes)
	uploadGB := bytesToGB(uploadBytes)

	trafficPerHourGB := 0.0
	trafficPerDayGB := 0.0
	if publicStats.UptimeSeconds > 0 {
		hours := float64(publicStats.UptimeSeconds) / 3600
		if hours > 0 {
			trafficPerHourGB = totalTrafficGB / hours
			trafficPerDayGB = trafficPerHourGB * 24
		}
	}

	trafficPerConnectionMB := 0.0
	if publicStats.TotalConnections > 0 {
		trafficPerConnectionMB = totalTrafficGB * 1024 / float64(publicStats.TotalConnections)
	}

	downloadPercent := 0.0
	uploadPercent := 0.0
	if totalBytes > 0 {
		downloadPercent = float64(downloadBytes) * 100 / float64(totalBytes)
		uploadPercent = float64(uploadBytes) * 100 / float64(totalBytes)
	}

	var avgDuration sql.NullFloat64
	if err := s.collector.GetDB().QueryRowContext(ctx, `SELECT AVG(duration) FROM connections WHERE duration > 0`).Scan(&avgDuration); err != nil && err != sql.ErrNoRows {
		log.Printf("[API] Failed to compute average duration: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to compute traffic statistics")
		return
	}

	writeJSON(w, TrafficStatsResponse{
		TotalTrafficGB:         totalTrafficGB,
		DownloadGB:             downloadGB,
		UploadGB:               uploadGB,
		DownloadPercent:        downloadPercent,
		UploadPercent:          uploadPercent,
		UptimeSeconds:          publicStats.UptimeSeconds,
		TotalConnections:       publicStats.TotalConnections,
		ActiveConnections:      publicStats.ActiveConnections,
		TrafficPerHourGB:       trafficPerHourGB,
		TrafficPerDayGB:        trafficPerDayGB,
		TrafficPerConnectionMB: trafficPerConnectionMB,
		AvgDurationSeconds:     avgDuration.Float64,
	})
}

func (s *Server) handleCountryStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	limit := parseLimit(r.URL.Query().Get("limit"), 50, 100)

	publicStats, err := s.collector.GetPublicStats(ctx)
	if err != nil {
		log.Printf("[API] Failed to get public stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get country statistics")
		return
	}

	countries, err := s.fetchCountryUsage(ctx, limit, publicStats.TotalConnections)
	if err != nil {
		log.Printf("[API] Failed to fetch country stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get country statistics")
		return
	}

	writeJSON(w, CountriesResponse{
		Countries:        countries,
		TotalConnections: publicStats.TotalConnections,
	})
}

func (s *Server) handleRecentConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	limit := parseLimit(r.URL.Query().Get("limit"), 10, 50)

	connections, err := s.fetchRecentConnections(ctx, limit, nil)
	if err != nil {
		log.Printf("[API] Failed to fetch recent connections: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get recent connections")
		return
	}

	writeJSON(w, RecentConnectionsResponse{Connections: connections})
}

func (s *Server) handleTodayStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	db := s.collector.GetDB()

	var totalConns, totalBytes int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now')`,
	).Scan(&totalConns, &totalBytes); err != nil {
		log.Printf("[API] Failed to get today's stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get today's statistics")
		return
	}

	rows, err := db.QueryContext(ctx,
		`SELECT strftime('%H', connected_at) as hour, COUNT(*)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now')
		 GROUP BY hour
		 ORDER BY hour DESC`)
	if err != nil {
		log.Printf("[API] Failed to get hourly stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get today's statistics")
		return
	}
	defer rows.Close()

	var hourly []HourlyStat
	for rows.Next() {
		var hour string
		var count int64
		if err := rows.Scan(&hour, &count); err == nil {
			hourly = append(hourly, HourlyStat{Hour: hour, Connections: count})
		}
	}

	writeJSON(w, TodayStatsResponse{
		TotalConnections: totalConns,
		TotalBytes:       totalBytes,
		Hourly:           hourly,
	})
}

func (s *Server) handleWeekStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	db := s.collector.GetDB()

	var totalConns, totalBytes int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-7 days')`,
	).Scan(&totalConns, &totalBytes); err != nil {
		log.Printf("[API] Failed to get weekly stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get weekly statistics")
		return
	}

	rows, err := db.QueryContext(ctx,
		`SELECT DATE(connected_at) as day, COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-7 days')
		 GROUP BY day
		 ORDER BY day DESC`)
	if err != nil {
		log.Printf("[API] Failed to get daily breakdown: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get weekly statistics")
		return
	}
	defer rows.Close()

	var daily []DailyStat
	for rows.Next() {
		var day string
		var count, bytes int64
		if err := rows.Scan(&day, &count, &bytes); err == nil {
			daily = append(daily, DailyStat{
				Day:         day,
				Connections: count,
				TotalBytes:  bytes,
			})
		}
	}

	averagePerDay := 0.0
	if totalConns > 0 {
		averagePerDay = float64(totalConns) / 7.0
	}

	writeJSON(w, WeekStatsResponse{
		TotalConnections: totalConns,
		TotalBytes:       totalBytes,
		AveragePerDay:    averagePerDay,
		Daily:            daily,
	})
}

func (s *Server) handlePeakUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	db := s.collector.GetDB()

	var peakHour string
	var peakHourCount int64
	err := db.QueryRowContext(ctx,
		`SELECT strftime('%H', connected_at) as hour, COUNT(*) as count
		 FROM connections
		 GROUP BY hour
		 ORDER BY count DESC
		 LIMIT 1`,
	).Scan(&peakHour, &peakHourCount)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[API] Failed to get peak hour: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get peak usage")
		return
	}

	var peakDay string
	var peakDayCount int64
	err = db.QueryRowContext(ctx,
		`SELECT DATE(connected_at) as day, COUNT(*) as count
		 FROM connections
		 GROUP BY day
		 ORDER BY count DESC
		 LIMIT 1`,
	).Scan(&peakDay, &peakDayCount)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[API] Failed to get peak day: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get peak usage")
		return
	}

	var busiestCountry, busiestCountryName string
	var busiestCountryCount int64
	err = db.QueryRowContext(ctx,
		`SELECT country, country_name, connections
		 FROM geo_stats
		 ORDER BY connections DESC
		 LIMIT 1`,
	).Scan(&busiestCountry, &busiestCountryName, &busiestCountryCount)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[API] Failed to get busiest country: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get peak usage")
		return
	}

	writeJSON(w, PeakUsageResponse{
		PeakHour:               peakHour,
		PeakHourConnections:    peakHourCount,
		PeakDay:                peakDay,
		PeakDayConnections:     peakDayCount,
		BusiestCountry:         busiestCountry,
		BusiestCountryName:     busiestCountryName,
		BusiestCountrySessions: busiestCountryCount,
	})
}

func (s *Server) handleCompareStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	db := s.collector.GetDB()

	var resp CompareResponse

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now')`,
	).Scan(&resp.TodayConnections, &resp.TodayBytes); err != nil {
		log.Printf("[API] Failed to get today stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get comparison statistics")
		return
	}

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE DATE(connected_at) = DATE('now', '-1 day')`,
	).Scan(&resp.YesterdayConnections, &resp.YesterdayBytes); err != nil {
		log.Printf("[API] Failed to get yesterday stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get comparison statistics")
		return
	}

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-7 days')`,
	).Scan(&resp.ThisWeekConnections, &resp.ThisWeekBytes); err != nil {
		log.Printf("[API] Failed to get this week stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get comparison statistics")
		return
	}

	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes_in + bytes_out), 0)
		 FROM connections
		 WHERE connected_at >= datetime('now', '-14 days')
		   AND connected_at < datetime('now', '-7 days')`,
	).Scan(&resp.LastWeekConnections, &resp.LastWeekBytes); err != nil {
		log.Printf("[API] Failed to get last week stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get comparison statistics")
		return
	}

	writeJSON(w, resp)
}

func (s *Server) handleSearchStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	country := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("country")))
	if country == "" {
		respondError(w, http.StatusBadRequest, "country parameter is required")
		return
	}

	ctx := r.Context()
	db := s.collector.GetDB()

	var resp SearchResponse
	resp.Country = country

	err := db.QueryRowContext(ctx,
		`SELECT country_name, connections, total_bytes
		 FROM geo_stats
		 WHERE country = ?`,
		country,
	).Scan(&resp.CountryName, &resp.TotalConnections, &resp.TotalBytes)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "no data for specified country")
		return
	}
	if err != nil {
		log.Printf("[API] Failed to fetch country stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to search statistics")
		return
	}

	recent, err := s.fetchRecentConnections(ctx, 5, &country)
	if err != nil {
		log.Printf("[API] Failed to fetch recent country connections: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to search statistics")
		return
	}
	resp.Recent = recent

	writeJSON(w, resp)
}

func (s *Server) handleExportStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()

	publicStats, err := s.collector.GetPublicStats(ctx)
	if err != nil {
		log.Printf("[API] Failed to get public stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to export statistics")
		return
	}

	topCountries, err := s.fetchCountryUsage(ctx, 10, publicStats.TotalConnections)
	if err != nil {
		log.Printf("[API] Failed to fetch top countries: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to export statistics")
		return
	}

	writeJSON(w, ExportResponse{
		Timestamp:    time.Now().UTC(),
		Stats:        publicStats,
		TopCountries: topCountries,
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	db := s.collector.GetDB()

	publicStats, err := s.collector.GetPublicStats(ctx)
	if err != nil {
		log.Printf("[API] Failed to get public stats: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get server info")
		return
	}

	downloadBytes, uploadBytes, err := s.fetchServerTotals(ctx)
	if err != nil {
		log.Printf("[API] Failed to get server totals: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get server info")
		return
	}

	var dbSizeBytes sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT page_count * page_size
		 FROM pragma_page_count(), pragma_page_size()`).
		Scan(&dbSizeBytes); err != nil && err != sql.ErrNoRows {
		log.Printf("[API] Failed to get database size: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get server info")
		return
	}

	var countriesServed sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM geo_stats WHERE connections > 0`).Scan(&countriesServed); err != nil && err != sql.ErrNoRows {
		log.Printf("[API] Failed to count countries: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get server info")
		return
	}

	var topCountry *CountryUsage
	row := db.QueryRowContext(ctx,
		`SELECT country, country_name, connections, total_bytes
		 FROM geo_stats
		 ORDER BY connections DESC
		 LIMIT 1`)
	var tc CountryUsage
	if err := row.Scan(&tc.Country, &tc.CountryName, &tc.Connections, &tc.TotalBytes); err == nil {
		if publicStats.TotalConnections > 0 {
			tc.Percentage = float64(tc.Connections) * 100 / float64(publicStats.TotalConnections)
		}
		topCountry = &tc
	} else if err != sql.ErrNoRows {
		log.Printf("[API] Failed to fetch top country: %v", err)
		respondError(w, http.StatusInternalServerError, "failed to get server info")
		return
	}

	writeJSON(w, InfoResponse{
		UptimeSeconds:     publicStats.UptimeSeconds,
		ActiveConnections: publicStats.ActiveConnections,
		TotalConnections:  publicStats.TotalConnections,
		TotalTrafficGB:    bytesToGB(downloadBytes + uploadBytes),
		DownloadGB:        bytesToGB(downloadBytes),
		UploadGB:          bytesToGB(uploadBytes),
		DatabaseSizeBytes: dbSizeBytes.Int64,
		CountriesServed:   countriesServed.Int64,
		TopCountry:        topCountry,
		UpdatedAt:         publicStats.UpdatedAt,
	})
}

// authMiddleware checks API key authorization
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("Authorization")
		if apiKey != "Bearer "+s.apiKey && apiKey != s.apiKey {
			respondError(w, http.StatusUnauthorized, "unauthorized")
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
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func (s *Server) fetchServerTotals(ctx context.Context) (int64, int64, error) {
	var totalBytesIn, totalBytesOut sql.NullInt64
	err := s.collector.GetDB().QueryRowContext(ctx,
		`SELECT total_bytes_in, total_bytes_out FROM server_stats WHERE id = 1`,
	).Scan(&totalBytesIn, &totalBytesOut)
	if err != nil {
		return 0, 0, err
	}
	return totalBytesIn.Int64, totalBytesOut.Int64, nil
}

func (s *Server) fetchCountryUsage(ctx context.Context, limit int, totalConnections int64) ([]CountryUsage, error) {
	rows, err := s.collector.GetDB().QueryContext(ctx,
		`SELECT country, country_name, connections, total_bytes
		 FROM geo_stats
		 ORDER BY connections DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var countries []CountryUsage
	for rows.Next() {
		var usage CountryUsage
		if err := rows.Scan(&usage.Country, &usage.CountryName, &usage.Connections, &usage.TotalBytes); err == nil {
			if totalConnections > 0 {
				usage.Percentage = float64(usage.Connections) * 100 / float64(totalConnections)
			}
			countries = append(countries, usage)
		}
	}
	return countries, nil
}

func (s *Server) fetchRecentConnections(ctx context.Context, limit int, countryFilter *string) ([]RecentConnection, error) {
	db := s.collector.GetDB()

	var rows *sql.Rows
	var err error
	if countryFilter != nil {
		rows, err = db.QueryContext(ctx,
			`SELECT c.country,
			        COALESCE(gs.country_name, c.country) as country_name,
			        COALESCE(c.city, ''),
			        c.connected_at,
			        c.bytes_in,
			        c.bytes_out,
			        c.duration
			 FROM connections c
			 LEFT JOIN geo_stats gs ON gs.country = c.country
			 WHERE c.disconnected_at IS NOT NULL
			   AND UPPER(c.country) = ?
			 ORDER BY c.connected_at DESC
			 LIMIT ?`, *countryFilter, limit)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT c.country,
			        COALESCE(gs.country_name, c.country) as country_name,
			        COALESCE(c.city, ''),
			        c.connected_at,
			        c.bytes_in,
			        c.bytes_out,
			        c.duration
			 FROM connections c
			 LEFT JOIN geo_stats gs ON gs.country = c.country
			 WHERE c.disconnected_at IS NOT NULL
			 ORDER BY c.connected_at DESC
			 LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connections []RecentConnection
	for rows.Next() {
		var entry RecentConnection
		if err := rows.Scan(&entry.Country, &entry.CountryName, &entry.City, &entry.ConnectedAt, &entry.BytesIn, &entry.BytesOut, &entry.DurationSeconds); err == nil {
			connections = append(connections, entry)
		}
	}

	return connections, nil
}

func parseOffset(param string) int {
	if param == "" {
		return 0
	}
	val, err := strconv.Atoi(param)
	if err != nil || val < 0 {
		return 0
	}
	return val
}

func parseLimit(param string, def, max int) int {
	if param == "" {
		return def
	}
	val, err := strconv.Atoi(param)
	if err != nil || val <= 0 {
		return def
	}
	if val > max {
		return max
	}
	return val
}

func bytesToGB(b int64) float64 {
	return float64(b) / (1024 * 1024 * 1024)
}

func writeJSON(w http.ResponseWriter, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[API] Failed to encode response: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("[API] Failed to encode error response: %v", err)
	}
}
