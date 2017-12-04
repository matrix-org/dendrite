package main

import (
	"context"
	"database/sql"
	"log"

	"github.com/gchaincl/sqlhooks"
	"github.com/gchaincl/sqlhooks/hooks/othooks"
	"github.com/mattn/go-sqlite3"
	"github.com/opentracing/opentracing-go"
)

func main() {
	tracer := opentracing.GlobalTracer()
	hooks := othooks.New(tracer)
	sql.Register("sqlite3ot", sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, hooks))
	db, err := sql.Open("sqlite3ot", ":memory:")
	if err != nil {
		log.Fatal(err)
	}

	span := tracer.StartSpan("sql")
	defer span.Finish()
	ctx := opentracing.ContextWithSpan(context.Background(), span)

	if _, err := db.ExecContext(ctx, "CREATE TABLE users(ID int, name text)"); err != nil {
		log.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, name) VALUES(?, ?)`, 1, "gus"); err != nil {
		log.Fatal(err)
	}

	if _, err := db.QueryContext(ctx, `SELECT id, name FROM users`); err != nil {
		log.Fatal(err)
	}

}
