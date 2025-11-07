//go:build mage

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func buildAssets() error {
	fmt.Println("Installing frontend dependencies with Bun...")
	if err := sh.Run("bun", "install", "--frozen-lockfile"); err != nil {
		return err
	}
	fmt.Println("Building frontend assets with Bun...")
	return sh.Run("bun", "run", "build")
}

// Build builds Kaunta for Linux with Green Tea GC
func Build() error {
	fmt.Println("Building Kaunta for Linux with Go 1.25 + Green Tea GC...")
	if err := buildAssets(); err != nil {
		return err
	}
	env := map[string]string{
		"GOOS":         "linux",
		"GOARCH":       "amd64",
		"GOEXPERIMENT": "greenteagc",
	}
	return sh.RunWith(env, "go", "build", "-o", "kaunta-linux-amd64", "./cmd/kaunta")
}

// BuildLocal builds Kaunta for current platform
func BuildLocal() error {
	fmt.Printf("Building Kaunta for %s/%s...\n", runtime.GOOS, runtime.GOARCH)
	if err := buildAssets(); err != nil {
		return err
	}
	return sh.Run("go", "build", "-o", "kaunta", "./cmd/kaunta")
}

// Test runs tests
func Test() error {
	fmt.Println("Running tests...")
	return sh.Run("go", "test", "-v", "./...")
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning build artifacts...")
	os.Remove("kaunta")
	os.Remove("kaunta-linux-amd64")
	return nil
}

// Deploy builds and deploys to production server
func Deploy() error {
	if err := Build(); err != nil {
		return err
	}

	fmt.Println("Deploying to production...")
	server := "root@78.47.110.90"

	// Upload binary
	if err := sh.Run("scp", "kaunta-linux-amd64", server+":/usr/local/bin/kaunta-new"); err != nil {
		return err
	}

	// Restart service
	cmd := "systemctl stop kaunta && mv /usr/local/bin/kaunta /usr/local/bin/kaunta-old && mv /usr/local/bin/kaunta-new /usr/local/bin/kaunta && chmod +x /usr/local/bin/kaunta && systemctl start kaunta"
	if err := sh.Run("ssh", server, cmd); err != nil {
		return err
	}

	fmt.Println("Deployment complete!")
	return sh.Run("ssh", server, "systemctl status kaunta")
}

// Update upgrades all Go dependencies
func Update() error {
	fmt.Println("Updating dependencies...")
	if err := sh.Run("go", "get", "-u", "./..."); err != nil {
		return err
	}
	return sh.Run("go", "mod", "tidy")
}

// Fmt runs gofmt on all Go files
func Fmt() error {
	fmt.Println("Formatting code...")
	return sh.Run("go", "fmt", "./...")
}

// Vet runs go vet on all Go files
func Vet() error {
	fmt.Println("Vetting code...")
	return sh.Run("go", "vet", "./...")
}

// Bench runs benchmarks
func Bench() error {
	fmt.Println("Running benchmarks...")
	return sh.Run("go", "test", "-bench=.", "./...")
}

// Deps downloads dependencies
func Deps() error {
	fmt.Println("Downloading dependencies...")
	return sh.Run("go", "mod", "download")
}

// Tidy tidies go.mod
func Tidy() error {
	fmt.Println("Tidying go.mod...")
	return sh.Run("go", "mod", "tidy")
}

// CI runs all checks for continuous integration
func CI() error {
	if err := buildAssets(); err != nil {
		return err
	}
	mg.SerialDeps(Deps, Fmt, Vet, Test)
	fmt.Println("All CI checks passed!")
	return nil
}
