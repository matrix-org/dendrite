package storage

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matryer/is"
)

var testCtx = context.Background()

var (
	testPushers = []api.Pusher{
		{
			SessionID:         42984798792,
			PushKey:           "dc_GxbDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ",
			Kind:              "http",
			AppID:             "com.example.app.ios",
			AppDisplayName:    "Mat Rix",
			DeviceDisplayName: "iPhone 9",
			ProfileTag:        "xxyyzz",
			Language:          "pl",
			Data: map[string]interface{}{
				"format": "event_id_only",
				"url":    "https://push-gateway.location.there/_matrix/push/v1/notify",
			},
		},
		{
			SessionID:         4298479873432,
			PushKey:           "dnjekDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ",
			Kind:              "http",
			AppID:             "com.example.app.ios",
			AppDisplayName:    "Riot",
			DeviceDisplayName: "Android 11",
			ProfileTag:        "aabbcc",
			Language:          "en",
			Data: map[string]interface{}{
				"format": "event_id_only",
				"url":    "https://push-gateway.location.there/_matrix/push/v1/notify",
			},
		},
		{
			SessionID:         4298479873432,
			PushKey:           "dc_GxbDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ",
			Kind:              "http",
			AppID:             "com.example.app.ios",
			AppDisplayName:    "Riot",
			DeviceDisplayName: "Android 11",
			ProfileTag:        "aabbcc",
			Language:          "en",
			Data: map[string]interface{}{
				"format": "event_id_only",
				"url":    "https://push-gateway.location.there/_matrix/push/v1/notify",
			},
		},
	}

	updatePusher = api.Pusher{
		AppID:             "com.example.app.ios",
		PushKey:           "dc_GxbDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ",
		SessionID:         429847987,
		Kind:              "http",
		AppDisplayName:    "Mat Rix 2",
		DeviceDisplayName: "iPhone 9a",
		ProfileTag:        "xxyyzzaa",
		Language:          "en",
		Data: map[string]interface{}{
			"format": "event_id_only",
			"url":    "https://push-gateway.location.here/_matrix/push/v1/notify",
		},
	}
)

var testUsers = []string{
	"admin",
	"admin",
	"admin0",
	"admin",
}

func mustNewDatabaseWithTestPushers(is *is.I) Database {
	dut := mustNewDatabase(is)
	for i, testPusher := range testPushers {
		err := dut.UpsertPusher(testCtx, testPusher, testUsers[i])
		is.NoErr(err)
	}
	return dut
}

func mustNewDatabase(is *is.I) Database {
	randPostfix := strconv.Itoa(rand.Int())
	dbPath := os.TempDir() + "/dendrite-" + randPostfix
	dut, err := Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource("file:" + dbPath),
	})
	is.NoErr(err)
	return dut
}

func TestInsertPusher(t *testing.T) {
	is := is.New(t)
	mustNewDatabaseWithTestPushers(is)
}

func TestSelectPushers(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestPushers(is)
	pushers, err := dut.GetPushers(testCtx, "admin")
	is.NoErr(err)
	is.Equal(len(pushers), 2)
	is.Equal(pushers[0], testPushers[0])
	is.Equal(pushers[1], testPushers[1])
	// for i := range testPushers {
	// }
}

func TestDeletePusher(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestPushers(is)
	err := dut.RemovePusher(
		testCtx,
		"com.example.app.ios",
		"dc_GxbDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ",
		"admin")
	is.NoErr(err)
	pushers, err := dut.GetPushers(testCtx, "admin")
	is.NoErr(err)
	is.Equal(len(pushers), 1)
	is.Equal(pushers[0], testPushers[1])
	pushers, err = dut.GetPushers(testCtx, "admin0")
	is.NoErr(err)
	is.Equal(len(pushers), 1)
	is.Equal(pushers[0], testPushers[2])
}

func TestDeletePushers(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestPushers(is)
	err := dut.RemovePushers(
		testCtx,
		"com.example.app.ios",
		"dc_GxbDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ")
	is.NoErr(err)
	pushers, err := dut.GetPushers(testCtx, "admin")
	is.NoErr(err)
	is.Equal(len(pushers), 1)
	is.Equal(pushers[0], testPushers[1])
	pushers, err = dut.GetPushers(testCtx, "admin0")
	is.NoErr(err)
	is.Equal(len(pushers), 0)
}

func TestUpdatePusher(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabase(is)
	err := dut.UpsertPusher(testCtx, testPushers[0], "admin")
	is.NoErr(err)
	err = dut.UpsertPusher(testCtx, updatePusher, "admin")
	is.NoErr(err)
	pushers, err := dut.GetPushers(testCtx, "admin")
	is.NoErr(err)
	is.Equal(len(pushers), 1)
	t.Log(pushers[0])
	t.Log(updatePusher)
	is.Equal(pushers[0], updatePusher)
}
