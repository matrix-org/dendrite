package sqlite3

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/dendrite/userapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func mustMakeDBs(t *testing.T) (*sql.DB, tables.AccountsTable, tables.DevicesTable, tables.StatsTable) {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("unable to open in-memory database: %v", err)
	}

	accDB, err := NewSQLiteAccountsTable(db, "localhost")
	if err != nil {
		t.Fatalf("unable to create acc db: %v", err)
	}
	devDB, err := NewSQLiteDevicesTable(db, "localhost")
	if err != nil {
		t.Fatalf("unable to open device db: %v", err)
	}
	statsDB, err := NewSQLiteStatsTable(db, "localhost")
	if err != nil {
		t.Fatalf("unable to open stats db: %v", err)
	}
	return db, accDB, devDB, statsDB
}

func mustMakeAccountAndDevice(
	t *testing.T,
	ctx context.Context,
	accDB tables.AccountsTable,
	devDB tables.DevicesTable,
	localpart string,
	accType api.AccountType,
	userAgent string,
) {
	t.Helper()

	appServiceID := ""
	if accType == api.AccountTypeAppService {
		appServiceID = util.RandomString(16)
	}

	_, err := accDB.InsertAccount(ctx, nil, localpart, "", appServiceID, accType)
	if err != nil {
		t.Fatalf("unable to create account: %v", err)
	}
	_, err = devDB.InsertDevice(ctx, nil, "deviceID", localpart, util.RandomString(16), nil, "", userAgent)
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
	_, err := db.ExecContext(ctx, "UPDATE device_devices SET last_seen_ts = $1 WHERE localpart = $2", gomatrixserverlib.AsTimestamp(timestamp), localpart)
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
	_, err := db.ExecContext(ctx, "UPDATE account_accounts SET created_ts = $1 WHERE localpart = $2", gomatrixserverlib.AsTimestamp(timestamp), localpart)
	if err != nil {
		t.Fatalf("unable to update device last seen")
	}
}

func mustUpdateUserDailyVisits(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	startTime time.Time,
) {
	_, err := db.ExecContext(ctx, updateUserDailyVisitsSQL,
		gomatrixserverlib.AsTimestamp(startTime.Truncate(time.Hour*24)),
		gomatrixserverlib.AsTimestamp(startTime.Truncate(time.Hour*24).Add(time.Hour)),
		gomatrixserverlib.AsTimestamp(time.Now()),
	)
	if err != nil {
		t.Fatalf("unable to update device last seen")
	}
}

// These tests must run sequentially, as they build up on each other
func Test_statsStatements_UserStatistics(t *testing.T) {

	ctx := context.Background()
	db, accDB, devDB, statsDB := mustMakeDBs(t)

	t.Run("want SQLite engine", func(t *testing.T) {
		_, gotDB, err := statsDB.UserStatistics(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if "SQLite" != gotDB.Engine { // can't use DeepEqual, as the Version might differ
			t.Errorf("UserStatistics() gotDB = %+v, want SQLite", gotDB.Engine)
		}
	})

	t.Run("Want Users", func(t *testing.T) {
		mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user1", api.AccountTypeUser, "Element Android")
		mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user2", api.AccountTypeUser, "Element iOS")
		mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user3", api.AccountTypeUser, "Element web")
		mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user4", api.AccountTypeGuest, "Element electron")
		mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user5", api.AccountTypeAdmin, "gecko")
		mustMakeAccountAndDevice(t, ctx, accDB, devDB, "user6", api.AccountTypeAppService, "gecko")
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
		mustUpdateUserDailyVisits(t, ctx, db, time.Now().AddDate(0, -1, 0))
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
			mustUpdateUserDailyVisits(t, ctx, db, time.Now().AddDate(0, 0, -i))
		}
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
}
