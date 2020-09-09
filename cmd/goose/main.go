// This is custom goose binary

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	// Example complex Go migration import:
	// _ "github.com/matrix-org/dendrite/serverkeyapi/storage/postgres/deltas"
	"github.com/pressly/goose"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

var (
	flags = flag.NewFlagSet("goose", flag.ExitOnError)
	dir   = flags.String("dir", ".", "directory with migration files")
)

func main() {
	err := flags.Parse(os.Args[1:])
	if err != nil {
		panic(err.Error())
	}
	args := flags.Args()

	if len(args) < 3 {
		fmt.Println(
			`Usage: goose [OPTIONS] DRIVER DBSTRING COMMAND

Drivers:
    postgres
    sqlite3

Examples:
    goose -d roomserver/storage/sqlite3/deltas sqlite3 ./roomserver.db status
    goose -d roomserver/storage/sqlite3/deltas sqlite3 ./roomserver.db up

    goose -d roomserver/storage/postgres/deltas postgres "user=dendrite dbname=dendrite sslmode=disable" status

Options:

  -dir string
    	directory with migration files (default ".")
  -table string
    	migrations table name (default "goose_db_version")
  -h	print help
  -v	enable verbose mode
  -version
    	print version

Commands:
    up                   Migrate the DB to the most recent version available
    up-by-one            Migrate the DB up by 1
    up-to VERSION        Migrate the DB to a specific VERSION
    down                 Roll back the version by 1
    down-to VERSION      Roll back to a specific VERSION
    redo                 Re-run the latest migration
    reset                Roll back all migrations
    status               Dump the migration status for the current DB
    version              Print the current version of the database
    create NAME [sql|go] Creates new migration file with the current timestamp
    fix                  Apply sequential ordering to migrations`,
		)
		return
	}

	engine := args[0]
	if engine != "sqlite3" && engine != "postgres" {
		fmt.Println("engine must be one of 'sqlite3' or 'postgres'")
		return
	}
	dbstring, command := args[1], args[2]

	db, err := goose.OpenDBWithDriver(engine, dbstring)
	if err != nil {
		log.Fatalf("goose: failed to open DB: %v\n", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("goose: failed to close DB: %v\n", err)
		}
	}()

	arguments := []string{}
	if len(args) > 3 {
		arguments = append(arguments, args[3:]...)
	}

	if err := goose.Run(command, db, *dir, arguments...); err != nil {
		log.Fatalf("goose %v: %v", command, err)
	}
}
