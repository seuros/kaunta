package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contriboss/go-update"
)

// Updater handles downloading and applying updates.
type Updater struct {
	httpClient *http.Client
}

// NewUpdater creates a new Updater instance.
func NewUpdater() *Updater {
	return &Updater{
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// UpdateTo downloads and applies the update from the given URL.
// The URL may be a .tar.gz archive (containing the executable) or a raw platform binary.
func (u *Updater) UpdateTo(downloadURL, targetPath string) error {
	// Download the archive
	resp, err := u.httpClient.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	// Read the entire response into memory for extraction
	archiveData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read archive: %w", err)
	}

	// Determine binary format: tar.gz archive or raw binary
	var binary []byte
	if looksLikeGzip(archiveData) || strings.HasSuffix(strings.ToLower(downloadURL), ".tar.gz") {
		b, exErr := extractBinary(archiveData, "kaunta")
		if exErr != nil {
			// Fallback: if archive extraction fails, assume raw binary
			binary = archiveData
		} else {
			binary = b
		}
	} else {
		binary = archiveData
	}

	// Apply the update using go-update
	opts := update.Options{
		TargetPath: targetPath,
	}

	// Check permissions before attempting update
	if err := opts.CheckPermissions(); err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}

	if err := update.Apply(bytes.NewReader(binary), opts); err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			return fmt.Errorf("update failed and rollback also failed: %w (rollback error: %v)", err, rerr)
		}
		return fmt.Errorf("update failed: %w", err)
	}

	return nil
}

// UpdateTo downloads and applies the update using a default Updater.
func UpdateTo(downloadURL, targetPath string) error {
	return NewUpdater().UpdateTo(downloadURL, targetPath)
}

func looksLikeGzip(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b
}

// extractBinary extracts the named binary from a tar.gz archive.
func extractBinary(archiveData []byte, binaryName string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Look for the binary file
		name := filepath.Base(header.Name)
		if header.Typeflag == tar.TypeReg && (name == binaryName || strings.HasPrefix(name, binaryName)) {
			// Skip if it's clearly not an executable (has extension other than none)
			ext := filepath.Ext(name)
			if ext != "" && ext != ".exe" {
				continue
			}

			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("failed to read binary from archive: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("binary %q not found in archive", binaryName)
}

// CheckPermissions verifies the current process can update the target executable.
func CheckPermissions(targetPath string) error {
	if targetPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
		targetPath = exe
	}

	opts := update.Options{TargetPath: targetPath}
	return opts.CheckPermissions()
}
