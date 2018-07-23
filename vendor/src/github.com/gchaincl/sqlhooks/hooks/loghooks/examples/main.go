package main

import (
	"database/sql"
	"log"

	"github.com/gchaincl/sqlhooks"
	"github.com/gchaincl/sqlhooks/hooks/loghooks"
	"github.com/mattn/go-sqlite3"
)

func main() {
	sql.Register("sqlite3log", sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, loghooks.New()))
	db, err := sql.Open("sqlite3log", ":memory:")
	if err != nil {
		log.Fatal(err)
	}

	if _, err := db.Exec("CREATE TABLE users(ID int, name text)"); err != nil {
		log.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO users (id, name) VALUES(?, ?)`, 1, "gus"); err != nil {
		log.Fatal(err)
	}

	if _, err := db.Query(`SELECT id, name FROM users`); err != nil {
		log.Fatal(err)
	}

}
