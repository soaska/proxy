package speedtest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/soaska/proxy/internal/geoip"
)

const SpeedTestCooldown = 10 * time.Minute

// Service provides speedtest functionality
type Service struct {
	db           *sql.DB
	geoip        *geoip.Service
	notifyFunc   func(result *Result, triggeredBy, triggeredIP, triggeredCountry string)
	mu           sync.Mutex
	lastTestTime time.Time
}

// Result represents a speedtest result
type Result struct {
	ID               int64     `json:"id"`
	DownloadMbps     float64   `json:"download_mbps"`
	UploadMbps       float64   `json:"upload_mbps"`
	PingMs           float64   `json:"ping_ms"`
	ServerName       string    `json:"server_name"`
	ServerLocation   string    `json:"server_location"`
	TriggeredBy      string    `json:"triggered_by,omitempty"`
	TriggeredIP      string    `json:"triggered_ip,omitempty"`
	TriggeredCountry string    `json:"triggered_country,omitempty"`
	TestedAt         time.Time `json:"tested_at"`
}

type ooklaResult struct {
	Download struct {
		Bandwidth int64 `json:"bandwidth"` // bytes per second
	} `json:"download"`
	Upload struct {
		Bandwidth int64 `json:"bandwidth"`
	} `json:"upload"`
	Ping struct {
		Latency float64 `json:"latency"`
	} `json:"ping"`
	Server struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	} `json:"server"`
}

// NewService creates a new speedtest service
func NewService(db *sql.DB, geoipService *geoip.Service) *Service {
	return &Service{
		db:    db,
		geoip: geoipService,
	}
}

// SetNotifyCallback sets the callback for speedtest notifications
func (s *Service) SetNotifyCallback(fn func(result *Result, triggeredBy, triggeredIP, triggeredCountry string)) {
	s.notifyFunc = fn
}

// RunSpeedtest executes a speedtest
func (s *Service) RunSpeedtest(ctx context.Context, triggeredBy, triggeredIP string) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check cooldown
	if time.Since(s.lastTestTime) < SpeedTestCooldown {
		nextAllowed := s.lastTestTime.Add(SpeedTestCooldown)
		return nil, fmt.Errorf("speedtest cooldown active, next test allowed at %s", nextAllowed.Format(time.RFC3339))
	}

	// Get GeoIP info for trigger
	triggeredCountry := ""
	if s.geoip != nil && triggeredIP != "" {
		country, _, _ := s.geoip.GetLocation(triggeredIP)
		triggeredCountry = country
	}

	// Run Ookla speedtest CLI
	log.Println("[SPEEDTEST] Running speed test...")
	cmd := exec.CommandContext(ctx, "speedtest", "--accept-license", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("speedtest failed: %w", err)
	}

	var ookla ooklaResult
	if err := json.Unmarshal(output, &ookla); err != nil {
		return nil, fmt.Errorf("failed to parse speedtest result: %w", err)
	}

	// Convert to Mbps
	result := &Result{
		DownloadMbps:     float64(ookla.Download.Bandwidth) * 8 / 1_000_000,
		UploadMbps:       float64(ookla.Upload.Bandwidth) * 8 / 1_000_000,
		PingMs:           ookla.Ping.Latency,
		ServerName:       ookla.Server.Name,
		ServerLocation:   ookla.Server.Location,
		TriggeredBy:      triggeredBy,
		TriggeredIP:      triggeredIP,
		TriggeredCountry: triggeredCountry,
		TestedAt:         time.Now(),
	}

	// Save to database
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO speedtest_results 
		 (download_mbps, upload_mbps, ping_ms, server_name, server_location, 
		  triggered_by, triggered_ip, triggered_country, tested_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.DownloadMbps, result.UploadMbps, result.PingMs,
		result.ServerName, result.ServerLocation,
		result.TriggeredBy, result.TriggeredIP, result.TriggeredCountry, result.TestedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to save speedtest result: %w", err)
	}

	result.ID, _ = res.LastInsertId()
	s.lastTestTime = result.TestedAt

	log.Printf("[SPEEDTEST] Test complete: Download=%.2f Mbps, Upload=%.2f Mbps, Ping=%.2f ms",
		result.DownloadMbps, result.UploadMbps, result.PingMs)

	// Send notification to bot
	if s.notifyFunc != nil {
		go s.notifyFunc(result, triggeredBy, triggeredIP, triggeredCountry)
	}

	return result, nil
}

// GetLatestResult returns the most recent speedtest result
func (s *Service) GetLatestResult(ctx context.Context) (*Result, error) {
	var result Result
	err := s.db.QueryRowContext(ctx,
		`SELECT id, download_mbps, upload_mbps, ping_ms, server_name, server_location, tested_at
		 FROM speedtest_results
		 ORDER BY tested_at DESC
		 LIMIT 1`,
	).Scan(&result.ID, &result.DownloadMbps, &result.UploadMbps, &result.PingMs,
		&result.ServerName, &result.ServerLocation, &result.TestedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetHistory returns speedtest history
func (s *Service) GetHistory(ctx context.Context, limit int) ([]*Result, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, download_mbps, upload_mbps, ping_ms, server_name, server_location, tested_at
		 FROM speedtest_results
		 ORDER BY tested_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Result
	for rows.Next() {
		var result Result
		if err := rows.Scan(&result.ID, &result.DownloadMbps, &result.UploadMbps, &result.PingMs,
			&result.ServerName, &result.ServerLocation, &result.TestedAt); err != nil {
			continue
		}
		results = append(results, &result)
	}

	return results, nil
}
