## Database migrations

We use [goose](https://github.com/pressly/goose) to handle database migrations. This allows us to execute
both SQL deltas (e.g `ALTER TABLE ...`) as well as manipulate data in the database in Go using Go functions.

To run a migration, the `goose` binary in this directory needs to be built:
```
$ go build ./cmd/goose
```

This binary allows Dendrite databases to be upgraded and downgraded. Sample usage for upgrading the roomserver database:

```
# for sqlite
$ ./goose -dir roomserver/storage/sqlite3/deltas sqlite3 ./roomserver.db up

# for postgres
$ ./goose -dir roomserver/storage/postgres/deltas postgres "user=dendrite dbname=dendrite sslmode=disable" up
```

For a full list of options, including rollbacks, see https://github.com/pressly/goose or use `goose` with no args.


### Rationale

Dendrite creates tables on startup using `CREATE TABLE IF NOT EXISTS`, so you might think that we should also
apply version upgrades on startup as well. This is convenient and doesn't involve an additional binary to run
which complicates upgrades. However, combining the upgrade mechanism and the server binary makes it difficult
to handle rollbacks. Firstly, how do you specify you wish to rollback? We would have to add additional flags
to the main server binary to say "rollback to version X". Secondly, if you roll back the server binary from
version 5 to version 4, the version 4 binary doesn't know how to rollback the database from version 5 to
version 4! For these reasons, we prefer to have a separate "upgrade" binary which is run for database upgrades.
Rather than roll-our-own migration tool, we decided to use [goose](https://github.com/pressly/goose) as it supports
complex migrations in Go code in addition to just executing SQL deltas. Other alternatives like
`github.com/golang-migrate/migrate` [do not support](https://github.com/golang-migrate/migrate/issues/15) these
kinds of complex migrations.

### Adding new deltas

You can add `.sql` or `.go` files manually or you can use goose to create them for you.

If you only want to add a SQL delta then run:

```
$ ./goose -dir serverkeyapi/storage/sqlite3/deltas sqlite3 ./foo.db create new_col sql
2020/09/09 14:37:43 Created new file: serverkeyapi/storage/sqlite3/deltas/20200909143743_new_col.sql
```

In this case, the version number is `20200909143743`. The important thing is that it is always increasing.

Then add up/downgrade SQL commands to the created file which looks like:
```sql
-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query';
-- +goose StatementEnd

```
You __must__ keep the `+goose` annotations. You'll need to repeat this process for Postgres.

For complex Go migrations:

```
$ ./goose -dir serverkeyapi/storage/sqlite3/deltas sqlite3 ./foo.db create complex_update go
2020/09/09 14:40:38 Created new file: serverkeyapi/storage/sqlite3/deltas/20200909144038_complex_update.go
```

Then modify the created `.go` file which looks like:

```go
package migrations

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose"
)

func init() {
	goose.AddMigration(upComplexUpdate, downComplexUpdate)
}

func upComplexUpdate(tx *sql.Tx) error {
	// This code is executed when the migration is applied.
	return nil
}

func downComplexUpdate(tx *sql.Tx) error {
	// This code is executed when the migration is rolled back.
	return nil
}

```

You __must__ import the package in `/cmd/goose/main.go` so `func init()` gets called.


#### Database limitations

- SQLite3 does NOT support `ALTER TABLE table_name DROP COLUMN` - you would have to rename the column or drop the table
  entirely and recreate it. ([example](https://github.com/matrix-org/dendrite/blob/master/userapi/storage/accounts/sqlite3/deltas/20200929203058_is_active.sql))
  
  More information: [sqlite.org](https://www.sqlite.org/lang_altertable.html)
