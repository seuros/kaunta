package cli

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = original

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestOutputOverviewText(t *testing.T) {
	stats := &OverviewStats{
		TotalVisitors:  100,
		TotalPageviews: 250,
		AvgEngagement:  12.5,
		TopPage:        &PageStat{Path: "/home", Pageviews: 120},
		TopReferrer:    &ReferrerStat{Domain: "google.com", Visitors: 80},
		BrowserDistribution: map[string]int64{
			"Chrome":  60,
			"Firefox": 20,
		},
		DeviceDistribution: map[string]int64{
			"Desktop": 70,
			"Mobile":  30,
		},
		CountryDistribution: map[string]int64{
			"US": 80,
			"FR": 20,
		},
	}

	output := captureStdout(t, func() {
		require.NoError(t, outputOverviewText(stats, "example.com", 7))
	})

	assert.Contains(t, output, "Analytics Overview for example.com (last 7 days)")
	assert.Contains(t, output, "Total Visitors:        100")
	assert.Contains(t, output, "Chrome: 60")
	assert.Contains(t, output, "Desktop: 70")
	assert.Contains(t, output, "US: 80")
}

func TestOutputPagesCSV(t *testing.T) {
	pages := []*PageStat{
		{Path: "/home", Pageviews: 150, UniqueVisitors: 100, BounceRate: 50.0, AvgTime: 12.3},
	}

	output := captureStdout(t, func() {
		require.NoError(t, outputPagesCSV(pages))
	})

	assert.Contains(t, output, "path,pageviews,unique_visitors,bounce_rate,avg_time_seconds")
	assert.Contains(t, output, "/home,150,100,50.0,12.3")
}

func TestOutputBreakdownTable(t *testing.T) {
	stats := &BreakdownStat{
		Dimension: "country",
		Items: []map[string]any{
			{"name": "US", "visitors": 50.0, "pageviews": 120.0, "bounce_rate": 40.0},
		},
	}

	output := captureStdout(t, func() {
		require.NoError(t, outputBreakdownTable(stats))
	})

	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "US")
	assert.Contains(t, output, "40.0%")
}

func TestOutputLiveJSON(t *testing.T) {
	data := &LiveStatsData{
		Timestamp:           time.Now(),
		ActiveVisitorsNow:   5,
		PageviewsLastMinute: 20,
		RecentEvents:        3,
	}

	output := captureStdout(t, func() {
		require.NoError(t, outputLiveJSON(data))
	})

	assert.Contains(t, output, `"active_visitors_now":5`)
	assert.Contains(t, output, `"pageviews_last_minute":20`)
}

func TestOutputLiveTerm(t *testing.T) {
	data := &LiveStatsData{
		Timestamp:           time.Date(2025, time.March, 3, 10, 0, 0, 0, time.UTC),
		ActiveVisitorsNow:   8,
		PageviewsLastMinute: 16,
		RecentEvents:        4,
		TopPageNow:          &PageStat{Path: "/home", Pageviews: 3},
		RecentReferrers: []map[string]any{
			{"referrer": "google.com", "count": 2},
		},
	}

	output := captureStdout(t, func() {
		require.NoError(t, outputLiveTerm(data))
	})

	assert.Contains(t, output, "Live Analytics")
	assert.Contains(t, output, "Active Visitors (last 5 min): 8")
	assert.Contains(t, output, "Top Page Now: /home (3 pageviews)")
	assert.Contains(t, output, "google.com: 2")
}
