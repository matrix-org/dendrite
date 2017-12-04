package sqlhooks

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHooks struct {
	before Hook
	after  Hook
}

func (h *testHooks) noop() {
	noop := func(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
		return ctx, nil
	}

	h.before, h.after = noop, noop
}

func (h *testHooks) Before(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
	return h.before(ctx, query, args...)
}

func (h *testHooks) After(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
	return h.after(ctx, query, args...)
}

type suite struct {
	db    *sql.DB
	hooks *testHooks
}

func newSuite(t *testing.T, driver driver.Driver, dsn string) *suite {
	hooks := &testHooks{}
	driverName := fmt.Sprintf("sqlhooks-%s", time.Now().String())
	sql.Register(driverName, Wrap(driver, hooks))

	db, err := sql.Open(driverName, dsn)
	require.NoError(t, err)
	require.NoError(t, db.Ping())

	return &suite{db, hooks}
}

func (s *suite) TestHooksExecution(t *testing.T, query string, args ...interface{}) {
	var before, after bool

	s.hooks.before = func(ctx context.Context, q string, a ...interface{}) (context.Context, error) {
		before = true
		return ctx, nil
	}
	s.hooks.after = func(ctx context.Context, q string, a ...interface{}) (context.Context, error) {
		after = true
		return ctx, nil
	}

	t.Run("Query", func(t *testing.T) {
		before, after = false, false
		_, err := s.db.Query(query, args...)
		require.NoError(t, err)
		assert.True(t, before, "Before Hook did not run for query: "+query)
		assert.True(t, after, "After Hook did not run for query:  "+query)
	})

	t.Run("QueryContext", func(t *testing.T) {
		before, after = false, false
		_, err := s.db.QueryContext(context.Background(), query, args...)
		require.NoError(t, err)
		assert.True(t, before, "Before Hook did not run for query: "+query)
		assert.True(t, after, "After Hook did not run for query:  "+query)
	})

	t.Run("Exec", func(t *testing.T) {
		before, after = false, false
		_, err := s.db.Exec(query, args...)
		require.NoError(t, err)
		assert.True(t, before, "Before Hook did not run for query: "+query)
		assert.True(t, after, "After Hook did not run for query:  "+query)
	})

	t.Run("ExecContext", func(t *testing.T) {
		before, after = false, false
		_, err := s.db.ExecContext(context.Background(), query, args...)
		require.NoError(t, err)
		assert.True(t, before, "Before Hook did not run for query: "+query)
		assert.True(t, after, "After Hook did not run for query:  "+query)
	})

	t.Run("Statements", func(t *testing.T) {
		before, after = false, false
		stmt, err := s.db.Prepare(query)
		require.NoError(t, err)

		// Hooks just run when the stmt is executed (Query or Exec)
		assert.False(t, before, "Before Hook run before execution: "+query)
		assert.False(t, after, "After Hook run before execution:  "+query)

		stmt.Query(args...)
		assert.True(t, before, "Before Hook did not run for query: "+query)
		assert.True(t, after, "After Hook did not run for query:  "+query)
	})
}

func (s *suite) testHooksArguments(t *testing.T, query string, args ...interface{}) {
	hook := func(ctx context.Context, q string, a ...interface{}) (context.Context, error) {
		assert.Equal(t, query, q)
		assert.Equal(t, args, a)
		assert.Equal(t, "val", ctx.Value("key").(string))
		return ctx, nil
	}
	s.hooks.before = hook
	s.hooks.after = hook

	ctx := context.WithValue(context.Background(), "key", "val")
	{
		_, err := s.db.QueryContext(ctx, query, args...)
		require.NoError(t, err)
	}

	{
		_, err := s.db.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}
}

func (s *suite) TestHooksArguments(t *testing.T, query string, args ...interface{}) {
	t.Run("TestHooksArguments", func(t *testing.T) { s.testHooksArguments(t, query, args...) })
}

func (s *suite) testHooksErrors(t *testing.T, query string) {
	boom := errors.New("boom")
	s.hooks.before = func(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
		return ctx, boom
	}

	s.hooks.after = func(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
		assert.False(t, true, "this should not run")
		return ctx, nil
	}

	_, err := s.db.Query(query)
	assert.Equal(t, boom, err)
}

func (s *suite) TestHooksErrors(t *testing.T, query string) {
	t.Run("TestHooksErrors", func(t *testing.T) { s.testHooksErrors(t, query) })
}

func TestNamedValueToValue(t *testing.T) {
	named := []driver.NamedValue{
		{Ordinal: 1, Value: "foo"},
		{Ordinal: 2, Value: 42},
	}
	want := []driver.Value{"foo", 42}
	dargs, err := namedValueToValue(named)
	require.NoError(t, err)
	assert.Equal(t, want, dargs)
}
