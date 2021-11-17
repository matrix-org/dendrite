package internal

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matryer/is"
)

var (
	ctx        = context.Background()
	localpart  = "foo"
	testPusher = api.Pusher{
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
	}
	testPusher2 = api.Pusher{
		SessionID:         42984798792,
		PushKey:           "dc_GxbDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ---",
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
	}
	nilPusher = api.Pusher{
		PushKey: "dc_GxbDa8El0pWKkDIM-rQ:APA91bHflmL6ycJMbLKX8VYLD-Ebft3t-SLQwIap-pDWP-evu1AWxsXxzyl1pgSZxDMn6OeznZsjXhTU0m5xz05dyJ4syX86S89uwxBwtbK-k0PHQt9wF8CgOcibm-OYZodpY5TtmknZ",
		AppID:   "com.example.app.ios",
	}
)

func TestPerformPusherSet(t *testing.T) {
	is := is.New(t)
	dut := mustNewPushserverAPI(is)
	pushers := mustSetPushers(is, dut, testPusher)
	is.Equal(len(pushers.Pushers), 1)
	pushKeyTS := pushers.Pushers[0].PushKeyTS
	is.True(pushKeyTS != 0)
	pushers.Pushers[0].PushKeyTS = 0
	is.Equal(pushers.Pushers[0], testPusher)
	pushers.Pushers[0].PushKeyTS = pushKeyTS

}

func TestPerformPusherSet_Append(t *testing.T) {
	is := is.New(t)
	dut := mustNewPushserverAPI(is)
	mustSetPushers(is, dut, testPusher)
	pushers := mustAppendPushers(is, dut, testPusher2)
	is.Equal(len(pushers.Pushers), 2)
	is.True(pushers.Pushers[1].PushKeyTS != 0)
	pushers.Pushers[1].PushKeyTS = 0
	is.Equal(pushers.Pushers[1], testPusher2)
}

func TestPerformPusherSet_Delete(t *testing.T) {
	is := is.New(t)
	dut := mustNewPushserverAPI(is)
	mustSetPushers(is, dut, testPusher)
	pushers := mustSetPushers(is, dut, nilPusher)
	// pushers := mustAppendPushers(is, dut, testPusher2)
	is.Equal(len(pushers.Pushers), 0)
}

func TestPerformPusherSet_AppendDelete(t *testing.T) {
	is := is.New(t)
	dut := mustNewPushserverAPI(is)
	mustSetPushers(is, dut, testPusher)
	mustAppendPushers(is, dut, testPusher2)
	pushers := mustAppendPushers(is, dut, nilPusher)
	is.Equal(len(pushers.Pushers), 1)
	is.True(pushers.Pushers[0].PushKeyTS != 0)
	pushers.Pushers[0].PushKeyTS = 0
	is.Equal(pushers.Pushers[0], testPusher2)
}

func mustNewPushserverAPI(is *is.I) api.PushserverInternalAPI {
	db := mustNewDatabase(is)
	return &PushserverInternalAPI{
		DB: db,
	}
}

func mustNewDatabase(is *is.I) storage.Database {
	randPostfix := strconv.Itoa(rand.Int())
	dbPath := os.TempDir() + "/dendrite-" + randPostfix
	dut, err := storage.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource("file:" + dbPath),
	})
	is.NoErr(err)
	return dut
}

func mustSetPushers(is *is.I, dut api.PushserverInternalAPI, p api.Pusher) *api.QueryPushersResponse {
	err := dut.PerformPusherSet(ctx, &api.PerformPusherSetRequest{
		Localpart: localpart,
		Append:    false,
		Pusher:    p,
	}, &struct{}{})
	is.NoErr(err)
	var pushers api.QueryPushersResponse
	err = dut.QueryPushers(ctx, &api.QueryPushersRequest{
		Localpart: localpart,
	}, &pushers)
	is.NoErr(err)
	return &pushers
}

func mustAppendPushers(is *is.I, dut api.PushserverInternalAPI, p api.Pusher) *api.QueryPushersResponse {
	err := dut.PerformPusherSet(ctx, &api.PerformPusherSetRequest{
		Localpart: localpart,
		Append:    true,
		Pusher:    p,
	}, &struct{}{})
	is.NoErr(err)
	var pushers api.QueryPushersResponse
	err = dut.QueryPushers(ctx, &api.QueryPushersRequest{
		Localpart: localpart,
	}, &pushers)
	is.NoErr(err)
	return &pushers
}
