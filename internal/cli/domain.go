package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage trusted domains for dashboard access",
	Long: `Manage trusted origins for multi-domain dashboard access.

Trusted domains allow users to access the Kaunta dashboard through custom
CNAMEs (e.g., analytics.example.com) while maintaining shared authentication
across all domains.`,
}

var domainAddCmd = &cobra.Command{
	Use:   "add <domain>",
	Short: "Add a trusted domain",
	Long: `Add a domain to the list of trusted origins for dashboard access.

The domain should be provided without protocol (no http:// or https://).
Port numbers will be automatically handled during validation.

Examples:
  kaunta domain add analytics.example.com
  kaunta domain add dashboard.mysite.com --description "Main analytics dashboard"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cleanDomain, err := config.SanitizeTrustedDomain(args[0])
		if err != nil {
			return err
		}

		// Connect to database
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()

		// Check if domain already exists
		var exists bool
		err = database.DB.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM trusted_origin WHERE lower(domain) = $1)",
			cleanDomain,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check existing domain: %w", err)
		}
		if exists {
			return fmt.Errorf("domain '%s' already exists", cleanDomain)
		}

		// Get description
		description, _ := cmd.Flags().GetString("description")

		// Insert domain
		query := `
			INSERT INTO trusted_origin (domain, description, is_active)
			VALUES ($1, NULLIF($2, ''), true)
			RETURNING id, domain, description, is_active, created_at
		`

		var result struct {
			ID          int
			Domain      string
			Description *string
			IsActive    bool
			CreatedAt   string
		}

		err = database.DB.QueryRow(query, cleanDomain, description).Scan(
			&result.ID,
			&result.Domain,
			&result.Description,
			&result.IsActive,
			&result.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to add domain: %w", err)
		}

		fmt.Printf("\n✓ Trusted domain added successfully\n")
		fmt.Printf("  ID:     %d\n", result.ID)
		fmt.Printf("  Domain: %s\n", result.Domain)
		if result.Description != nil && *result.Description != "" {
			fmt.Printf("  Desc:   %s\n", *result.Description)
		}
		fmt.Printf("  Active: %v\n", result.IsActive)
		fmt.Printf("  Added:  %s\n", result.CreatedAt)
		fmt.Println("\nNote: Changes take effect within 5 minutes (cache TTL)")

		return nil
	},
}

var domainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all trusted domains",
	Long: `List all trusted domains for dashboard access.

Shows both active and inactive domains. Use --active flag to show only active domains.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Connect to database
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()

		// Build query
		query := `SELECT id, domain, description, is_active, created_at, updated_at
		          FROM trusted_origin `

		activeOnly, _ := cmd.Flags().GetBool("active")
		if activeOnly {
			query += "WHERE is_active = true "
		}

		query += "ORDER BY created_at DESC"

		rows, err := database.DB.Query(query)
		if err != nil {
			return fmt.Errorf("failed to list domains: %w", err)
		}
		defer func() { _ = rows.Close() }()

		var domains []struct {
			ID          int
			Domain      string
			Description *string
			IsActive    bool
			CreatedAt   string
			UpdatedAt   string
		}

		for rows.Next() {
			var domain struct {
				ID          int
				Domain      string
				Description *string
				IsActive    bool
				CreatedAt   string
				UpdatedAt   string
			}
			if err := rows.Scan(
				&domain.ID,
				&domain.Domain,
				&domain.Description,
				&domain.IsActive,
				&domain.CreatedAt,
				&domain.UpdatedAt,
			); err != nil {
				return fmt.Errorf("failed to scan domain: %w", err)
			}
			domains = append(domains, domain)
		}

		if err = rows.Err(); err != nil {
			return fmt.Errorf("error iterating domains: %w", err)
		}

		if len(domains) == 0 {
			if activeOnly {
				fmt.Println("No active trusted domains found")
			} else {
				fmt.Println("No trusted domains found")
			}
			fmt.Println("\nAdd a domain with: kaunta domain add <domain>")
			return nil
		}

		fmt.Printf("\nTotal domains: %d\n\n", len(domains))
		fmt.Printf("%-4s  %-8s  %-40s  %-30s  %s\n", "ID", "Active", "Domain", "Description", "Created")
		fmt.Println(strings.Repeat("-", 120))

		for _, domain := range domains {
			status := "✓"
			if !domain.IsActive {
				status = "✗"
			}

			desc := "-"
			if domain.Description != nil && *domain.Description != "" {
				desc = *domain.Description
				if len(desc) > 28 {
					desc = desc[:25] + "..."
				}
			}

			fmt.Printf("%-4d  %-8s  %-40s  %-30s  %s\n",
				domain.ID,
				status,
				domain.Domain,
				desc,
				domain.CreatedAt,
			)
		}

		return nil
	},
}

var domainRemoveCmd = &cobra.Command{
	Use:   "remove <domain>",
	Short: "Remove a trusted domain",
	Long: `Remove a domain from the list of trusted origins.

This will permanently delete the domain. Users will no longer be able to
access the dashboard from this domain.

Examples:
  kaunta domain remove analytics.example.com
  kaunta domain remove 1  # Can also use domain ID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := strings.ToLower(strings.TrimSpace(args[0]))

		// Connect to database
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()

		// Build query to match either ID or domain
		var domainName string
		err := database.DB.QueryRow(
			"SELECT domain FROM trusted_origin WHERE id::text = $1 OR lower(domain) = $1",
			identifier,
		).Scan(&domainName)
		if err != nil {
			return fmt.Errorf("domain '%s' not found", identifier)
		}

		// Confirm deletion
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Remove trusted domain '%s'? (yes/no): ", domainName)
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.ToLower(strings.TrimSpace(response))

			if response != "yes" && response != "y" {
				fmt.Println("Removal cancelled")
				return nil
			}
		}

		// Delete domain
		result, err := database.DB.Exec(
			"DELETE FROM trusted_origin WHERE id::text = $1 OR lower(domain) = $1",
			identifier,
		)
		if err != nil {
			return fmt.Errorf("failed to remove domain: %w", err)
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("domain '%s' not found", identifier)
		}

		fmt.Printf("✓ Trusted domain '%s' removed successfully\n", domainName)
		fmt.Println("Note: Changes take effect within 5 minutes (cache TTL)")

		return nil
	},
}

var domainToggleCmd = &cobra.Command{
	Use:   "toggle <domain>",
	Short: "Toggle domain active status",
	Long: `Enable or disable a trusted domain without removing it.

This allows you to temporarily disable access from a domain without
permanently deleting it from the database.

Examples:
  kaunta domain toggle analytics.example.com
  kaunta domain toggle 1  # Can also use domain ID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := strings.ToLower(strings.TrimSpace(args[0]))

		// Connect to database
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()

		// Toggle active status
		var domain string
		var newStatus bool
		err := database.DB.QueryRow(`
			UPDATE trusted_origin
			SET is_active = NOT is_active,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id::text = $1 OR lower(domain) = $1
			RETURNING domain, is_active
		`, identifier).Scan(&domain, &newStatus)

		if err != nil {
			return fmt.Errorf("domain '%s' not found", identifier)
		}

		status := "enabled"
		if !newStatus {
			status = "disabled"
		}

		fmt.Printf("✓ Domain '%s' %s successfully\n", domain, status)
		fmt.Println("Note: Changes take effect within 5 minutes (cache TTL)")

		return nil
	},
}

var domainVerifyCmd = &cobra.Command{
	Use:   "verify <origin>",
	Short: "Verify if an origin is trusted",
	Long: `Test if a given origin URL is in the trusted domains list.

Useful for debugging CSRF issues or verifying domain configuration.

Examples:
  kaunta domain verify https://analytics.example.com
  kaunta domain verify http://localhost:3000`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		origin := args[0]

		// Connect to database
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()

		// Use the PostgreSQL function to validate
		var isTrusted bool
		err := database.DB.QueryRow("SELECT is_trusted_origin($1)", origin).Scan(&isTrusted)
		if err != nil {
			return fmt.Errorf("failed to verify origin: %w", err)
		}

		if isTrusted {
			fmt.Printf("✓ Origin '%s' is TRUSTED\n", origin)
		} else {
			fmt.Printf("✗ Origin '%s' is NOT TRUSTED\n", origin)
			fmt.Println("\nAdd the domain with: kaunta domain add <domain>")
		}

		return nil
	},
}

func init() {
	// Add flags
	domainAddCmd.Flags().StringP("description", "d", "", "Description of the domain")
	domainListCmd.Flags().Bool("active", false, "Show only active domains")
	domainRemoveCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// Add subcommands
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainListCmd)
	domainCmd.AddCommand(domainRemoveCmd)
	domainCmd.AddCommand(domainToggleCmd)
	domainCmd.AddCommand(domainVerifyCmd)

	// Register with root command
	RootCmd.AddCommand(domainCmd)
}
