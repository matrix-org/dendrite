package othooks

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gchaincl/sqlhooks"
	sqlite3 "github.com/mattn/go-sqlite3"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	tracer *mocktracer.MockTracer
)

func init() {
	tracer = mocktracer.New()
	driver := sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, New(tracer))
	sql.Register("ot", driver)
}

func TestSpansAreRecorded(t *testing.T) {
	db, err := sql.Open("ot", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	tracer.Reset()

	parent := tracer.StartSpan("parent")
	ctx := opentracing.ContextWithSpan(context.Background(), parent)

	{
		rows, err := db.QueryContext(ctx, "SELECT 1+?", "1")
		require.NoError(t, err)
		rows.Close()
	}

	{
		rows, err := db.QueryContext(ctx, "SELECT 1+?", "1")
		require.NoError(t, err)
		rows.Close()
	}

	parent.Finish()

	spans := tracer.FinishedSpans()
	require.Len(t, spans, 3)

	span := spans[1]
	assert.Equal(t, "sql", span.OperationName)

	logFields := span.Logs()[0].Fields
	assert.Equal(t, "query", logFields[0].Key)
	assert.Equal(t, "SELECT 1+?", logFields[0].ValueString)
	assert.Equal(t, "args", logFields[1].Key)
	assert.Equal(t, "[1]", logFields[1].ValueString)
	assert.NotEmpty(t, span.FinishTime)
}

func TesNoSpansAreRecorded(t *testing.T) {
	db, err := sql.Open("ot", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	tracer.Reset()

	rows, err := db.QueryContext(context.Background(), "SELECT 1")
	require.NoError(t, err)
	rows.Close()

	assert.Empty(t, tracer.FinishedSpans())
}
