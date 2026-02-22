package database

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	original := DB
	DB = mockDB

	return mock, func() {
		DB = original
		_ = mockDB.Close()
	}
}

func TestGetMaterializedViewStatsReturnsViews(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	now := time.Now()
	rows := sqlmock.NewRows([]string{"view_name", "size", "last_refresh"}).
		AddRow("public.realtime_website_stats", "10 MB", now).
		AddRow("public.daily_website_stats", "2 MB", nil)

	mock.ExpectQuery("SELECT\\s+schemaname").
		WillReturnRows(rows)

	stats, err := GetMaterializedViewStats()
	require.NoError(t, err)

	views, ok := stats["views"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, views, 2)
	assert.Equal(t, "public.realtime_website_stats", views[0]["name"])
	assert.Equal(t, "10 MB", views[0]["size"])
	assert.NotEmpty(t, views[0]["last_refresh"])
	assert.NotEmpty(t, views[0]["age"])
	assert.Equal(t, "public.daily_website_stats", views[1]["name"])
	assert.Equal(t, "2 MB", views[1]["size"])
	assert.Nil(t, views[1]["last_refresh"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMaterializedViewStatsQueryError(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	mock.ExpectQuery("SELECT\\s+schemaname").
		WillReturnError(assert.AnError)

	stats, err := GetMaterializedViewStats()
	require.Error(t, err)
	assert.Nil(t, stats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMaterializedViewSchedulerRefreshView(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	mock.ExpectExec("REFRESH MATERIALIZED VIEW CONCURRENTLY test_view").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mvs := &MaterializedViewScheduler{}
	mvs.refreshView("test_view")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMaterializedViewSchedulerRefreshViewError(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	mock.ExpectExec("REFRESH MATERIALIZED VIEW CONCURRENTLY bad_view").
		WillReturnError(assert.AnError)

	mvs := &MaterializedViewScheduler{}
	mvs.refreshView("bad_view")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewPartitionSchedulerInitializesFields(t *testing.T) {
	ps := NewPartitionScheduler("postgres://example")
	require.Equal(t, "postgres://example", ps.databaseURL)
	require.NotNil(t, ps.stopChan)
}

func TestNewMaterializedViewSchedulerInitializesStopChan(t *testing.T) {
	mvs := NewMaterializedViewScheduler()
	require.NotNil(t, mvs.stopChan)
}

func TestPartitionSchedulerCreatesFuturePartitions(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	partitionDaysAhead = 2
	nowFunc = func() time.Time {
		return time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() {
		partitionDaysAhead = 30
		nowFunc = time.Now
	})

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS website_event_2025_01_02").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS website_event_2025_01_03").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ps := &PartitionScheduler{}
	ps.createFuturePartitions()

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPartitionSchedulerCleanupOldPartitions(t *testing.T) {
	mock, cleanup := withMockDB(t)
	defer cleanup()

	retentionPeriodDays = 30
	nowFunc = func() time.Time {
		return time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() {
		retentionPeriodDays = 90
		nowFunc = time.Now
	})

	rows := sqlmock.NewRows([]string{"tablename"}).
		AddRow("website_event_2025_01_01").
		AddRow("website_event_2025_01_02")

	mock.ExpectQuery("SELECT\\s+tablename").
		WithArgs("website_event_2025_01_30").
		WillReturnRows(rows)

	mock.ExpectExec("DROP TABLE IF EXISTS website_event_2025_01_01").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP TABLE IF EXISTS website_event_2025_01_02").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ps := &PartitionScheduler{}
	ps.cleanupOldPartitions()

	require.NoError(t, mock.ExpectationsWereMet())
}
