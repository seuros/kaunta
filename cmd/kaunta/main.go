package main

import (
	"embed"
	"strings"

	"github.com/seuros/kaunta/internal/cli"
	"github.com/seuros/kaunta/internal/logging"
	"go.uber.org/zap"
)

//go:embed VERSION
var versionFile string

//go:embed assets
var assetsFS embed.FS

//go:embed assets/kaunta.min.js
var trackerScript []byte

//go:embed assets/dist/vendor.js
var vendorJS []byte

//go:embed assets/dist/vendor.css
var vendorCSS []byte

//go:embed assets/data/countries-110m.json
var countriesGeoJSON []byte

//go:embed views
var viewsFS embed.FS

var executeCLI = cli.Execute

func run() error {
	version := strings.TrimSpace(versionFile)
	return executeCLI(
		version,
		assetsFS,
		trackerScript,
		vendorJS,
		vendorCSS,
		countriesGeoJSON,
		viewsFS,
	)
}

func main() {
	if err := run(); err != nil {
		logging.Fatal("kaunta execution failed", zap.Error(err))
	}
}
