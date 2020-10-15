// This is custom goose binary

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	pgaccounts "github.com/matrix-org/dendrite/userapi/storage/accounts/postgres/deltas"
	slaccounts "github.com/matrix-org/dendrite/userapi/storage/accounts/sqlite3/deltas"
	pgdevices "github.com/matrix-org/dendrite/userapi/storage/devices/postgres/deltas"
	sldevices "github.com/matrix-org/dendrite/userapi/storage/devices/sqlite3/deltas"
	"github.com/pressly/goose"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

const (
	AppService       = "appservice"
	FederationSender = "federationsender"
	KeyServer        = "keyserver"
	MediaAPI         = "mediaapi"
	RoomServer       = "roomserver"
	SigningKeyServer = "signingkeyserver"
	SyncAPI          = "syncapi"
	UserAPIAccounts  = "userapi_accounts"
	UserAPIDevices   = "userapi_devices"
)

var (
	dir       = flags.String("dir", "", "directory with migration files")
	flags     = flag.NewFlagSet("goose", flag.ExitOnError)
	component = flags.String("component", "", "dendrite component name")
	knownDBs  = []string{
		AppService, FederationSender, KeyServer, MediaAPI, RoomServer, SigningKeyServer, SyncAPI, UserAPIAccounts, UserAPIDevices,
	}
)

// nolint: gocyclo
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
    goose -component roomserver sqlite3 ./roomserver.db status
    goose -component roomserver sqlite3 ./roomserver.db up

    goose -component roomserver postgres "user=dendrite dbname=dendrite sslmode=disable" status

Options:
  -component string
    	Dendrite component name e.g roomserver, signingkeyserver, clientapi, syncapi
  -table string
    	migrations table name (default "goose_db_version")
  -h	print help
  -v	enable verbose mode
  -dir string
        directory with migration files, only relevant when creating new migrations.
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

	knownComponent := false
	for _, c := range knownDBs {
		if c == *component {
			knownComponent = true
			break
		}
	}
	if !knownComponent {
		fmt.Printf("component must be one of %v\n", knownDBs)
		return
	}

	if engine == "sqlite3" {
		loadSQLiteDeltas(*component)
	} else {
		loadPostgresDeltas(*component)
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

	// goose demands a directory even though we don't use it for upgrades
	d := *dir
	if d == "" {
		d = os.TempDir()
	}
	if err := goose.Run(command, db, d, arguments...); err != nil {
		log.Fatalf("goose %v: %v", command, err)
	}
}

func loadSQLiteDeltas(component string) {
	switch component {
	case UserAPIAccounts:
		slaccounts.LoadFromGoose()
	case UserAPIDevices:
		sldevices.LoadFromGoose()
	}
}

func loadPostgresDeltas(component string) {
	switch component {
	case UserAPIAccounts:
		pgaccounts.LoadFromGoose()
	case UserAPIDevices:
		pgdevices.LoadFromGoose()
	}
}
