package stats

import "time"

// ConnectionStats represents a single connection record
type ConnectionStats struct {
	ID             int64      `db:"id"`
	ClientIP       string     `db:"client_ip"`
	TargetAddr     string     `db:"target_addr"`
	Country        string     `db:"country"`
	City           string     `db:"city"`
	BytesIn        int64      `db:"bytes_in"`
	BytesOut       int64      `db:"bytes_out"`
	ConnectedAt    time.Time  `db:"connected_at"`
	DisconnectedAt *time.Time `db:"disconnected_at"`
	Duration       int64      `db:"duration"`
}

// ServerStats represents overall server statistics
type ServerStats struct {
	ID               int64     `db:"id"`
	StartTime        time.Time `db:"start_time"`
	TotalConnections int64     `db:"total_connections"`
	TotalBytesIn     int64     `db:"total_bytes_in"`
	TotalBytesOut    int64     `db:"total_bytes_out"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// GeoStats represents geographical statistics
type GeoStats struct {
	Country     string    `db:"country"`
	CountryName string    `db:"country_name"`
	Connections int64     `db:"connections"`
	TotalBytes  int64     `db:"total_bytes"`
	LastUpdated time.Time `db:"last_updated"`
}

// SpeedTestResult represents a speedtest result
type SpeedTestResult struct {
	ID               int64     `db:"id"`
	DownloadMbps     float64   `db:"download_mbps"`
	UploadMbps       float64   `db:"upload_mbps"`
	PingMs           float64   `db:"ping_ms"`
	ServerName       string    `db:"server_name"`
	ServerLocation   string    `db:"server_location"`
	TriggeredBy      string    `db:"triggered_by"`
	TriggeredIP      string    `db:"triggered_ip"`
	TriggeredCountry string    `db:"triggered_country"`
	TestedAt         time.Time `db:"tested_at"`
}

// PublicStatsResponse is the response for public statistics API
type PublicStatsResponse struct {
	UptimeSeconds     int64          `json:"uptime_seconds"`
	TotalConnections  int64          `json:"total_connections"`
	ActiveConnections int32          `json:"active_connections"`
	TotalTrafficGB    float64        `json:"total_traffic_gb"`
	Countries         []CountryStats `json:"countries"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// CountryStats represents per-country statistics
type CountryStats struct {
	Country     string  `json:"country"`
	CountryName string  `json:"country_name"`
	Connections int64   `json:"connections"`
	Percentage  float64 `json:"percentage"`
}
