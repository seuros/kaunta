package models

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasAnyUsers_TableDoesNotExist(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Mock query for checking if users table exists
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(rows)

	// Test
	hasUsers, err := HasAnyUsers(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, hasUsers)

	// Verify expectations
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestHasAnyUsers_TableExistsNoUsers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Mock query for checking if users table exists
	tableRows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery("SELECT EXISTS.*information_schema").WillReturnRows(tableRows)

	// Mock query for checking if any users exist
	userRows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery("SELECT EXISTS.*FROM users").WillReturnRows(userRows)

	// Test
	hasUsers, err := HasAnyUsers(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, hasUsers)

	// Verify expectations
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestHasAnyUsers_TableExistsWithUsers(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Mock query for checking if users table exists
	tableRows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery("SELECT EXISTS.*information_schema").WillReturnRows(tableRows)

	// Mock query for checking if any users exist
	userRows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery("SELECT EXISTS.*FROM users").WillReturnRows(userRows)

	// Test
	hasUsers, err := HasAnyUsers(context.Background(), db)
	require.NoError(t, err)
	assert.True(t, hasUsers)

	// Verify expectations
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestHasAnyUsers_DatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Mock query to return an error
	mock.ExpectQuery("SELECT EXISTS").WillReturnError(sql.ErrConnDone)

	// Test
	hasUsers, err := HasAnyUsers(context.Background(), db)
	assert.Error(t, err)
	assert.False(t, hasUsers)

	// Verify expectations
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestHasAnyUsers_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Mock query for checking if users table exists
	tableRows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery("SELECT EXISTS.*information_schema").WillReturnRows(tableRows)

	// Mock query for users to return error (e.g., permission issue)
	mock.ExpectQuery("SELECT EXISTS.*FROM users").WillReturnError(sql.ErrNoRows)

	// Test - should return false on error (treats as no users for setup purposes)
	hasUsers, err := HasAnyUsers(context.Background(), db)
	require.NoError(t, err) // Should not propagate the error
	assert.False(t, hasUsers)

	// Verify expectations
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}
