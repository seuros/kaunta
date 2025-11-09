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

//go:embed index.html
var indexTemplate []byte

var executeCLI = cli.Execute

func run() error {
	version := strings.TrimSpace(versionFile)
	return executeCLI(
		version,
		trackerScript,
		vendorJS,
		vendorCSS,
		countriesGeoJSON,
		dashboardTemplate,
		indexTemplate,
	)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
