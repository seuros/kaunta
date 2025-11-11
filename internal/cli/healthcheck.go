package cli

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var healthcheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Check if the server is healthy",
	Long:  "Performs an HTTP request to the /up endpoint to verify the server and database are operational",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Try environment variable first, then viper config, then default
		port := os.Getenv("PORT")
		if port == "" {
			port = viper.GetString("port")
		}
		if port == "" {
			port = "3000" // Default port
		}

		url := fmt.Sprintf("http://localhost:%s/up", port)

		client := &http.Client{
			Timeout: 2 * time.Second,
		}

		resp, err := client.Get(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Healthcheck failed: %v\n", err)
			return fmt.Errorf("healthcheck failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "Healthcheck failed: status %d\n", resp.StatusCode)
			return fmt.Errorf("healthcheck failed: status %d", resp.StatusCode)
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(healthcheckCmd)
}
