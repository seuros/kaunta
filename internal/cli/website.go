package cli

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/seuros/kaunta/internal/database"
	"github.com/spf13/cobra"
)

var websiteCmd = &cobra.Command{
	Use:   "website",
	Short: "Manage websites and tracking",
	Long: `Manage websites and tracking configuration.

Website commands allow you to manage tracked websites and their tracking settings.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(cmd.Help())
	},
}

// List command flags
var (
	listFormat string
)

var websiteListCmd = &cobra.Command{
	Use:   "list [--format json|table|csv]",
	Short: "List tracked websites",
	Long: `Display all tracked websites and their configuration.

Supported formats:
  table  - Human-readable table (default)
  json   - JSON array format
  csv    - Comma-separated values`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebsiteList(listFormat)
	},
}

// Show command flags
var (
	showFormat string
)

var websiteShowCmd = &cobra.Command{
	Use:   "show <domain>",
	Short: "Show detailed website information",
	Long: `Display detailed information about a specific website including
allowed domains, share ID, and timestamps.

Can look up by domain or website_id if domain not found.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebsiteShow(args[0], showFormat)
	},
}

// Create command flags
var (
	createName    string
	createAllowed string
)

var websiteCreateCmd = &cobra.Command{
	Use:   "create <domain> [--name <name>] [--allowed <domains-csv>]",
	Short: "Create a new tracked website",
	Long: `Create a new website for analytics tracking.

Arguments:
  domain              Domain name for the website (required, max 253 chars)

Options:
  --name              Display name for the website (defaults to domain)
  --allowed           Comma-separated list of allowed CORS domains

Examples:
  kaunta website create example.com
  kaunta website create example.com --name "My Site"
  kaunta website create example.com --allowed "example.com,www.example.com"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebsiteCreate(args[0], createName, createAllowed)
	},
}

// Update command flags
var (
	updateName    string
	updateAllowed string
)

var websiteUpdateCmd = &cobra.Command{
	Use:   "update <domain> [--name <new-name>] [--allowed <domains-csv>]",
	Short: "Update a website",
	Long: `Update the configuration of an existing website.

You can update:
  - name: Display name
  - allowed: Allowed CORS domains

Examples:
  kaunta website update example.com --name "Updated Name"
  kaunta website update example.com --allowed "example.com,new.example.com"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebsiteUpdate(args[0], updateName, updateAllowed)
	},
}

// Delete command flags
var (
	deleteForce bool
)

var websiteDeleteCmd = &cobra.Command{
	Use:   "delete <domain> [--force]",
	Short: "Delete a website (soft delete)",
	Long: `Soft delete a website (sets deleted_at timestamp).

The website data is preserved in the database but won't appear in listings.
Use --force to skip the confirmation prompt.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebsiteDelete(args[0], deleteForce)
	},
}

var websiteTrackingCodeCmd = &cobra.Command{
	Use:   "tracking-code <domain> [--format js|html|snippet]",
	Short: "Generate tracking code snippet",
	Long: `Generate a JavaScript tracking code snippet ready to embed in your website.

Supported formats:
  js       - JavaScript only (default)
  html     - Full HTML with script tags
  snippet  - HTML comment with instructions

This command outputs code that you can copy and paste into your site.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebsiteTrackingCode(args[0], trackingFormat)
	},
}

// Domain management command flags
var (
	trackingFormat string
)

var (
	fetchWebsiteByDomain  = GetWebsiteByDomain
	createWebsiteFunc     = CreateWebsite
	updateWebsiteFunc     = UpdateWebsite
	deleteWebsiteFunc     = DeleteWebsite
	addAllowedDomainsFunc = AddAllowedDomains
	removeAllowedDomainFn = RemoveAllowedDomain
	getAllowedDomainsFunc = GetAllowedDomains
	connectDatabase       = database.Connect
	closeDatabase         = database.Close
)

var websiteAddDomainCmd = &cobra.Command{
	Use:   "add-domain <website-domain> <allowed-domain> [--allowed <more-domains-csv>]",
	Short: "Add allowed CORS domains to a website",
	Long: `Add one or more CORS allowed domains to a website.

Arguments:
  website-domain     The domain of the website to update (case-insensitive)
  allowed-domain     First domain to allow (required)

Options:
  --allowed          Comma-separated list of additional domains to allow

Examples:
  kaunta website add-domain mysite.com www.mysite.com
  kaunta website add-domain mysite.com cdn.mysite.com --allowed "static.mysite.com,assets.mysite.com"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAddDomain(args[0], args[1], addDomainAllowed)
	},
}

var websiteRemoveDomainCmd = &cobra.Command{
	Use:   "remove-domain <website-domain> <allowed-domain>",
	Short: "Remove an allowed CORS domain from a website",
	Long: `Remove a CORS allowed domain from a website.

Arguments:
  website-domain     The domain of the website to update (case-insensitive)
  allowed-domain     The domain to remove

The last allowed domain cannot be removed for security reasons.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRemoveDomain(args[0], args[1])
	},
}

var websiteListDomainsCmd = &cobra.Command{
	Use:   "list-domains <website-domain> [--format json|table|text]",
	Short: "List allowed CORS domains for a website",
	Long: `Display all allowed CORS domains configured for a website.

Supported formats:
  text   - One domain per line (default)
  json   - JSON array format
  table  - Human-readable table with numbering

Examples:
  kaunta website list-domains mysite.com
  kaunta website list-domains mysite.com --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runListDomains(args[0], listDomainsFormat)
	},
}

// Domain management command flags
var (
	addDomainAllowed  string
	listDomainsFormat string
)

// Command implementations

func runWebsiteList(format string) error {
	if format == "" {
		format = "table"
	}

	// Ensure database is connected
	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	websites, err := ListWebsites(ctx)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		return outputJSON(websites)
	case "csv":
		return outputCSV(websites)
	case "table":
		return outputTable(websites)
	default:
		return fmt.Errorf("invalid format: %s", format)
	}
}

func runWebsiteShow(domain, format string) error {
	if format == "" {
		format = "table"
	}

	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	website, err := fetchWebsiteByDomain(ctx, domain, nil)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		return outputSingleJSON(website)
	case "table":
		return outputSingleTable(website)
	default:
		return fmt.Errorf("invalid format: %s", format)
	}
}

func runWebsiteCreate(domain, name, allowedCSV string) error {
	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allowedDomains := ParseAllowedDomains(allowedCSV)

	website, err := createWebsiteFunc(ctx, domain, name, allowedDomains)
	if err != nil {
		return err
	}

	fmt.Println("Website created successfully!")
	fmt.Println()
	_ = outputSingleTable(website)
	fmt.Println()
	fmt.Printf("Tracking Code ID: %s\n", website.WebsiteID)
	fmt.Println()
	fmt.Println("Next: Use 'kaunta website tracking-code <domain>' to generate the tracking snippet")

	return nil
}

func runWebsiteUpdate(domain, name, allowedCSV string) error {
	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	if name == "" && allowedCSV == "" {
		return fmt.Errorf("must specify at least one option: --name or --allowed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var namePtr *string
	if name != "" {
		namePtr = &name
	}

	var allowedDomains []string
	if allowedCSV != "" {
		allowedDomains = ParseAllowedDomains(allowedCSV)
	}

	website, err := updateWebsiteFunc(ctx, domain, namePtr, allowedDomains)
	if err != nil {
		return err
	}

	fmt.Println("Website updated successfully!")
	fmt.Println()
	_ = outputSingleTable(website)

	return nil
}

func runWebsiteDelete(domain string, force bool) error {
	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	// Confirm deletion unless --force is used
	if !force {
		fmt.Printf("Are you sure you want to delete website '%s'? (yes/no): ", domain)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		response := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if response != "yes" && response != "y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deletedAt, err := deleteWebsiteFunc(ctx, domain)
	if err != nil {
		return err
	}

	fmt.Printf("Website '%s' deleted successfully\n", domain)
	fmt.Printf("Deleted at: %s\n", deletedAt.Format(time.RFC3339))

	return nil
}

func runWebsiteTrackingCode(domain string, format string) error {
	if format == "" {
		format = "js"
	}

	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	website, err := fetchWebsiteByDomain(ctx, domain, nil)
	if err != nil {
		return err
	}

	// Generate tracking code based on format
	var trackingCode string
	switch format {
	case "js":
		trackingCode = fmt.Sprintf(`<script>
  window.kaunataConfig = { websiteId: "%s" };
</script>
<script async src="/k.js"></script>`, website.WebsiteID)
	case "html":
		trackingCode = fmt.Sprintf(`<!-- Kaunta Analytics Tracking Code -->
<script>
  window.kaunataConfig = {
    websiteId: "%s"
  };
</script>
<script async src="/k.js"></script>
<!-- End Kaunta Code -->`, website.WebsiteID)
	case "snippet":
		trackingCode = fmt.Sprintf(`<!--
  Kaunta Analytics Tracking Code

  Copy and paste the below code into the <head> section of your website.
  This code will automatically track page views and visitor interactions.

  Website ID: %s

  For installation instructions, visit: https://github.com/seuros/kaunta
-->
<script>
  window.kaunataConfig = { websiteId: "%s" };
</script>
<script async src="/k.js"></script>`, website.WebsiteID, website.WebsiteID)
	default:
		return fmt.Errorf("invalid format: %s (use js, html, or snippet)", format)
	}

	fmt.Println(trackingCode)

	return nil
}

func runAddDomain(websiteDomain, allowedDomain, additionalDomainsCSV string) error {
	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	// Validate domains
	if err := validateDomain(allowedDomain); err != nil {
		return err
	}

	additionalDomains := ParseAllowedDomains(additionalDomainsCSV)
	for _, d := range additionalDomains {
		if err := validateDomain(d); err != nil {
			return err
		}
	}

	// Collect all domains to add
	domainsToAdd := []string{allowedDomain}
	domainsToAdd = append(domainsToAdd, additionalDomains...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	website, err := addAllowedDomainsFunc(ctx, websiteDomain, domainsToAdd)
	if err != nil {
		return err
	}

	fmt.Println("Allowed domains updated successfully!")
	fmt.Println()
	fmt.Printf("Website: %s\n", website.Domain)
	fmt.Printf("Total allowed domains: %d\n", len(website.AllowedDomains))
	fmt.Println()
	fmt.Println("Allowed domains:")
	for i, d := range website.AllowedDomains {
		fmt.Printf("  %d. %s\n", i+1, d)
	}

	return nil
}

func runRemoveDomain(websiteDomain, allowedDomain string) error {
	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	website, err := removeAllowedDomainFn(ctx, websiteDomain, allowedDomain)
	if err != nil {
		return err
	}

	fmt.Println("Domain removed successfully!")
	fmt.Println()
	fmt.Printf("Website: %s\n", website.Domain)
	fmt.Printf("Remaining allowed domains: %d\n", len(website.AllowedDomains))
	fmt.Println()
	if len(website.AllowedDomains) > 0 {
		fmt.Println("Remaining allowed domains:")
		for i, d := range website.AllowedDomains {
			fmt.Printf("  %d. %s\n", i+1, d)
		}
	} else {
		fmt.Println("No allowed domains remaining.")
	}

	return nil
}

func runListDomains(websiteDomain, format string) error {
	if format == "" {
		format = "text"
	}

	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	domains, website, err := getAllowedDomainsFunc(ctx, websiteDomain)
	if err != nil {
		return err
	}

	if len(domains) == 0 {
		fmt.Printf("No allowed domains configured for '%s'\n", website.Domain)
		return nil
	}

	switch format {
	case "text":
		for _, d := range domains {
			fmt.Println(d)
		}
	case "json":
		data, err := json.MarshalIndent(domains, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	case "table":
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(w, "#\tDOMAIN\n")
		_, _ = fmt.Fprintf(w, "-\t------\n")
		for i, d := range domains {
			_, _ = fmt.Fprintf(w, "%d\t%s\n", i+1, d)
		}
		_ = w.Flush()
	default:
		return fmt.Errorf("invalid format: %s (use text, json, or table)", format)
	}

	return nil
}

// Output formatting functions

func outputJSON(websites []*WebsiteDetail) error {
	output := make([]map[string]interface{}, len(websites))
	for i, w := range websites {
		output[i] = map[string]interface{}{
			"domain":          w.Domain,
			"name":            w.Name,
			"website_id":      w.WebsiteID,
			"created_at":      w.CreatedAt,
			"updated_at":      w.UpdatedAt,
			"allowed_domains": w.AllowedDomains,
			"share_id":        w.ShareID,
		}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

func outputSingleJSON(website *WebsiteDetail) error {
	output := map[string]interface{}{
		"website_id":      website.WebsiteID,
		"domain":          website.Domain,
		"name":            website.Name,
		"created_at":      website.CreatedAt,
		"updated_at":      website.UpdatedAt,
		"allowed_domains": website.AllowedDomains,
		"share_id":        website.ShareID,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

func outputCSV(websites []*WebsiteDetail) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Write header
	err := w.Write([]string{"domain", "name", "website_id", "created_at"})
	if err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows
	for _, website := range websites {
		err := w.Write([]string{
			website.Domain,
			website.Name,
			website.WebsiteID,
			website.CreatedAt.Format(time.RFC3339),
		})
		if err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

func outputTable(websites []*WebsiteDetail) error {
	if len(websites) == 0 {
		fmt.Println("No websites found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	// Write header
	_, _ = fmt.Fprintln(w, "DOMAIN\tNAME\tWEBSITE ID\tCREATED AT")
	_, _ = fmt.Fprintln(w, "------\t----\t-----------\t----------")

	// Write rows
	for _, website := range websites {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			website.Domain,
			website.Name,
			website.WebsiteID,
			website.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}

	return nil
}

func outputSingleTable(website *WebsiteDetail) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	_, _ = fmt.Fprintf(w, "Domain:\t%s\n", website.Domain)
	_, _ = fmt.Fprintf(w, "Name:\t%s\n", website.Name)
	_, _ = fmt.Fprintf(w, "Website ID:\t%s\n", website.WebsiteID)
	_, _ = fmt.Fprintf(w, "Created:\t%s\n", website.CreatedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Updated:\t%s\n", website.UpdatedAt.Format(time.RFC3339))

	if website.ShareID != nil {
		_, _ = fmt.Fprintf(w, "Share ID:\t%s\n", *website.ShareID)
	} else {
		_, _ = fmt.Fprintf(w, "Share ID:\t(none)\n")
	}

	if len(website.AllowedDomains) > 0 {
		_, _ = fmt.Fprintf(w, "Allowed Domains:\t%s\n", strings.Join(website.AllowedDomains, ", "))
	} else {
		_, _ = fmt.Fprintf(w, "Allowed Domains:\t(none)\n")
	}

	_ = w.Flush()
	return nil
}

func init() {
	// Add subcommands to website
	websiteCmd.AddCommand(websiteListCmd)
	websiteCmd.AddCommand(websiteShowCmd)
	websiteCmd.AddCommand(websiteCreateCmd)
	websiteCmd.AddCommand(websiteUpdateCmd)
	websiteCmd.AddCommand(websiteDeleteCmd)
	websiteCmd.AddCommand(websiteTrackingCodeCmd)
	websiteCmd.AddCommand(websiteAddDomainCmd)
	websiteCmd.AddCommand(websiteRemoveDomainCmd)
	websiteCmd.AddCommand(websiteListDomainsCmd)
	// checkWebsiteCmd added in devops.go

	// List command flags
	websiteListCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, json, csv)")

	// Show command flags
	websiteShowCmd.Flags().StringVarP(&showFormat, "format", "f", "table", "Output format (table, json)")

	// Create command flags
	websiteCreateCmd.Flags().StringVarP(&createName, "name", "n", "", "Display name for the website")
	websiteCreateCmd.Flags().StringVarP(&createAllowed, "allowed", "a", "", "Comma-separated list of allowed CORS domains")

	// Update command flags
	websiteUpdateCmd.Flags().StringVarP(&updateName, "name", "n", "", "New display name for the website")
	websiteUpdateCmd.Flags().StringVarP(&updateAllowed, "allowed", "a", "", "Comma-separated list of allowed CORS domains")

	// Delete command flags
	websiteDeleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation prompt")

	// Tracking code command flags
	websiteTrackingCodeCmd.Flags().StringVarP(&trackingFormat, "format", "f", "js", "Output format (js, html, snippet)")

	// Add domain command flags
	websiteAddDomainCmd.Flags().StringVarP(&addDomainAllowed, "allowed", "a", "", "Comma-separated list of additional domains to allow")

	// List domains command flags
	websiteListDomainsCmd.Flags().StringVarP(&listDomainsFormat, "format", "f", "text", "Output format (text, json, table)")
}
