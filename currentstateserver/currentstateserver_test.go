// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package currentstateserver

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/currentstateserver/inthttp"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/internal/test"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/naffka"
)

var (
	testRoomVersion = gomatrixserverlib.RoomVersionV1
	testData        = []json.RawMessage{
		[]byte(`{"auth_events":[],"content":{"creator":"@userid:kaer.morhen"},"depth":0,"event_id":"$0ok8ynDp7kjc95e3:kaer.morhen","hashes":{"sha256":"17kPoH+h0Dk4Omn7Sus0qMb6+oGcf+CZFEgDhv7UKWs"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"jP4a04f5/F10Pw95FPpdCyKAO44JOwUQ/MZOOeA/RTU1Dn+AHPMzGSaZnuGjRr/xQuADt+I3ctb5ZQfLKNzHDw"}},"state_key":"","type":"m.room.create"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}]],"content":{"membership":"join"},"depth":1,"event_id":"$LEwEu0kxrtu5fOiS:kaer.morhen","hashes":{"sha256":"B7M88PhXf3vd1LaFtjQutFu4x/w7fHD28XKZ4sAsJTo"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"p2vqmuJn7ZBRImctSaKbXCAxCcBlIjPH9JHte1ouIUGy84gpu4eLipOvSBCLL26hXfC0Zrm4WUto6Hr+ohdrCg"}},"state_key":"@userid:kaer.morhen","type":"m.room.member"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"join_rule":"public"},"depth":2,"event_id":"$SMHlqUrNhhBBRLeN:kaer.morhen","hashes":{"sha256":"vIuJQvmMjrGxshAkj1SXe0C4RqvMbv4ZADDw9pFCWqQ"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"hBMsb3Qppo3RaqqAl4JyTgaiWEbW5hlckATky6PrHun+F3YM203TzG7w9clwuQU5F5pZoB1a6nw+to0hN90FAw"}},"state_key":"","type":"m.room.join_rules"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"history_visibility":"shared"},"depth":3,"event_id":"$6F1yGIbO0J7TM93h:kaer.morhen","hashes":{"sha256":"Mr23GKSlZW7UCCYLgOWawI2Sg6KIoMjUWO2TDenuOgw"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$SMHlqUrNhhBBRLeN:kaer.morhen",{"sha256":"SylzE8U02I+6eyEHgL+FlU0L5YdqrVp8OOlxKS9VQW0"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"sHLKrFI3hKGrEJfpMVZSDS3LvLasQsy50CTsOwru9XTVxgRsPo6wozNtRVjxo1J3Rk18RC9JppovmQ5VR5EcDw"}},"state_key":"","type":"m.room.history_visibility"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"ban":50,"events":null,"events_default":0,"invite":0,"kick":50,"redact":50,"state_default":50,"users":null,"users_default":0},"depth":4,"event_id":"$UKNe10XzYzG0TeA9:kaer.morhen","hashes":{"sha256":"ngbP3yja9U5dlckKerUs/fSOhtKxZMCVvsfhPURSS28"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$6F1yGIbO0J7TM93h:kaer.morhen",{"sha256":"A4CucrKSoWX4IaJXhq02mBg1sxIyZEftbC+5p3fZAvk"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"zOmwlP01QL3yFchzuR9WHvogOoBZA3oVtNIF3lM0ZfDnqlSYZB9sns27G/4HVq0k7alaK7ZE3oGoCrVnMkPNCw"}},"state_key":"","type":"m.room.power_levels"}`),
		// messages
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":5,"event_id":"$gl2T9l3qm0kUbiIJ:kaer.morhen","hashes":{"sha256":"Qx3nRMHLDPSL5hBAzuX84FiSSP0K0Kju2iFoBWH4Za8"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$UKNe10XzYzG0TeA9:kaer.morhen",{"sha256":"KtSRyMjt0ZSjsv2koixTRCxIRCGoOp6QrKscsW97XRo"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"sqDgv3EG7ml5VREzmT9aZeBpS4gAPNIaIeJOwqjDhY0GPU/BcpX5wY4R7hYLrNe5cChgV+eFy/GWm1Zfg5FfDg"}},"type":"m.room.message"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":6,"event_id":"$MYSbs8m4rEbsCWXD:kaer.morhen","hashes":{"sha256":"kgbYM7v4Ud2YaBsjBTolM4ySg6rHcJNYI6nWhMSdFUA"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$gl2T9l3qm0kUbiIJ:kaer.morhen",{"sha256":"C/rD04h9wGxRdN2G/IBfrgoE1UovzLZ+uskwaKZ37/Q"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"x0UoKh968jj/F5l1/R7Ew0T6CTKuew3PLNHASNxqck/bkNe8yYQiDHXRr+kZxObeqPZZTpaF1+EI+bLU9W8GDQ"}},"type":"m.room.message"}`),
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":7,"event_id":"$N5x9WJkl9ClPrAEg:kaer.morhen","hashes":{"sha256":"FWM8oz4yquTunRZ67qlW2gzPDzdWfBP6RPHXhK1I/x8"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$MYSbs8m4rEbsCWXD:kaer.morhen",{"sha256":"fatqgW+SE8mb2wFn3UN+drmluoD4UJ/EcSrL6Ur9q1M"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"Y+LX/xcyufoXMOIoqQBNOzy6lZfUGB1ffgXIrSugk6obMiyAsiRejHQN/pciZXsHKxMJLYRFAz4zSJoS/LGPAA"}},"type":"m.room.message"}`),
	}
	testEvents      = []gomatrixserverlib.HeaderedEvent{}
	testStateEvents = make(map[gomatrixserverlib.StateKeyTuple]gomatrixserverlib.HeaderedEvent)

	kafkaTopic = "room_events"
)

func init() {
	for _, j := range testData {
		e, err := gomatrixserverlib.NewEventFromTrustedJSON(j, false, testRoomVersion)
		if err != nil {
			panic("cannot load test data: " + err.Error())
		}
		h := e.Headered(testRoomVersion)
		testEvents = append(testEvents, h)
		if e.StateKey() != nil {
			testStateEvents[gomatrixserverlib.StateKeyTuple{
				EventType: e.Type(),
				StateKey:  *e.StateKey(),
			}] = h
		}
	}
}

func MustWriteOutputEvent(t *testing.T, producer sarama.SyncProducer, out *roomserverAPI.OutputNewRoomEvent) error {
	value, err := json.Marshal(roomserverAPI.OutputEvent{
		Type:         roomserverAPI.OutputTypeNewRoomEvent,
		NewRoomEvent: out,
	})
	if err != nil {
		t.Fatalf("failed to marshal output event: %s", err)
	}
	_, _, err = producer.SendMessage(&sarama.ProducerMessage{
		Topic: kafkaTopic,
		Key:   sarama.StringEncoder(out.Event.RoomID()),
		Value: sarama.ByteEncoder(value),
	})
	if err != nil {
		t.Fatalf("failed to send message: %s", err)
	}
	return nil
}

func MustMakeInternalAPI(t *testing.T) (api.CurrentStateInternalAPI, sarama.SyncProducer) {
	cfg := &config.Dendrite{}
	cfg.Kafka.Topics.OutputRoomEvent = config.Topic(kafkaTopic)
	cfg.Database.CurrentState = config.DataSource("file::memory:")
	db, err := sqlutil.Open(sqlutil.SQLiteDriverName(), "file::memory:", nil)
	if err != nil {
		t.Fatalf("Failed to open naffka database: %s", err)
	}
	naffkaDB, err := naffka.NewSqliteDatabase(db)
	if err != nil {
		t.Fatalf("Failed to setup naffka database: %s", err)
	}
	naff, err := naffka.New(naffkaDB)
	if err != nil {
		t.Fatalf("Failed to create naffka consumer: %s", err)
	}
	return NewInternalAPI(cfg, naff), naff
}

func TestQueryCurrentState(t *testing.T) {
	currStateAPI, producer := MustMakeInternalAPI(t)
	plTuple := gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.power_levels",
		StateKey:  "",
	}
	plEvent := testEvents[4]
	MustWriteOutputEvent(t, producer, &roomserverAPI.OutputNewRoomEvent{
		Event:             plEvent,
		AddsStateEventIDs: []string{plEvent.EventID()},
	})
	// we have no good way to know /when/ the server has consumed the event
	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		req     api.QueryCurrentStateRequest
		wantRes api.QueryCurrentStateResponse
		wantErr error
	}{
		{
			req: api.QueryCurrentStateRequest{
				RoomID: plEvent.RoomID(),
				StateTuples: []gomatrixserverlib.StateKeyTuple{
					plTuple,
				},
			},
			wantRes: api.QueryCurrentStateResponse{
				StateEvents: map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent{
					plTuple: &plEvent,
				},
			},
		},
	}

	runCases := func(testAPI api.CurrentStateInternalAPI) {
		for _, tc := range testCases {
			var gotRes api.QueryCurrentStateResponse
			gotErr := testAPI.QueryCurrentState(context.TODO(), &tc.req, &gotRes)
			if tc.wantErr == nil && gotErr != nil || tc.wantErr != nil && gotErr == nil {
				t.Errorf("QueryCurrentState error, got %s want %s", gotErr, tc.wantErr)
				continue
			}
			for tuple, wantEvent := range tc.wantRes.StateEvents {
				gotEvent, ok := gotRes.StateEvents[tuple]
				if !ok {
					t.Errorf("QueryCurrentState want tuple %+v but it is missing from the response", tuple)
					continue
				}
				if !reflect.DeepEqual(gotEvent.JSON(), wantEvent.JSON()) {
					t.Errorf("QueryCurrentState tuple %+v got event JSON %s want %s", tuple, string(gotEvent.JSON()), string(wantEvent.JSON()))
				}
			}
		}
	}
	t.Run("HTTP API", func(t *testing.T) {
		router := mux.NewRouter().PathPrefix(httputil.InternalPathPrefix).Subrouter()
		AddInternalRoutes(router, currStateAPI)
		apiURL, cancel := test.ListenAndServe(t, router, false)
		defer cancel()
		httpAPI, err := inthttp.NewCurrentStateAPIClient(apiURL, &http.Client{})
		if err != nil {
			t.Fatalf("failed to create HTTP client")
		}
		runCases(httpAPI)
	})
	t.Run("Monolith", func(t *testing.T) {
		runCases(currStateAPI)
	})
}
