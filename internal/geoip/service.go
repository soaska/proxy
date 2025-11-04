package geoip

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

// Service provides GeoIP lookup functionality
type Service struct {
	reader *geoip2.Reader
	mu     sync.RWMutex
}

// NewService creates a new GeoIP service
func NewService(dbPath string) (*Service, error) {
	reader, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GeoIP database: %w", err)
	}

	log.Printf("[GeoIP] GeoIP database loaded from %s", dbPath)
	return &Service{
		reader: reader,
	}, nil
}

// GetLocation returns the country and city for an IP address
func (s *Service) GetLocation(ipStr string) (country, city string, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", "", fmt.Errorf("invalid IP address: %s", ipStr)
	}

	record, err := s.reader.City(ip)
	if err != nil {
		return "", "", fmt.Errorf("GeoIP lookup failed: %w", err)
	}

	country = record.Country.IsoCode
	if len(record.City.Names) > 0 {
		// Prefer English name
		if name, ok := record.City.Names["en"]; ok {
			city = name
		} else {
			// Fallback to first available name
			for _, name := range record.City.Names {
				city = name
				break
			}
		}
	}

	return country, city, nil
}

// GetCountryName returns the full country name for a country code
func (s *Service) GetCountryName(countryCode string) string {
	// Simple mapping for common countries
	countryNames := map[string]string{
		"RU": "Russia",
		"US": "United States",
		"DE": "Germany",
		"GB": "United Kingdom",
		"FR": "France",
		"NL": "Netherlands",
		"CN": "China",
		"JP": "Japan",
		"KR": "South Korea",
		"IN": "India",
		"BR": "Brazil",
		"CA": "Canada",
		"AU": "Australia",
		"IT": "Italy",
		"ES": "Spain",
		"PL": "Poland",
		"UA": "Ukraine",
		"TR": "Turkey",
		"SE": "Sweden",
		"NO": "Norway",
	}

	if name, ok := countryNames[countryCode]; ok {
		return name
	}
	return countryCode
}

// Close closes the GeoIP database
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.reader != nil {
		return s.reader.Close()
	}
	return nil
}
