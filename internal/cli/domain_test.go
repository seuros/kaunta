//go:build integration

package cli

import (
	"context"
	"testing"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/middleware"
	"github.com/seuros/kaunta/internal/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrustedOriginDatabaseFunctions(t *testing.T) {
	testDB := test.NewTestDB(t)
	defer func() { _ = testDB.Close() }()

	ctx := context.Background()

	// Override the global database connection for the test
	originalDB := database.DB
	database.DB = testDB.DB
	t.Cleanup(func() {
		database.DB = originalDB
	})

	t.Run("insert and retrieve trusted origins", func(t *testing.T) {
		// Insert test domains
		domains := []struct {
			domain      string
			description string
		}{
			{"analytics.example.com", "Main analytics domain"},
			{"dashboard.test.com", "Test dashboard"},
			{"stats.mysite.io", "Stats subdomain"},
		}

		for _, d := range domains {
			_, err := testDB.DB.ExecContext(ctx,
				"INSERT INTO trusted_origin (domain, description, is_active) VALUES ($1, $2, true)",
				d.domain, d.description,
			)
			require.NoError(t, err, "Failed to insert domain: %s", d.domain)
		}

		// Verify domains were inserted
		var count int
		err := testDB.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM trusted_origin WHERE is_active = true").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 3, count, "Should have 3 active domains")
	})

	t.Run("is_trusted_origin function validates correctly", func(t *testing.T) {
		// Clear existing data
		_, err := testDB.DB.ExecContext(ctx, "DELETE FROM trusted_origin")
		require.NoError(t, err)

		// Insert test domain
		_, err = testDB.DB.ExecContext(ctx,
			"INSERT INTO trusted_origin (domain, is_active) VALUES ($1, true)",
			"analytics.example.com",
		)
		require.NoError(t, err)

		tests := []struct {
			name     string
			origin   string
			expected bool
		}{
			{"exact match", "analytics.example.com", true},
			{"with https protocol", "https://analytics.example.com", true},
			{"with http protocol", "http://analytics.example.com", true},
			{"with port", "https://analytics.example.com:443", true},
			{"with port and path", "https://analytics.example.com:443/dashboard", true},
			{"case insensitive", "ANALYTICS.EXAMPLE.COM", true},
			{"different domain", "other.example.com", false},
			{"empty origin", "", false},
			{"null origin", "null", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var isTrusted bool
				err := testDB.DB.QueryRowContext(ctx,
					"SELECT is_trusted_origin($1)",
					tt.origin,
				).Scan(&isTrusted)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, isTrusted, "Origin: %s", tt.origin)
			})
		}
	})

	t.Run("get_trusted_origins function returns array", func(t *testing.T) {
		// Clear and insert test domains
		_, err := testDB.DB.ExecContext(ctx, "DELETE FROM trusted_origin")
		require.NoError(t, err)

		domains := []string{"domain1.com", "domain2.com", "domain3.com"}
		for _, domain := range domains {
			_, err = testDB.DB.ExecContext(ctx,
				"INSERT INTO trusted_origin (domain, is_active) VALUES ($1, true)",
				domain,
			)
			require.NoError(t, err)
		}

		// Get trusted origins using PostgreSQL function
		rows, err := testDB.DB.QueryContext(ctx, "SELECT unnest(get_trusted_origins())")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		var results []string
		for rows.Next() {
			var domain string
			err := rows.Scan(&domain)
			require.NoError(t, err)
			results = append(results, domain)
		}

		assert.Len(t, results, 3, "Should return 3 domains")
		assert.ElementsMatch(t, domains, results, "Returned domains should match inserted domains")
	})

	t.Run("inactive domains are not trusted", func(t *testing.T) {
		// Clear and insert inactive domain
		_, err := testDB.DB.ExecContext(ctx, "DELETE FROM trusted_origin")
		require.NoError(t, err)

		_, err = testDB.DB.ExecContext(ctx,
			"INSERT INTO trusted_origin (domain, is_active) VALUES ($1, false)",
			"inactive.example.com",
		)
		require.NoError(t, err)

		// Verify it's not trusted
		var isTrusted bool
		err = testDB.DB.QueryRowContext(ctx,
			"SELECT is_trusted_origin($1)",
			"https://inactive.example.com",
		).Scan(&isTrusted)
		require.NoError(t, err)
		assert.False(t, isTrusted, "Inactive domain should not be trusted")

		// Verify get_trusted_origins doesn't return it
		rows, err := testDB.DB.QueryContext(ctx, "SELECT unnest(get_trusted_origins())")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		count := 0
		for rows.Next() {
			count++
		}
		assert.Equal(t, 0, count, "Inactive domain should not be in trusted origins list")
	})
}

func TestTrustedOriginsCache(t *testing.T) {
	testDB := test.NewTestDB(t)
	defer func() { _ = testDB.Close() }()

	ctx := context.Background()

	// Override the global database connection for the test
	originalDB := database.DB
	database.DB = testDB.DB
	t.Cleanup(func() {
		database.DB = originalDB
	})

	t.Run("cache loads domains from database", func(t *testing.T) {
		// Clear and insert test domains
		_, err := testDB.DB.ExecContext(ctx, "DELETE FROM trusted_origin")
		require.NoError(t, err)

		domains := []string{"cache1.com", "cache2.com", "cache3.com"}
		for _, domain := range domains {
			_, err = testDB.DB.ExecContext(ctx,
				"INSERT INTO trusted_origin (domain, is_active) VALUES ($1, true)",
				domain,
			)
			require.NoError(t, err)
		}

		// Initialize cache
		err = middleware.InitTrustedOriginsCache()
		require.NoError(t, err)

		// Get cached origins
		origins, err := middleware.GetTrustedOrigins()
		require.NoError(t, err)
		assert.Len(t, origins, 3, "Cache should contain 3 domains")
		assert.ElementsMatch(t, domains, origins, "Cached domains should match database")
	})

	t.Run("cache handles empty database", func(t *testing.T) {
		// Clear all domains
		_, err := testDB.DB.ExecContext(ctx, "DELETE FROM trusted_origin")
		require.NoError(t, err)

		// Force cache refresh
		err = middleware.InitTrustedOriginsCache()
		require.NoError(t, err)

		// Get cached origins
		origins, err := middleware.GetTrustedOrigins()
		require.NoError(t, err)
		assert.Len(t, origins, 0, "Cache should be empty")
	})

	t.Run("cache refreshes after updates", func(t *testing.T) {
		// Start with one domain
		_, err := testDB.DB.ExecContext(ctx, "DELETE FROM trusted_origin")
		require.NoError(t, err)

		_, err = testDB.DB.ExecContext(ctx,
			"INSERT INTO trusted_origin (domain, is_active) VALUES ($1, true)",
			"initial.com",
		)
		require.NoError(t, err)

		// Initialize cache
		err = middleware.InitTrustedOriginsCache()
		require.NoError(t, err)

		origins, err := middleware.GetTrustedOrigins()
		require.NoError(t, err)
		assert.Len(t, origins, 1, "Should have 1 domain initially")

		// Add more domains
		_, err = testDB.DB.ExecContext(ctx,
			"INSERT INTO trusted_origin (domain, is_active) VALUES ($1, true), ($2, true)",
			"added1.com", "added2.com",
		)
		require.NoError(t, err)

		// Force cache refresh
		err = middleware.InitTrustedOriginsCache()
		require.NoError(t, err)

		// Verify cache updated
		origins, err = middleware.GetTrustedOrigins()
		require.NoError(t, err)
		assert.Len(t, origins, 3, "Cache should have 3 domains after refresh")
	})
}
