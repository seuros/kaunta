package cli

import (
	"database/sql"
	"io"
	"os"
	"testing"

	"github.com/seuros/kaunta/internal/database"
	"github.com/stretchr/testify/require"
)

func captureOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fnErr := fn()

	_ = w.Close()
	os.Stdout = originalStdout

	output, readErr := io.ReadAll(r)
	require.NoError(t, readErr)
	_ = r.Close()

	return string(output), fnErr
}

func stubDB(t *testing.T) {
	t.Helper()
	originalDB := database.DB
	database.DB = new(sql.DB)
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func stubConnectClose(t *testing.T) {
	t.Helper()
	originalConnect := connectDatabase
	originalClose := closeDatabase
	connectDatabase = func() error { return nil }
	closeDatabase = func() error { return nil }
	t.Cleanup(func() {
		connectDatabase = originalConnect
		closeDatabase = originalClose
	})
}
