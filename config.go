package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	configPath = "config.yml"
)

type config struct {
	Listen         string        `yaml:"listen"`
	Whitelist      []string      `yaml:"whitelist"`
	UpdateInterval time.Duration `yaml:"refresh_interval"`
	Subnet         string        `yaml:"subnet"`
	SubnetMask     int           `yaml:"subnet_mask"`

	// Stats configuration
	Stats StatsConfig `yaml:"stats"`

	// API configuration
	API APIConfig `yaml:"api"`

	// Telegram bot configuration
	Telegram TelegramConfig `yaml:"telegram"`
}

type StatsConfig struct {
	Enabled       bool   `yaml:"enabled"`
	DatabasePath  string `yaml:"database_path"`
	GeoIPPath     string `yaml:"geoip_path"`
	RetentionDays int    `yaml:"retention_days"`
}

type APIConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Listen      string   `yaml:"listen"`
	APIKey      string   `yaml:"api_key"`
	CORSOrigins []string `yaml:"cors_origins"`
}

type TelegramConfig struct {
	Enabled  bool    `yaml:"enabled"`
	BotToken string  `yaml:"bot_token"`
	AdminIDs []int64 `yaml:"admin_ids"`
}

var cfg *config

func loadConfig() error {
	// Initialize with defaults
	cfg = &config{
		Listen:         ":6666",
		UpdateInterval: time.Minute,
		Stats: StatsConfig{
			Enabled:       true,
			DatabasePath:  "./data/stats.db",
			GeoIPPath:     "./data/GeoLite2-City.mmdb",
			RetentionDays: 90,
		},
		API: APIConfig{
			Enabled: true,
			Listen:  ":8080",
		},
		Telegram: TelegramConfig{
			Enabled: false,
		},
	}

	// Load from file if exists
	f, err := os.Open(configPath)
	if err == nil {
		defer f.Close()
		if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
			return fmt.Errorf("failed to decode config file: %w", err)
		}
	}

	// Override with environment variables
	applyEnvOverrides()

	return nil
}

func applyEnvOverrides() {
	if v := os.Getenv("LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("SUBNET"); v != "" {
		cfg.Subnet = v
	}
	if v := os.Getenv("SUBNET_MASK"); v != "" {
		if mask, err := strconv.Atoi(v); err == nil {
			cfg.SubnetMask = mask
		}
	}
	if v := os.Getenv("REFRESH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.UpdateInterval = d
		}
	}

	// Stats
	if v := os.Getenv("STATS_ENABLED"); v != "" {
		cfg.Stats.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("STATS_DATABASE_PATH"); v != "" {
		cfg.Stats.DatabasePath = v
	}
	if v := os.Getenv("STATS_GEOIP_PATH"); v != "" {
		cfg.Stats.GeoIPPath = v
	}
	if v := os.Getenv("STATS_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil {
			cfg.Stats.RetentionDays = days
		}
	}

	// API
	if v := os.Getenv("API_ENABLED"); v != "" {
		cfg.API.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("API_LISTEN"); v != "" {
		cfg.API.Listen = v
	}
	if v := os.Getenv("API_KEY"); v != "" {
		cfg.API.APIKey = v
	}

	// Telegram
	if v := os.Getenv("TELEGRAM_ENABLED"); v != "" {
		cfg.Telegram.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
}
