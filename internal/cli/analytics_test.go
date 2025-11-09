package cli

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStatsOverviewTable(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	stubWebsiteIDLookup(t, func(ctx context.Context, domain string) (string, error) {
		assert.Equal(t, "example.com", domain)
		return "site-123", nil
	})

	stubOverviewFetcher(t, func(ctx context.Context, db *sql.DB, websiteID string, days int) (*OverviewStats, error) {
		assert.Equal(t, "site-123", websiteID)
		assert.Equal(t, 7, days)
		return &OverviewStats{
			TotalVisitors:       42,
			TotalPageviews:      84,
			AvgEngagement:       15.5,
			BrowserDistribution: map[string]int64{"Chrome": 30},
			DeviceDistribution:  map[string]int64{"Desktop": 40},
			CountryDistribution: map[string]int64{"US": 25},
			TopPage: &PageStat{
				Path:      "/",
				Pageviews: 50,
			},
			TopReferrer: &ReferrerStat{
				Domain:   "google.com",
				Visitors: 20,
			},
		}, nil
	})

	output, err := captureOutput(t, func() error {
		return runStatsOverview("example.com", 7, "table")
	})
	require.NoError(t, err)
	assert.Contains(t, output, "Analytics Overview for example.com")
	assert.Contains(t, output, "Total Visitors")
	assert.Contains(t, output, "Chrome: 30")
}

func TestRunStatsOverviewInvalidDays(t *testing.T) {
	err := runStatsOverview("example.com", 0, "table")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "days must be between 1 and 365")
}

func TestRunStatsPagesCSV(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	stubWebsiteIDLookup(t, func(ctx context.Context, domain string) (string, error) {
		return "site-123", nil
	})

	stubTopPagesFetcher(t, func(ctx context.Context, db *sql.DB, websiteID string, days int, limit int) ([]*PageStat, error) {
		assert.Equal(t, 5, limit)
		return []*PageStat{
			{
				Path:           "/home",
				Pageviews:      100,
				UniqueVisitors: 80,
				BounceRate:     12.5,
				AvgTime:        45.2,
			},
		}, nil
	})

	output, err := captureOutput(t, func() error {
		return runStatsPages("example.com", 7, 5, "csv")
	})
	require.NoError(t, err)
	assert.Contains(t, output, "path,pageviews,unique_visitors")
	assert.Contains(t, output, "/home")
}

func TestRunStatsPagesInvalidTop(t *testing.T) {
	err := runStatsPages("example.com", 7, 0, "table")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "top must be between 1 and 100")
}

func TestRunStatsBreakdownJSON(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	stubWebsiteIDLookup(t, func(ctx context.Context, domain string) (string, error) {
		return "site-123", nil
	})

	stubBreakdownFetcher(t, func(ctx context.Context, db *sql.DB, websiteID string, dimension string, days int, limit int) (*BreakdownStat, error) {
		assert.Equal(t, "country", dimension)
		return &BreakdownStat{
			Dimension: "country",
			Items: []map[string]interface{}{
				{"name": "US", "visitors": 10, "pageviews": 20, "bounce_rate": 40.0},
			},
		}, nil
	})

	output, err := captureOutput(t, func() error {
		return runStatsBreakdown("example.com", "country", 7, 5, "json")
	})
	require.NoError(t, err)
	assert.Contains(t, output, `"dimension": "country"`)
	assert.Contains(t, output, "US")
}

func TestRunStatsBreakdownInvalidDimension(t *testing.T) {
	err := runStatsBreakdown("example.com", "", 7, 5, "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--by dimension is required")

	err = runStatsBreakdown("example.com", "invalid", 7, 5, "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid dimension")
}

func TestRunStatsLiveTextHandlesTickerAndSignal(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	stubWebsiteIDLookup(t, func(ctx context.Context, domain string) (string, error) {
		assert.Equal(t, "example.com", domain)
		return "site-123", nil
	})

	tickCh := make(chan time.Time)
	stopped := false
	stubTickerFactory(t, func(d time.Duration) (<-chan time.Time, func()) {
		return tickCh, func() { stopped = true }
	})

	var capturedSignal chan<- os.Signal
	stubSignalNotify(t, func(c chan<- os.Signal, sig ...os.Signal) {
		capturedSignal = c
	})

	callCh := make(chan int, 4)
	callCount := 0
	stubLiveStatsFetcher(t, func(ctx context.Context, db *sql.DB, websiteID string) (*LiveStatsData, error) {
		callCount++
		callCh <- callCount
		return &LiveStatsData{
			Timestamp:         time.Unix(int64(callCount), 0),
			ActiveVisitorsNow: int64(callCount),
			TopPageNow: &PageStat{
				Path:      "/live",
				Pageviews: int64(callCount),
			},
		}, nil
	})

	outputCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		out, err := captureOutput(t, func() error {
			return runStatsLive("example.com", 2, "text")
		})
		outputCh <- out
		errCh <- err
	}()

	<-callCh // initial fetch
	tickCh <- time.Now()
	<-callCh // update fetch

	require.Eventually(t, func() bool {
		return capturedSignal != nil
	}, time.Second, 10*time.Millisecond)

	capturedSignal <- os.Interrupt

	err := <-errCh
	output := <-outputCh

	require.NoError(t, err)
	assert.Contains(t, output, "Live stats for example.com")
	assert.Contains(t, output, "Active Visitors (last 5 min)")
	assert.True(t, stopped)
}

func stubWebsiteIDLookup(t *testing.T, fn func(ctx context.Context, domain string) (string, error)) {
	t.Helper()
	original := getWebsiteIDByDomainFn
	getWebsiteIDByDomainFn = fn
	t.Cleanup(func() {
		getWebsiteIDByDomainFn = original
	})
}

func stubOverviewFetcher(t *testing.T, fn func(context.Context, *sql.DB, string, int) (*OverviewStats, error)) {
	t.Helper()
	original := getOverviewStats
	getOverviewStats = fn
	t.Cleanup(func() {
		getOverviewStats = original
	})
}

func stubTopPagesFetcher(t *testing.T, fn func(context.Context, *sql.DB, string, int, int) ([]*PageStat, error)) {
	t.Helper()
	original := getTopPagesFn
	getTopPagesFn = fn
	t.Cleanup(func() {
		getTopPagesFn = original
	})
}

func stubBreakdownFetcher(t *testing.T, fn func(context.Context, *sql.DB, string, string, int, int) (*BreakdownStat, error)) {
	t.Helper()
	original := getBreakdownStatsFn
	getBreakdownStatsFn = fn
	t.Cleanup(func() {
		getBreakdownStatsFn = original
	})
}

func stubLiveStatsFetcher(t *testing.T, fn func(context.Context, *sql.DB, string) (*LiveStatsData, error)) {
	t.Helper()
	original := getLiveStatsFn
	getLiveStatsFn = fn
	t.Cleanup(func() {
		getLiveStatsFn = original
	})
}
