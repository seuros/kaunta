package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/models"
)

var apikeyCmd = &cobra.Command{
	Use:   "apikey",
	Short: "Manage API keys for server-side ingestion",
	Long: `Manage API keys for the server-side analytics ingestion API.

API keys allow backends (Rails, Node, Python, etc.) to push analytics events
programmatically via POST /api/ingest.`,
}

var apikeyCreateCmd = &cobra.Command{
	Use:   "create <website-domain>",
	Short: "Create a new API key for a website",
	Long: `Create a new API key for server-side event ingestion.

The full API key is displayed ONCE on creation. Save it securely - it cannot be retrieved later.

Examples:
  kaunta apikey create example.com
  kaunta apikey create example.com --name "Rails Backend"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAPIKeyCreate(args[0])
	},
}

var apikeyListCmd = &cobra.Command{
	Use:   "list <website-domain>",
	Short: "List API keys for a website",
	Long: `List all API keys for a website, including revoked keys.

Examples:
  kaunta apikey list example.com
  kaunta apikey list example.com --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAPIKeyList(args[0])
	},
}

var apikeyRevokeCmd = &cobra.Command{
	Use:   "revoke <key-id-or-prefix>",
	Short: "Revoke an API key",
	Long: `Revoke an API key by its ID or prefix.

Revoked keys immediately stop working. This action cannot be undone.

Examples:
  kaunta apikey revoke kaunta_live_abc
  kaunta apikey revoke 550e8400-e29b-41d4-a716-446655440000`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAPIKeyRevoke(args[0])
	},
}

var apikeyShowCmd = &cobra.Command{
	Use:   "show <key-id-or-prefix>",
	Short: "Show details of an API key",
	Long: `Display detailed information about an API key.

Examples:
  kaunta apikey show kaunta_live_abc
  kaunta apikey show 550e8400-e29b-41d4-a716-446655440000`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAPIKeyShow(args[0])
	},
}

// Command flags
var (
	apikeyName       string
	apikeyListFormat string
)

func runAPIKeyCreate(websiteDomain string) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find website by domain
	website, err := GetWebsiteByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return fmt.Errorf("website not found: %w", err)
	}

	websiteID, err := uuid.Parse(website.WebsiteID)
	if err != nil {
		return fmt.Errorf("invalid website ID: %w", err)
	}

	// Create API key
	var namePtr *string
	if apikeyName != "" {
		namePtr = &apikeyName
	}

	result, err := models.GenerateAPIKey(websiteID, nil, namePtr)
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	fmt.Println()
	fmt.Println("API Key created successfully!")
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("IMPORTANT: Save this key now. It will NOT be shown again.")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("API Key: %s\n", result.FullKey)
	fmt.Println()
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("Key ID:     %s\n", result.APIKey.KeyID)
	fmt.Printf("Website:    %s (%s)\n", website.Domain, website.WebsiteID)
	if result.APIKey.Name != nil {
		fmt.Printf("Name:       %s\n", *result.APIKey.Name)
	}
	fmt.Printf("Scopes:     %s\n", strings.Join(result.APIKey.Scopes, ", "))
	fmt.Printf("Rate Limit: %d req/min\n", result.APIKey.RateLimitPerMinute)
	fmt.Printf("Created:    %s\n", result.APIKey.CreatedAt.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("Usage example:")
	fmt.Println()
	fmt.Printf("  curl -X POST https://your-kaunta-host/api/ingest \\\n")
	fmt.Printf("    -H \"Authorization: Bearer %s\" \\\n", result.FullKey)
	fmt.Printf("    -H \"Content-Type: application/json\" \\\n")
	fmt.Printf("    -d '{\"event\": \"page_view\", \"visitor_id\": \"anon_123\", \"url\": \"/products\"}'\n")
	fmt.Println()

	return nil
}

func runAPIKeyList(websiteDomain string) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find website by domain
	website, err := GetWebsiteByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return fmt.Errorf("website not found: %w", err)
	}

	websiteID, err := uuid.Parse(website.WebsiteID)
	if err != nil {
		return fmt.Errorf("invalid website ID: %w", err)
	}

	// List API keys
	keys, err := models.ListAPIKeys(websiteID)
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	if len(keys) == 0 {
		fmt.Printf("No API keys found for website '%s'\n", websiteDomain)
		fmt.Println()
		fmt.Println("Create one with: kaunta apikey create", websiteDomain)
		return nil
	}

	fmt.Printf("\nAPI Keys for %s (%d total)\n\n", websiteDomain, len(keys))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PREFIX\tNAME\tSTATUS\tLAST USED\tCREATED")
	_, _ = fmt.Fprintln(w, "------\t----\t------\t---------\t-------")

	for _, key := range keys {
		name := "-"
		if key.Name != nil {
			name = *key.Name
		}

		status := "active"
		if key.RevokedAt != nil {
			status = "revoked"
		} else if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
			status = "expired"
		}

		lastUsed := "never"
		if key.LastUsedAt != nil {
			lastUsed = key.LastUsedAt.Format("2006-01-02 15:04")
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			key.KeyPrefix,
			name,
			status,
			lastUsed,
			key.CreatedAt.Format("2006-01-02 15:04"),
		)
	}

	_ = w.Flush()
	fmt.Println()

	return nil
}

func runAPIKeyRevoke(keyIDOrPrefix string) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	// Try parsing as UUID first
	if keyID, err := uuid.Parse(keyIDOrPrefix); err == nil {
		if err := models.RevokeAPIKey(keyID); err != nil {
			return fmt.Errorf("failed to revoke API key: %w", err)
		}
		fmt.Printf("API key %s revoked successfully\n", keyID)
		return nil
	}

	// Try as prefix
	if err := models.RevokeAPIKeyByPrefix(keyIDOrPrefix); err != nil {
		return fmt.Errorf("failed to revoke API key with prefix '%s': %w", keyIDOrPrefix, err)
	}

	fmt.Printf("API key with prefix '%s' revoked successfully\n", keyIDOrPrefix)
	return nil
}

func runAPIKeyShow(keyIDOrPrefix string) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	var key *models.APIKey
	var err error

	// Try parsing as UUID first
	if keyID, parseErr := uuid.Parse(keyIDOrPrefix); parseErr == nil {
		key, err = models.GetAPIKeyByID(keyID)
	} else {
		// Try as prefix - need to query by prefix
		key, err = getAPIKeyByPrefix(keyIDOrPrefix)
	}

	if err != nil {
		return fmt.Errorf("API key not found: %w", err)
	}

	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintf(w, "Key ID:\t%s\n", key.KeyID)
	_, _ = fmt.Fprintf(w, "Prefix:\t%s\n", key.KeyPrefix)
	_, _ = fmt.Fprintf(w, "Website ID:\t%s\n", key.WebsiteID)

	if key.Name != nil {
		_, _ = fmt.Fprintf(w, "Name:\t%s\n", *key.Name)
	} else {
		_, _ = fmt.Fprintf(w, "Name:\t(none)\n")
	}

	_, _ = fmt.Fprintf(w, "Scopes:\t%s\n", strings.Join(key.Scopes, ", "))
	_, _ = fmt.Fprintf(w, "Rate Limit:\t%d req/min\n", key.RateLimitPerMinute)

	status := "active"
	if key.RevokedAt != nil {
		status = fmt.Sprintf("revoked (%s)", key.RevokedAt.Format(time.RFC3339))
	} else if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		status = fmt.Sprintf("expired (%s)", key.ExpiresAt.Format(time.RFC3339))
	}
	_, _ = fmt.Fprintf(w, "Status:\t%s\n", status)

	_, _ = fmt.Fprintf(w, "Created:\t%s\n", key.CreatedAt.Format(time.RFC3339))

	if key.LastUsedAt != nil {
		_, _ = fmt.Fprintf(w, "Last Used:\t%s\n", key.LastUsedAt.Format(time.RFC3339))
	} else {
		_, _ = fmt.Fprintf(w, "Last Used:\tnever\n")
	}

	if key.ExpiresAt != nil {
		_, _ = fmt.Fprintf(w, "Expires:\t%s\n", key.ExpiresAt.Format(time.RFC3339))
	}

	_ = w.Flush()
	fmt.Println()

	return nil
}

// getAPIKeyByPrefix finds an API key by its prefix
func getAPIKeyByPrefix(prefix string) (*models.APIKey, error) {
	query := `
		SELECT
			key_id,
			website_id,
			created_by,
			key_prefix,
			name,
			scopes,
			rate_limit_per_minute,
			created_at,
			last_used_at,
			revoked_at,
			expires_at
		FROM api_keys
		WHERE key_prefix = $1
	`

	var key models.APIKey
	var createdByNull, nameNull *string
	var scopes []string
	var lastUsedAt, revokedAt, expiresAt *time.Time

	err := database.DB.QueryRow(query, prefix).Scan(
		&key.KeyID,
		&key.WebsiteID,
		&createdByNull,
		&key.KeyPrefix,
		&nameNull,
		&scopes,
		&key.RateLimitPerMinute,
		&key.CreatedAt,
		&lastUsedAt,
		&revokedAt,
		&expiresAt,
	)
	if err != nil {
		return nil, err
	}

	if createdByNull != nil {
		id, _ := uuid.Parse(*createdByNull)
		key.CreatedBy = &id
	}
	key.Name = nameNull
	key.Scopes = scopes
	key.LastUsedAt = lastUsedAt
	key.RevokedAt = revokedAt
	key.ExpiresAt = expiresAt

	return &key, nil
}

func init() {
	// Create command flags
	apikeyCreateCmd.Flags().StringVarP(&apikeyName, "name", "n", "", "Friendly name for the API key (e.g., 'Rails Backend')")

	// List command flags
	apikeyListCmd.Flags().StringVarP(&apikeyListFormat, "format", "f", "table", "Output format (table, json)")

	// Add subcommands
	apikeyCmd.AddCommand(apikeyCreateCmd)
	apikeyCmd.AddCommand(apikeyListCmd)
	apikeyCmd.AddCommand(apikeyRevokeCmd)
	apikeyCmd.AddCommand(apikeyShowCmd)

	// Register with root command
	RootCmd.AddCommand(apikeyCmd)
}
