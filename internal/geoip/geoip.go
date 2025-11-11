package geoip

import (
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/oschwald/geoip2-golang"

	"github.com/seuros/kaunta/internal/logging"
)

var (
	reader *geoip2.Reader
	dbPath string
)

// Init initializes the GeoIP database
// Downloads GeoLite2-City if not present locally (optional - warns if missing)
func Init(dataDir string) error {
	dbPath = filepath.Join(dataDir, "GeoLite2-City.mmdb")

	// Download if missing
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		logging.L().Info("geoip database not found; attempting download", "path", dbPath)
		if err := downloadDatabase(dbPath); err != nil {
			logging.L().Warn("geoip database download failed", "error", err)
			logging.L().Warn("geoip lookups will return 'Unknown' until database is installed manually")
			logging.L().Info("download GeoIP from https://geoip.maxmind.com/ and place file", "path", dbPath)
			// Don't fail - continue without GeoIP
			return nil
		}
		logging.L().Info("geoip database downloaded successfully")
	}

	// Open database
	var err error
	reader, err = geoip2.Open(dbPath)
	if err != nil {
		logging.L().Warn("could not load geoip database", "error", err)
		logging.L().Warn("geoip lookups will return 'Unknown'")
		// Don't fail - continue without GeoIP
		return nil
	}

	logging.L().Info("geoip database loaded")
	return nil
}

// LookupIP returns country, city, and region for an IP address
func LookupIP(ipStr string) (country, city, region string) {
	if reader == nil {
		return "", "", ""
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", "", ""
	}

	record, err := reader.City(ip)
	if err != nil {
		logging.L().Warn("geoip lookup error", "ip", ipStr, "error", err)
		return "", "", ""
	}

	country = record.Country.IsoCode
	// Keep country empty if not found (don't use "Unknown" - session.country is CHAR(2))

	city = record.City.Names["en"]

	// Handle subdivisions safely - only access if present
	if len(record.Subdivisions) > 0 {
		region = record.Subdivisions[0].Names["en"]
	}

	return country, city, region
}

// Close closes the GeoIP database
func Close() error {
	if reader != nil {
		return reader.Close()
	}
	return nil
}

// downloadDatabase downloads GeoLite2-City database from jsDelivr CDN
// Using the geolite2-city package mirror hosted by jsDelivr
func downloadDatabase(dbPath string) error {
	// Create directory if needed
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Use jsDelivr CDN mirror of geolite2-city
	// Source: https://www.npmjs.com/package/geolite2-city
	url := "https://cdn.jsdelivr.net/npm/geolite2-city/GeoLite2-City.mmdb.gz"

	logging.L().Info("downloading geoip database", "url", url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.L().Warn("failed to close geoip response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Decompress gzip stream
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		if err := gzReader.Close(); err != nil {
			logging.L().Warn("failed to close geoip gzip reader", "error", err)
		}
	}()

	// Write to file
	out, err := os.Create(dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			logging.L().Warn("failed to close geoip output file", "error", err)
		}
	}()

	if _, err := io.Copy(out, gzReader); err != nil {
		return fmt.Errorf("failed to write database: %w", err)
	}

	return nil
}
