package tables_test

import (
	"context"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi/storage/postgres"
	"github.com/matrix-org/dendrite/userapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

const serverNotice = "notice"

func mustCreateProfileTable(t *testing.T, dbType test.DBType) (tab tables.ProfileTable, close func()) {
	var connStr string
	connStr, close = test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatal(err)
	}
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresProfilesTable(db, serverNotice)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteProfilesTable(db, serverNotice)
	}
	if err != nil {
		t.Fatalf("failed to create profiles table: %v", err)
	}
	return tab, close
}

func mustCreateProfile(t *testing.T, ctx context.Context, tab tables.ProfileTable, localPart string, serverName gomatrixserverlib.ServerName) {
	if err := tab.InsertProfile(ctx, nil, localPart, serverName); err != nil {
		t.Fatalf("failed to insert profile: %v", err)
	}
}

func TestProfileTable(t *testing.T) {
	ctx := context.Background()
	serverName1 := gomatrixserverlib.ServerName("localhost")
	serverName2 := gomatrixserverlib.ServerName("notlocalhost")
	avatarURL := "newAvatarURL"
	displayName := "newDisplayName"

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateProfileTable(t, dbType)
		defer close()
		// Create serverNotice user
		if err := tab.InsertProfile(ctx, nil, serverNotice, serverName1); err != nil {
			t.Fatalf("failed to insert profile: %v", err)
		}
		// Ensure the a localpart is unique per serverName
		if err := tab.InsertProfile(ctx, nil, serverNotice, serverName1); err == nil {
			t.Fatalf("expected SQL insert to fail, but it didn't")
		}

		mustCreateProfile(t, ctx, tab, "dummy1", serverName1)
		mustCreateProfile(t, ctx, tab, "dummy1", serverName2)
		mustCreateProfile(t, ctx, tab, "testing", serverName2)

		if err := tab.SetAvatarURL(ctx, nil, "dummy1", serverName1, avatarURL); err != nil {
			t.Fatalf("failed to set avatar url: %v", err)
		}
		if err := tab.SetDisplayName(ctx, nil, "dummy1", serverName2, displayName); err != nil {
			t.Fatalf("failed to set avatar url: %v", err)
		}

		// Verify dummy1 on serverName2 is as expected, just to test the function
		dummy1, err := tab.SelectProfileByLocalpart(ctx, "dummy1", serverName2)
		if err != nil {
			t.Fatalf("failed to query profile by localpart: %v", err)
		}
		// Make sure that only dummy1 on serverName1 got the displayName changed and avatarURL is unchanged
		if dummy1.AvatarURL == avatarURL {
			t.Fatalf("expected avatarURL %s, got %s", avatarURL, dummy1.AvatarURL)
		}
		if dummy1.DisplayName != displayName {
			t.Fatalf("expected displayName %s, got %s", displayName, dummy1.DisplayName)
		}

		searchRes, err := tab.SelectProfilesBySearch(ctx, "dummy", 10)
		if err != nil {
			t.Fatalf("unable to search profiles: %v", err)
		}
		// serverNotice user and testing should not be returned here, only the dummy users
		if count := len(searchRes); count > 2 {
			t.Fatalf("expected 2 results, got %d", count)
		}
		for _, profile := range searchRes {
			if profile.Localpart != "dummy1" {
				t.Fatalf("got unexpected localpart: %v", profile.Localpart)
			}
			// Make sure that only dummy1 on serverName1 got the avatarURL changed and displayName is unchanged
			if gomatrixserverlib.ServerName(profile.ServerName) == serverName1 && profile.AvatarURL != avatarURL {
				t.Fatalf("expected avatarURL %s, got %s", avatarURL, profile.AvatarURL)
			}
			if gomatrixserverlib.ServerName(profile.ServerName) == serverName1 && profile.DisplayName == displayName {
				t.Fatalf("expected displayName %s, got %s", displayName, profile.DisplayName)
			}
			// Make sure that only dummy1 on serverName1 got the displayName changed and avatarURL is unchanged
			if gomatrixserverlib.ServerName(profile.ServerName) == serverName2 && profile.AvatarURL == avatarURL {
				t.Fatalf("expected avatarURL %s, got %s", avatarURL, profile.AvatarURL)
			}
			if gomatrixserverlib.ServerName(profile.ServerName) == serverName2 && profile.DisplayName != displayName {
				t.Fatalf("expected displayName %s, got %s", displayName, profile.DisplayName)
			}
		}

	})
}
