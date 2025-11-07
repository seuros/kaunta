package cli

import (
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Kaunta analytics server",
	Long: `Start the Kaunta analytics server.

The serve command starts the web server that runs the Kaunta analytics platform.
It requires the DATABASE_URL environment variable to be set.

Environment variables:
  DATABASE_URL  PostgreSQL connection string (required)
  PORT          Server port (default: 3000)
  DATA_DIR      GeoIP database directory (default: ./data)

Example:
  DATABASE_URL="postgres://user:pass@localhost/kaunta" kaunta serve`,
	RunE: func(cmd *cobra.Command, args []string) error {
	return serveAnalytics(
		TrackerScript,
		VendorJS,
		VendorCSS,
		CountriesGeoJSON,
		DashboardTemplate,
	)
	},
}
