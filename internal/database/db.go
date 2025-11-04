package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB initializes the SQLite database and runs migrations
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Println("[DB] Database initialized successfully")
	return db, nil
}

func runMigrations(db *sql.DB) error {
	migrations := []string{
		// connections table
		`CREATE TABLE IF NOT EXISTS connections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			client_ip TEXT NOT NULL,
			target_addr TEXT NOT NULL,
			country TEXT,
			city TEXT,
			bytes_in INTEGER NOT NULL DEFAULT 0,
			bytes_out INTEGER NOT NULL DEFAULT 0,
			connected_at DATETIME NOT NULL,
			disconnected_at DATETIME,
			duration INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connections_client_ip ON connections(client_ip)`,
		`CREATE INDEX IF NOT EXISTS idx_connections_country ON connections(country)`,
		`CREATE INDEX IF NOT EXISTS idx_connections_connected_at ON connections(connected_at)`,

		// server_stats table
		`CREATE TABLE IF NOT EXISTS server_stats (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			start_time DATETIME NOT NULL,
			total_connections INTEGER NOT NULL DEFAULT 0,
			total_bytes_in INTEGER NOT NULL DEFAULT 0,
			total_bytes_out INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// geo_stats table
		`CREATE TABLE IF NOT EXISTS geo_stats (
			country TEXT PRIMARY KEY,
			country_name TEXT,
			connections INTEGER NOT NULL DEFAULT 0,
			total_bytes INTEGER NOT NULL DEFAULT 0,
			last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// speedtest_results table
		`CREATE TABLE IF NOT EXISTS speedtest_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			download_mbps REAL NOT NULL,
			upload_mbps REAL NOT NULL,
			ping_ms REAL NOT NULL,
			server_name TEXT,
			server_location TEXT,
			triggered_by TEXT,
			triggered_ip TEXT,
			triggered_country TEXT,
			tested_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_speedtest_tested_at ON speedtest_results(tested_at DESC)`,

		// Initialize server_stats if empty
		`INSERT OR IGNORE INTO server_stats (id, start_time, total_connections, total_bytes_in, total_bytes_out)
		 VALUES (1, datetime('now'), 0, 0, 0)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// CleanupOldStats removes statistics older than retention days
func CleanupOldStats(db *sql.DB, retentionDays int) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	_, err := db.Exec(`DELETE FROM connections WHERE connected_at < ?`, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to cleanup old connections: %w", err)
	}

	log.Printf("[DB] Cleaned up connections older than %d days", retentionDays)
	return nil
}
