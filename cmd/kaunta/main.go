package main

import (
	_ "embed"
	"log"
	"strings"

	"github.com/seuros/kaunta/internal/cli"
)

//go:embed VERSION
var versionFile string

//go:embed assets/kaunta.min.js
var trackerScript []byte

//go:embed assets/dist/vendor.js
var vendorJS []byte

//go:embed assets/dist/vendor.css
var vendorCSS []byte

//go:embed assets/data/countries-110m.json
var countriesGeoJSON []byte

//go:embed dashboard.html
var dashboardTemplate []byte

func main() {
	// Extract version from embedded file
	version := strings.TrimSpace(versionFile)

	// Execute CLI with embedded assets
	if err := cli.Execute(
		version,
		trackerScript,
		vendorJS,
		vendorCSS,
		countriesGeoJSON,
		dashboardTemplate,
	); err != nil {
		log.Fatal(err)
	}
}
