package util_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage"
	userUtil "github.com/matrix-org/dendrite/userapi/util"
)

func TestNotifyUserCountsAsync(t *testing.T) {
	alice := test.NewUser(t)
	aliceLocalpart, serverName, err := gomatrixserverlib.SplitID('@', alice.ID)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()

	// Create a test room, just used to provide events
	room := test.NewRoom(t, alice)
	dummyEvent := room.Events()[len(room.Events())-1]

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		receivedRequest := make(chan bool, 1)
		// create a test server which responds to our /notify call
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Return empty result, otherwise the call is handled as failed
			if _, err := w.Write([]byte("{}")); err != nil {
				t.Error(err)
			}
			receivedRequest <- true
		}))
		defer srv.Close()

		// Create DB and Dendrite base
		connStr, close := test.PrepareDBConnectionString(t, dbType)
		defer close()
		base, _, _ := testrig.Base(nil)
		defer base.Close()
		db, err := storage.NewUserAPIDatabase(base, &config.DatabaseOptions{
			ConnectionString: config.DataSource(connStr),
		}, "test", bcrypt.MinCost, 0, 0, "")
		if err != nil {
			t.Error(err)
		}

		// Prepare pusher with our test server URL
		if err := db.UpsertPusher(ctx, api.Pusher{
			Kind:    api.HTTPKind,
			AppID:   util.RandomString(8),
			PushKey: util.RandomString(8),
			Data: map[string]interface{}{
				"url":      srv.URL,
				"event_id": dummyEvent.EventID(),
			},
		}, aliceLocalpart, serverName); err != nil {
			t.Error(err)
		}

		// Insert a dummy event
		if err := db.InsertNotification(ctx, aliceLocalpart, serverName, dummyEvent.EventID(), 0, nil, &api.Notification{
			Event: gomatrixserverlib.HeaderedToClientEvent(dummyEvent, gomatrixserverlib.FormatAll),
		}); err != nil {
			t.Error(err)
		}

		// Notify the user about a new notification
		if err := userUtil.NotifyUserCountsAsync(ctx, pushgateway.NewHTTPClient(true), aliceLocalpart, serverName, db); err != nil {
			t.Error(err)
		}
		select {
		case <-time.After(time.Second * 5):
			t.Error("timed out waiting for response")
		case <-receivedRequest:
		}
	})

}
