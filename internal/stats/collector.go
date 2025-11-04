package stats

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/soaska/proxy/internal/geoip"
)

// StatsCollector collects and manages connection statistics
type StatsCollector struct {
	db              *sql.DB
	geoip           *geoip.Service
	activeConns     sync.Map // map[uint64]*ConnectionTracker
	serverStartTime time.Time
	retentionDays   int

	// Atomic counters for fast access
	activeCount atomic.Int32
	totalConns  atomic.Int64
}

// NewStatsCollector creates a new statistics collector
func NewStatsCollector(db *sql.DB, geoipService *geoip.Service, retentionDays int) *StatsCollector {
	if retentionDays < 0 {
		retentionDays = 0
	}

	sc := &StatsCollector{
		db:              db,
		geoip:           geoipService,
		serverStartTime: time.Now(),
		retentionDays:   retentionDays,
	}

	// Initialize server_stats if needed
	sc.initServerStats()

	// Start background cleanup
	go sc.cleanupLoop()

	log.Println("[STATS] Stats collector initialized")
	return sc
}

// TrackConnection creates a new connection tracker
func (sc *StatsCollector) TrackConnection(ctx context.Context, clientIP, targetAddr string) *ConnectionTracker {
	// Get GeoIP info
	country := "Unknown"
	city := ""
	if sc.geoip != nil {
		var err error
		country, city, err = sc.geoip.GetLocation(clientIP)
		if err != nil {
			log.Printf("[STATS] Failed to get geo location for %s: %v", clientIP, err)
			country = "Unknown"
			city = ""
		}
	}

	// Create connection record
	connectedAt := time.Now()
	result, err := sc.db.ExecContext(ctx,
		`INSERT INTO connections (client_ip, target_addr, country, city, connected_at)
		 VALUES (?, ?, ?, ?, ?)`,
		clientIP, targetAddr, country, city, connectedAt,
	)
	if err != nil {
		log.Printf("[STATS] Failed to insert connection: %v", err)
		return nil
	}

	connID, _ := result.LastInsertId()

	// Update counters
	sc.activeCount.Add(1)
	sc.totalConns.Add(1)
	sc.updateServerStats(1, 0, 0)
	sc.updateGeoStats(country, 0, true)

	// Create tracker
	tracker := &ConnectionTracker{
		id:        uint64(connID),
		collector: sc,
		country:   country,
		startTime: connectedAt,
	}

	sc.activeConns.Store(tracker.id, tracker)

	log.Printf("[STATS] New connection: %s -> %s (Country: %s, City: %s)",
		clientIP, targetAddr, country, city)

	return tracker
}

// GetPublicStats returns public statistics for API
func (sc *StatsCollector) GetPublicStats(ctx context.Context) (*PublicStatsResponse, error) {
	var serverStats ServerStats
	err := sc.db.QueryRowContext(ctx,
		`SELECT start_time, total_connections, total_bytes_in, total_bytes_out, updated_at
		 FROM server_stats WHERE id = 1`,
	).Scan(&serverStats.StartTime, &serverStats.TotalConnections,
		&serverStats.TotalBytesIn, &serverStats.TotalBytesOut, &serverStats.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get server stats: %w", err)
	}

	// Calculate uptime
	uptime := time.Since(sc.serverStartTime).Seconds()

	// Calculate total traffic in GB
	totalBytes := serverStats.TotalBytesIn + serverStats.TotalBytesOut
	totalTrafficGB := float64(totalBytes) / (1024 * 1024 * 1024)

	// Get geo statistics
	rows, err := sc.db.QueryContext(ctx,
		`SELECT country, country_name, connections, total_bytes
		 FROM geo_stats
		 ORDER BY connections DESC
		 LIMIT 20`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get geo stats: %w", err)
	}
	defer rows.Close()

	var countries []CountryStats
	var totalConnsForPercent int64

	// First pass: collect data and calculate total
	for rows.Next() {
		var cs CountryStats
		if err := rows.Scan(&cs.Country, &cs.CountryName, &cs.Connections, new(int64)); err != nil {
			continue
		}
		countries = append(countries, cs)
		totalConnsForPercent += cs.Connections
	}

	// Second pass: calculate percentages
	if totalConnsForPercent > 0 {
		for i := range countries {
			countries[i].Percentage = float64(countries[i].Connections) * 100.0 / float64(totalConnsForPercent)
		}
	}

	return &PublicStatsResponse{
		UptimeSeconds:     int64(uptime),
		TotalConnections:  serverStats.TotalConnections,
		ActiveConnections: sc.activeCount.Load(),
		TotalTrafficGB:    totalTrafficGB,
		Countries:         countries,
		UpdatedAt:         time.Now(),
	}, nil
}

// GetActiveConnections returns the number of active connections
func (sc *StatsCollector) GetActiveConnections() int32 {
	return sc.activeCount.Load()
}

// initServerStats initializes or updates server_stats table
func (sc *StatsCollector) initServerStats() {
	_, err := sc.db.Exec(
		`INSERT OR IGNORE INTO server_stats (id, start_time, total_connections, total_bytes_in, total_bytes_out)
		 VALUES (1, ?, 0, 0, 0)`,
		sc.serverStartTime,
	)
	if err != nil {
		log.Printf("[STATS] Failed to initialize server stats: %v", err)
	}
}

// updateServerStats updates server statistics
func (sc *StatsCollector) updateServerStats(connDelta int64, bytesIn, bytesOut int64) {
	_, err := sc.db.Exec(
		`UPDATE server_stats 
		 SET total_connections = total_connections + ?,
		     total_bytes_in = total_bytes_in + ?,
		     total_bytes_out = total_bytes_out + ?,
		     updated_at = datetime('now')
		 WHERE id = 1`,
		connDelta, bytesIn, bytesOut,
	)
	if err != nil {
		log.Printf("[STATS] Failed to update server stats: %v", err)
	}
}

// updateGeoStats updates geographical statistics
func (sc *StatsCollector) updateGeoStats(country string, bytes int64, incrementConnections bool) {
	if country == "" || country == "Unknown" {
		return
	}

	countryName := country
	if sc.geoip != nil {
		countryName = sc.geoip.GetCountryName(country)
	}

	connDelta := int64(0)
	if incrementConnections {
		connDelta = 1
	}

	_, err := sc.db.Exec(
		`INSERT INTO geo_stats (country, country_name, connections, total_bytes, last_updated)
		 VALUES (?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(country) DO UPDATE SET
		     connections = connections + ?,
		     total_bytes = total_bytes + ?,
		     last_updated = datetime('now')`,
		country, countryName, connDelta, bytes, connDelta, bytes,
	)
	if err != nil {
		log.Printf("[STATS] Failed to update geo stats: %v", err)
	}
}

// cleanupLoop runs periodic cleanup tasks
func (sc *StatsCollector) cleanupLoop() {
	if sc.retentionDays <= 0 {
		log.Println("[STATS] Retention policy disabled; skipping cleanup loop")
		return
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	sc.cleanupExpiredConnections()

	for range ticker.C {
		sc.cleanupExpiredConnections()
	}
}

func (sc *StatsCollector) cleanupExpiredConnections() {
	cutoff := time.Now().AddDate(0, 0, -sc.retentionDays)
	if _, err := sc.db.Exec(`DELETE FROM connections WHERE connected_at < ?`, cutoff); err != nil {
		log.Printf("[STATS] Failed to cleanup old connections: %v", err)
		return
	}
	log.Printf("[STATS] Old connections cleaned up (retention=%d days)", sc.retentionDays)
}

// Close gracefully closes the stats collector
func (sc *StatsCollector) Close() {
	log.Println("[STATS] Closing stats collector...")
	// Close all active trackers
	sc.activeConns.Range(func(key, value interface{}) bool {
		if tracker, ok := value.(*ConnectionTracker); ok {
			tracker.Close(context.Background())
		}
		return true
	})
}
