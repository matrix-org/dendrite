package tables_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/postgres"
	"github.com/matrix-org/dendrite/userapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/dendrite/userapi/types"
)

func mustMakeDBs(t *testing.T, dbType test.DBType) (
	*sql.DB, tables.AccountsTable, tables.DevicesTable, tables.StatsTable, func(),
) {
	t.Helper()

	var (
		accTable   tables.AccountsTable
		devTable   tables.DevicesTable
		statsTable tables.StatsTable
		err        error
	)

	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, nil)
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	switch dbType {
	case test.DBTypeSQLite:
		accTable, err = sqlite3.NewSQLiteAccountsTable(db, "localhost")
		if err != nil {
			t.Fatalf("unable to create acc db: %v", err)
		}
		devTable, err = sqlite3.NewSQLiteDevicesTable(db, "localhost")
		if err != nil {
			t.Fatalf("unable to open device db: %v", err)
		}
		statsTable, err = sqlite3.NewSQLiteStatsTable(db, "localhost")
		if err != nil {
			t.Fatalf("unable to open stats db: %v", err)
		}
	case test.DBTypePostgres:
		accTable, err = postgres.NewPostgresAccountsTable(db, "localhost")
		if err != nil {
			t.Fatalf("unable to create acc db: %v", err)
		}
		devTable, err = postgres.NewPostgresDevicesTable(db, "localhost")
		if err != nil {
			t.Fatalf("unable to open device db: %v", err)
		}
		statsTable, err = postgres.NewPostgresStatsTable(db, "localhost")
		if err != nil {
			t.Fatalf("unable to open stats db: %v", err)
		}
	}

	return db, accTable, devTable, statsTable, close
}

func mustMakeAccountAndDevice(
	t *testing.T,
	ctx context.Context,
	accDB tables.AccountsTable,
	devDB tables.DevicesTable,
	localpart string,
	serverName gomatrixserverlib.ServerName, // nolint:unparam
	accType api.AccountType,
	userAgent string,
) {
	t.Helper()

	appServiceID := ""
	if accType == api.AccountTypeAppService {
		appServiceID = util.RandomString(16)
	}

	_, err := accDB.InsertAccount(ctx, nil, localpart, serverName, "", appServiceID, accType)
	if err != nil {
		t.Fatalf("unable to create account: %v", err)
	}
	_, err = devDB.InsertDevice(ctx, nil, "deviceID", localpart, serverName, util.RandomString(16), nil, "", userAgent)
	if err != nil {
		t.Fatalf("unable to create device: %v", err)
	}
}

func mustUpdateDeviceLastSeen(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	localpart string,
	timestamp time.Time,
) {
	t.Helper()
	_, err := db.ExecContext(ctx, "UPDATE userapi_devices SET last_seen_ts = $1 WHERE localpart = $2", gomatrixserverlib.AsTimestamp(timestamp), localpart)
	if err != nil {
		t.Fatalf("unable to update device last seen")
	}
}

func mustUserUpdateRegistered(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	localpart string,
	timestamp time.Time,
) {
	_, err := db.ExecContext(ctx, "UPDATE userapi_accounts SET created_ts = $1 WHERE localpart = $2", gomatrixserverlib.AsTimestamp(timestamp), localpart)
	if err != nil {
		t.Fatalf("unable to update device last seen")
	}
}

// These tests must run sequentially, as they build up on each other
func Test_UserStatistics(t *testing.T) {

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, accDB, devDB, statsDB, close := mustMakeDBs(t, dbType)
		defer close()
		wantType := "SQLite"
		if dbType == test.DBTypePostgres {
			wantType = "Postgres"
		}

		t.Run(fmt.Sprintf("want %s database engine", wantType), func(t *testing.T) {
			_, gotDB, err := statsDB.UserStatistics(ctx, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if wantType != gotDB.Engine { // can't use DeepEqual, as the Version might differ
				t.Errorf("UserStatistics() got DB engine = %+v, want %s", gotDB.Engine, wantType)
			}
		})

		t.Run("Want Users", func(t *testing.T) {
			mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user1", "localhost", api.AccountTypeUser, "Element Android")
			mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user2", "localhost", api.AccountTypeUser, "Element iOS")
			mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user3", "localhost", api.AccountTypeUser, "Element web")
			mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user4", "localhost", api.AccountTypeGuest, "Element Electron")
			mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user5", "localhost", api.AccountTypeAdmin, "gecko")
			mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user6", "localhost", api.AccountTypeAppService, "gecko")
			gotStats, _, err := statsDB.UserStatistics(ctx, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantStats := &types.UserStatistics{
				RegisteredUsersByType: map[string]int64{
					"native":  4,
					"guest":   1,
					"bridged": 1,
				},
				R30Users: map[string]int64{},
				R30UsersV2: map[string]int64{
					"ios":      0,
					"android":  0,
					"web":      0,
					"electron": 0,
					"all":      0,
				},
				AllUsers:        6,
				NonBridgedUsers: 5,
				DailyUsers:      6,
				MonthlyUsers:    6,
			}
			if !reflect.DeepEqual(gotStats, wantStats) {
				t.Errorf("UserStatistics() gotStats = \n%+v\nwant\n%+v", gotStats, wantStats)
			}
		})

		t.Run("Users not active for one/two month", func(t *testing.T) {
			mustUpdateDeviceLastSeen(t, ctx, db, "user1", time.Now().AddDate(0, -2, 0))
			mustUpdateDeviceLastSeen(t, ctx, db, "user2", time.Now().AddDate(0, -1, 0))
			gotStats, _, err := statsDB.UserStatistics(ctx, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantStats := &types.UserStatistics{
				RegisteredUsersByType: map[string]int64{
					"native":  4,
					"guest":   1,
					"bridged": 1,
				},
				R30Users: map[string]int64{},
				R30UsersV2: map[string]int64{
					"ios":      0,
					"android":  0,
					"web":      0,
					"electron": 0,
					"all":      0,
				},
				AllUsers:        6,
				NonBridgedUsers: 5,
				DailyUsers:      4,
				MonthlyUsers:    4,
			}
			if !reflect.DeepEqual(gotStats, wantStats) {
				t.Errorf("UserStatistics() gotStats = \n%+v\nwant\n%+v", gotStats, wantStats)
			}
		})

		/* R30Users counts the number of 30 day retained users, defined as:
		- Users who have created their accounts more than 30 days ago
		- Where last seen at most 30 days ago
		- Where account creation and last_seen are > 30 days apart
		*/
		t.Run("R30Users tests", func(t *testing.T) {
			mustUserUpdateRegistered(t, ctx, db, "user1", time.Now().AddDate(0, -2, 0))
			mustUpdateDeviceLastSeen(t, ctx, db, "user1", time.Now())
			mustUserUpdateRegistered(t, ctx, db, "user4", time.Now().AddDate(0, -2, 0))
			mustUpdateDeviceLastSeen(t, ctx, db, "user4", time.Now())
			startTime := time.Now().AddDate(0, 0, -2)
			err := statsDB.UpdateUserDailyVisits(ctx, nil, startTime, startTime.Truncate(time.Hour*24))
			if err != nil {
				t.Fatalf("unable to update daily visits stats: %v", err)
			}

			gotStats, _, err := statsDB.UserStatistics(ctx, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantStats := &types.UserStatistics{
				RegisteredUsersByType: map[string]int64{
					"native":  3,
					"bridged": 1,
				},
				R30Users: map[string]int64{
					"all":      2,
					"android":  1,
					"electron": 1,
				},
				R30UsersV2: map[string]int64{
					"ios":      0,
					"android":  0,
					"web":      0,
					"electron": 0,
					"all":      0,
				},
				AllUsers:        6,
				NonBridgedUsers: 5,
				DailyUsers:      5,
				MonthlyUsers:    5,
			}
			if !reflect.DeepEqual(gotStats, wantStats) {
				t.Errorf("UserStatistics() gotStats = \n%+v\nwant\n%+v", gotStats, wantStats)
			}
		})

		/*
			R30UsersV2 counts the number of 30 day retained users, defined as users that:
			- Appear more than once in the past 60 days
			- Have more than 30 days between the most and least recent appearances that occurred in the past 60 days.
			most recent -> neueste
			least recent -> älteste

		*/
		t.Run("R30UsersV2 tests", func(t *testing.T) {
			// generate some data
			for i := 100; i > 0; i-- {
				mustUpdateDeviceLastSeen(t, ctx, db, "user1", time.Now().AddDate(0, 0, -i))
				mustUpdateDeviceLastSeen(t, ctx, db, "user5", time.Now().AddDate(0, 0, -i))
				startTime := time.Now().AddDate(0, 0, -i)
				err := statsDB.UpdateUserDailyVisits(ctx, nil, startTime, startTime.Truncate(time.Hour*24))
				if err != nil {
					t.Fatalf("unable to update daily visits stats: %v", err)
				}
			}
			gotStats, _, err := statsDB.UserStatistics(ctx, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantStats := &types.UserStatistics{
				RegisteredUsersByType: map[string]int64{
					"native":  3,
					"bridged": 1,
				},
				R30Users: map[string]int64{
					"all":      2,
					"android":  1,
					"electron": 1,
				},
				R30UsersV2: map[string]int64{
					"ios":      0,
					"android":  1,
					"web":      1,
					"electron": 0,
					"all":      2,
				},
				AllUsers:        6,
				NonBridgedUsers: 5,
				DailyUsers:      3,
				MonthlyUsers:    5,
			}
			if !reflect.DeepEqual(gotStats, wantStats) {
				t.Errorf("UserStatistics() gotStats = \n%+v\nwant\n%+v", gotStats, wantStats)
			}
		})
	})

}
