package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/seuros/kaunta/internal/database"
	"github.com/stretchr/testify/require"
)

type mockResponse struct {
	match   string
	columns []string
	rows    [][]interface{}
	args    []interface{}
	err     error
}

type mockQueue struct {
	mu        sync.Mutex
	responses []mockResponse
}

func newMockQueue(responses []mockResponse) *mockQueue {
	return &mockQueue{
		responses: append([]mockResponse(nil), responses...),
	}
}

func (mq *mockQueue) pop(query string, args []driver.NamedValue) (mockResponse, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.responses) == 0 {
		return mockResponse{}, fmt.Errorf("unexpected query: %s", query)
	}

	resp := mq.responses[0]
	mq.responses = mq.responses[1:]

	if resp.match != "" && !strings.Contains(normalizeWhitespace(query), normalizeWhitespace(resp.match)) {
		return mockResponse{}, fmt.Errorf("query mismatch: got %q, expected to contain %q", query, resp.match)
	}

	if len(resp.args) > 0 {
		if len(resp.args) != len(args) {
			return mockResponse{}, fmt.Errorf("argument count mismatch: got %d, want %d", len(args), len(resp.args))
		}
		for i, expected := range resp.args {
			if fmt.Sprint(args[i].Value) != fmt.Sprint(expected) {
				return mockResponse{}, fmt.Errorf("arg %d mismatch: got %v, want %v", i, args[i].Value, expected)
			}
		}
	}

	return resp, nil
}

func (mq *mockQueue) expectationsMet() error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if len(mq.responses) != 0 {
		return fmt.Errorf("not all expectations met: %d remaining", len(mq.responses))
	}
	return nil
}

type mockDriver struct {
	queue *mockQueue
}

func (d *mockDriver) Open(name string) (driver.Conn, error) {
	return &mockConn{queue: d.queue}, nil
}

type mockConn struct {
	queue *mockQueue
}

func (c *mockConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (c *mockConn) Close() error                        { return nil }
func (c *mockConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }

func (c *mockConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	resp, err := c.queue.pop(query, args)
	if err != nil {
		return nil, err
	}
	if resp.err != nil {
		return nil, resp.err
	}

	values := make([][]driver.Value, len(resp.rows))
	for i, row := range resp.rows {
		values[i] = make([]driver.Value, len(row))
		for j, v := range row {
			values[i][j] = v
		}
	}

	return &mockRows{
		columns: resp.columns,
		rows:    values,
	}, nil
}

func (c *mockConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{
			Ordinal: i + 1,
			Value:   arg,
		}
	}
	return c.QueryContext(context.Background(), query, named)
}

type mockRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *mockRows) Columns() []string { return r.columns }
func (r *mockRows) Close() error      { return nil }

func (r *mockRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}

	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

var driverCounter struct {
	sync.Mutex
	value int
}

func registerMockDriver(queue *mockQueue) (string, error) {
	driverCounter.Lock()
	defer driverCounter.Unlock()

	name := fmt.Sprintf("mock-driver-%d", driverCounter.value)
	driverCounter.value++

	sql.Register(name, &mockDriver{queue: queue})
	return name, nil
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func setupHTTPTest(t *testing.T, route string, handler http.HandlerFunc, responses []mockResponse) (http.Handler, *mockQueue, func()) {
	t.Helper()

	queue := newMockQueue(responses)

	driverName, err := registerMockDriver(queue)
	require.NoError(t, err)

	db, err := sql.Open(driverName, "")
	require.NoError(t, err)

	originalDB := database.DB
	database.DB = db

	router := chi.NewRouter()
	router.Get(route, handler)

	cleanup := func() {
		database.DB = originalDB
		_ = db.Close()
	}

	return router, queue, cleanup
}
