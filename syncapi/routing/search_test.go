package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/element-hq/dendrite/internal/fulltext"
	"github.com/element-hq/dendrite/internal/sqlutil"
	rsapi "github.com/element-hq/dendrite/roomserver/api"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/test/testrig"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

type FakeSyncRoomserverAPI struct{ rsapi.SyncRoomserverAPI }

func (f *FakeSyncRoomserverAPI) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(string(senderID), true)
}

func TestSearch(t *testing.T) {
	alice := test.NewUser(t)
	aliceDevice := userapi.Device{UserID: alice.ID}
	room := test.NewRoom(t, alice)
	room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "context before"})
	room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "hello world3!"})
	room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "context after"})

	roomsFilter := []string{room.ID}
	roomsFilterUnknown := []string{"!unknown"}

	emptyFromString := ""
	fromStringValid := "1"
	fromStringInvalid := "iCantBeParsed"

	testCases := []struct {
		name              string
		wantOK            bool
		searchReq         SearchRequest
		device            *userapi.Device
		wantResponseCount int
		from              *string
	}{
		{
			name:      "no user ID",
			searchReq: SearchRequest{},
			device:    &userapi.Device{},
		},
		{
			name:      "with alice ID",
			wantOK:    true,
			searchReq: SearchRequest{},
			device:    &aliceDevice,
		},
		{
			name:   "searchTerm specified, found at the beginning",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{RoomEvents: RoomEvents{SearchTerm: "hello"}},
			},
			device:            &aliceDevice,
			wantResponseCount: 1,
		},
		{
			name:   "searchTerm specified, found at the end",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{RoomEvents: RoomEvents{SearchTerm: "world3"}},
			},
			device:            &aliceDevice,
			wantResponseCount: 1,
		},
		/* the following would need matchQuery.SetFuzziness(1) in bleve.go
		{
			name:   "searchTerm fuzzy search",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{RoomEvents: RoomEvents{SearchTerm: "hell"}}, // this still should find hello world
			},
			device:            &aliceDevice,
			wantResponseCount: 1,
		},
		*/
		{
			name:   "searchTerm specified but no result",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{RoomEvents: RoomEvents{SearchTerm: "i don't match"}},
			},
			device: &aliceDevice,
		},
		{
			name:   "filter on room",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{
					RoomEvents: RoomEvents{
						SearchTerm: "hello",
						Filter: synctypes.RoomEventFilter{
							Rooms: &roomsFilter,
						},
					},
				},
			},
			device:            &aliceDevice,
			wantResponseCount: 1,
		},
		{
			name: "filter on unknown room",
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{
					RoomEvents: RoomEvents{
						SearchTerm: "hello",
						Filter: synctypes.RoomEventFilter{
							Rooms: &roomsFilterUnknown,
						},
					},
				},
			},
			device: &aliceDevice,
		},
		{
			name:   "include state",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{
					RoomEvents: RoomEvents{
						SearchTerm: "hello",
						Filter: synctypes.RoomEventFilter{
							Rooms: &roomsFilter,
						},
						IncludeState: true,
					},
				},
			},
			device:            &aliceDevice,
			wantResponseCount: 1,
		},
		{
			name:   "empty from does not error",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{
					RoomEvents: RoomEvents{
						SearchTerm: "hello",
						Filter: synctypes.RoomEventFilter{
							Rooms: &roomsFilter,
						},
					},
				},
			},
			wantResponseCount: 1,
			device:            &aliceDevice,
			from:              &emptyFromString,
		},
		{
			name:   "valid from does not error",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{
					RoomEvents: RoomEvents{
						SearchTerm: "hello",
						Filter: synctypes.RoomEventFilter{
							Rooms: &roomsFilter,
						},
					},
				},
			},
			wantResponseCount: 1,
			device:            &aliceDevice,
			from:              &fromStringValid,
		},
		{
			name: "invalid from does error",
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{
					RoomEvents: RoomEvents{
						SearchTerm: "hello",
						Filter: synctypes.RoomEventFilter{
							Rooms: &roomsFilter,
						},
					},
				},
			},
			device: &aliceDevice,
			from:   &fromStringInvalid,
		},
		{
			name:   "order by stream position",
			wantOK: true,
			searchReq: SearchRequest{
				SearchCategories: SearchCategories{RoomEvents: RoomEvents{SearchTerm: "hello", OrderBy: "recent"}},
			},
			device:            &aliceDevice,
			wantResponseCount: 1,
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()

		// create requisites
		fts, err := fulltext.New(processCtx, cfg.SyncAPI.Fulltext)
		assert.NoError(t, err)
		assert.NotNil(t, fts)

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		db, err := storage.NewSyncServerDatasource(processCtx.Context(), cm, &cfg.SyncAPI.Database)
		assert.NoError(t, err)

		elements := []fulltext.IndexElement{}
		// store the events in the database
		var sp types.StreamPosition
		for _, x := range room.Events() {
			var stateEvents []*rstypes.HeaderedEvent
			var stateEventIDs []string
			if x.Type() == spec.MRoomMember {
				stateEvents = append(stateEvents, x)
				stateEventIDs = append(stateEventIDs, x.EventID())
			}
			x.StateKeyResolved = x.StateKey()
			sp, err = db.WriteEvent(processCtx.Context(), x, stateEvents, stateEventIDs, nil, nil, false, gomatrixserverlib.HistoryVisibilityShared)
			assert.NoError(t, err)
			if x.Type() != "m.room.message" {
				continue
			}
			elements = append(elements, fulltext.IndexElement{
				EventID:        x.EventID(),
				RoomID:         x.RoomID().String(),
				Content:        string(x.Content()),
				ContentType:    x.Type(),
				StreamPosition: int64(sp),
			})
		}
		// Index the events
		err = fts.Index(elements...)
		assert.NoError(t, err)

		// run the tests
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				reqBody := &bytes.Buffer{}
				err = json.NewEncoder(reqBody).Encode(tc.searchReq)
				assert.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, "/", reqBody)

				res := Search(req, tc.device, db, fts, tc.from, &FakeSyncRoomserverAPI{})
				if !tc.wantOK && !res.Is2xx() {
					return
				}
				resp, ok := res.JSON.(SearchResponse)
				if !ok && !tc.wantOK {
					t.Fatalf("not a SearchResponse: %T: %s", res.JSON, res.JSON)
				}
				assert.Equal(t, tc.wantResponseCount, resp.SearchCategories.RoomEvents.Count)

				// if we requested state, it should not be empty
				if tc.searchReq.SearchCategories.RoomEvents.IncludeState {
					assert.NotEmpty(t, resp.SearchCategories.RoomEvents.State)
				}
			})
		}
	})
}
