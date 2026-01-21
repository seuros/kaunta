//go:build !docker

package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/blang/semver"
	"github.com/seuros/kaunta/internal/selfupdate"
	"github.com/spf13/cobra"
)

var (
	selfUpgradeRequested bool
	selfUpgradeCheckOnly bool
	selfUpgradeAutoYes   bool
)

func setupSelfUpgrade() {
	RootCmd.PersistentFlags().BoolVar(&selfUpgradeRequested, "self-upgrade", false, "Upgrade Kaunta to the latest release and exit")
	RootCmd.PersistentFlags().BoolVar(
		&selfUpgradeCheckOnly, "self-upgrade-check", false,
		"Only check whether a newer Kaunta release is available")
	RootCmd.PersistentFlags().BoolVar(&selfUpgradeAutoYes, "self-upgrade-yes", false, "Skip confirmation prompts when running --self-upgrade")

	existingPreRun := RootCmd.PersistentPreRunE
	RootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Handle self-upgrade FIRST, before any config/database initialization
		if err := handleSelfUpgradeFlags(); err != nil {
			return err
		}

		// Only run existing pre-run if we didn't exit from self-upgrade
		if existingPreRun != nil {
			if err := existingPreRun(cmd, args); err != nil {
				return err
			}
		}

		return nil
	}
}

// hideSelfUpgradeFlagsIfDevBuild hides self-upgrade flags when running dev builds
func hideSelfUpgradeFlagsIfDevBuild() {
	if strings.TrimSpace(Version) == "" {
		_ = RootCmd.PersistentFlags().MarkHidden("self-upgrade")
		_ = RootCmd.PersistentFlags().MarkHidden("self-upgrade-check")
		_ = RootCmd.PersistentFlags().MarkHidden("self-upgrade-yes")
	}
}

func handleSelfUpgradeFlags() error {
	// Self-upgrade is flag-only (no config) to avoid accidental auto-upgrades
	if !selfUpgradeRequested && !selfUpgradeCheckOnly {
		return nil
	}

	if err := runSelfUpgrade(selfUpgradeCheckOnly, selfUpgradeAutoYes); err != nil {
		return err
	}

	os.Exit(0)
	return nil
}

func runSelfUpgrade(checkOnly, autoYes bool) error {
	versionStr := strings.TrimSpace(strings.TrimPrefix(Version, "v"))
	if versionStr == "" {
		return errors.New("self-upgrade is only available for release builds")
	}

	current, err := semver.Parse(versionStr)
	if err != nil {
		return fmt.Errorf("invalid current version %q: %w", Version, err)
	}

	fmt.Printf("Checking current version... v%s\n", current)

	fmt.Print("Checking latest released version... ")
	client := selfupdate.NewClient("seuros", "kaunta")
	latest, err := client.DetectLatest()
	if err != nil {
		fmt.Println()
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVer := latest.Version
	fmt.Printf("v%s\n", latestVer)

	if !latestVer.GT(current) {
		fmt.Println("Kaunta is already up to date")
		return nil
	}

	fmt.Printf("New release found! v%s --> v%s\n", current, latestVer)
	if checkOnly {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}

	fmt.Println()
	fmt.Println("Kaunta release status:")
	fmt.Printf("  * Current exe: %q\n", exe)
	fmt.Printf("  * Target OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	// Find the appropriate asset for this platform
	asset, err := latest.FindAsset()
	if err != nil {
		return fmt.Errorf("failed to find download: %w", err)
	}
	if asset.BrowserDownloadURL != "" {
		fmt.Printf("  * Download URL: %s\n", asset.BrowserDownloadURL)
	}
	fmt.Println()

	if !autoYes {
		fmt.Println("The new release will download and replace the current binary.")
		fmt.Print("Do you want to continue? [Y/n] ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		response = strings.ToLower(strings.TrimSpace(response))
		if response != "" && response != "y" && response != "yes" {
			fmt.Println("Update cancelled.")
			return nil
		}
	}

	fmt.Println("Downloading release...")
	if err := selfupdate.UpdateTo(asset.BrowserDownloadURL, exe); err != nil {
		return fmt.Errorf("self-upgrade failed: %w", err)
	}

	fmt.Printf("Updated Kaunta to v%s\n", latestVer)
	return nil
}
