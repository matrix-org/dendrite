// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package fulltext_test

import (
	"reflect"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/internal/fulltext"
	"github.com/matrix-org/dendrite/setup/config"
)

func mustOpenIndex(t *testing.T, tempDir string) *fulltext.Search {
	t.Helper()
	cfg := config.Fulltext{
		Enabled:  true,
		InMemory: true,
		Language: "en",
	}
	if tempDir != "" {
		cfg.IndexPath = config.Path(tempDir)
		cfg.InMemory = false
	}
	fts, err := fulltext.New(cfg)
	if err != nil {
		t.Fatal("failed to open fulltext index:", err)
	}
	return fts
}

func mustAddTestData(t *testing.T, fts *fulltext.Search, firstStreamPos int64) (eventIDs, roomIDs []string) {
	t.Helper()
	// create some more random data
	var batchItems []fulltext.IndexElement
	streamPos := firstStreamPos

	wantRoomID := util.RandomString(16)

	for i := 0; i < 30; i++ {
		streamPos++
		eventID := util.RandomString(16)
		// Create more data for the first room
		if i > 15 {
			wantRoomID = util.RandomString(16)
		}
		e := fulltext.IndexElement{
			EventID:        eventID,
			RoomID:         wantRoomID,
			Content:        "lorem ipsum",
			StreamPosition: streamPos,
		}
		e.SetContentType("m.room.message")
		batchItems = append(batchItems, e)
		roomIDs = append(roomIDs, wantRoomID)
		eventIDs = append(eventIDs, eventID)
	}
	e := fulltext.IndexElement{
		EventID:        util.RandomString(16),
		RoomID:         wantRoomID,
		Content:        "Roomname testing",
		StreamPosition: streamPos,
	}
	e.SetContentType(gomatrixserverlib.MRoomName)
	batchItems = append(batchItems, e)
	e = fulltext.IndexElement{
		EventID:        util.RandomString(16),
		RoomID:         wantRoomID,
		Content:        "Room topic fulltext",
		StreamPosition: streamPos,
	}
	e.SetContentType(gomatrixserverlib.MRoomTopic)
	batchItems = append(batchItems, e)
	if err := fts.Index(batchItems...); err != nil {
		t.Fatalf("failed to batch insert elements: %v", err)
	}
	return eventIDs, roomIDs
}

func TestOpen(t *testing.T) {
	dataDir := t.TempDir()
	fts := mustOpenIndex(t, dataDir)
	if err := fts.Close(); err != nil {
		t.Fatal("unable to close fulltext index", err)
	}

	// open existing index
	fts = mustOpenIndex(t, dataDir)
	defer fts.Close()
}

func TestIndex(t *testing.T) {
	fts := mustOpenIndex(t, "")
	defer fts.Close()

	// add some data
	var streamPos int64 = 1
	roomID := util.RandomString(8)
	eventID := util.RandomString(16)
	e := fulltext.IndexElement{
		EventID:        eventID,
		RoomID:         roomID,
		Content:        "lorem ipsum",
		StreamPosition: streamPos,
	}
	e.SetContentType("m.room.message")

	if err := fts.Index(e); err != nil {
		t.Fatal("failed to index element", err)
	}

	// create some more random data
	mustAddTestData(t, fts, streamPos)
}

func TestDelete(t *testing.T) {
	fts := mustOpenIndex(t, "")
	defer fts.Close()
	eventIDs, roomIDs := mustAddTestData(t, fts, 0)
	res1, err := fts.Search("lorem", roomIDs[:1], nil, 50, 0, false)
	if err != nil {
		t.Fatal(err)
	}

	if err = fts.Delete(eventIDs[0]); err != nil {
		t.Fatal(err)
	}

	res2, err := fts.Search("lorem", roomIDs[:1], nil, 50, 0, false)
	if err != nil {
		t.Fatal(err)
	}

	if res1.Total <= res2.Total {
		t.Fatalf("got unexpected result: %d <= %d", res1.Total, res2.Total)
	}
}

func TestSearch(t *testing.T) {
	type args struct {
		term             string
		keys             []string
		limit            int
		from             int
		orderByStreamPos bool
		roomIndex        []int
	}
	tests := []struct {
		name      string
		args      args
		wantCount int
		wantErr   bool
	}{
		{
			name:      "Can search for many results in one room",
			wantCount: 16,
			args: args{
				term:      "lorem",
				roomIndex: []int{0},
				limit:     20,
			},
		},
		{
			name:      "Can search for one result in one room",
			wantCount: 1,
			args: args{
				term:      "lorem",
				roomIndex: []int{16},
				limit:     20,
			},
		},
		{
			name:      "Can search for many results in multiple rooms",
			wantCount: 17,
			args: args{
				term:      "lorem",
				roomIndex: []int{0, 16},
				limit:     20,
			},
		},
		{
			name:      "Can search for many results in all rooms, reversed",
			wantCount: 30,
			args: args{
				term:             "lorem",
				limit:            30,
				orderByStreamPos: true,
			},
		},
		{
			name:      "Can search for specific search room name",
			wantCount: 1,
			args: args{
				term:      "testing",
				roomIndex: []int{},
				limit:     20,
				keys:      []string{"content.name"},
			},
		},
		{
			name:      "Can search for specific search room topic",
			wantCount: 1,
			args: args{
				term:      "fulltext",
				roomIndex: []int{},
				limit:     20,
				keys:      []string{"content.topic"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := mustOpenIndex(t, "")
			eventIDs, roomIDs := mustAddTestData(t, f, 0)
			var searchRooms []string
			for _, x := range tt.args.roomIndex {
				searchRooms = append(searchRooms, roomIDs[x])
			}
			t.Logf("searching in rooms: %v - %v\n", searchRooms, tt.args.keys)

			got, err := f.Search(tt.args.term, searchRooms, tt.args.keys, tt.args.limit, tt.args.from, tt.args.orderByStreamPos)
			if (err != nil) != tt.wantErr {
				t.Errorf("Search() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(len(got.Hits), tt.wantCount) {
				t.Errorf("Search() got = %v, want %v", len(got.Hits), tt.wantCount)
			}
			if tt.args.orderByStreamPos {
				if got.Hits[0].ID != eventIDs[29] {
					t.Fatalf("expected ID %s, got %s", eventIDs[29], got.Hits[0].ID)
				}
			}
		})
	}
}
