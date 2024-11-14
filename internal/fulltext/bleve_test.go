// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package fulltext_test

import (
	"reflect"
	"testing"

	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	"github.com/element-hq/dendrite/internal/fulltext"
	"github.com/element-hq/dendrite/setup/config"
)

func mustOpenIndex(t *testing.T, tempDir string) (*fulltext.Search, *process.ProcessContext) {
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
	ctx := process.NewProcessContext()
	fts, err := fulltext.New(ctx, cfg)
	if err != nil {
		t.Fatal("failed to open fulltext index:", err)
	}
	return fts, ctx
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
	e.SetContentType(spec.MRoomName)
	batchItems = append(batchItems, e)
	e = fulltext.IndexElement{
		EventID:        util.RandomString(16),
		RoomID:         wantRoomID,
		Content:        "Room topic fulltext",
		StreamPosition: streamPos,
	}
	e.SetContentType(spec.MRoomTopic)
	batchItems = append(batchItems, e)
	if err := fts.Index(batchItems...); err != nil {
		t.Fatalf("failed to batch insert elements: %v", err)
	}
	return eventIDs, roomIDs
}

func TestOpen(t *testing.T) {
	dataDir := t.TempDir()
	_, ctx := mustOpenIndex(t, dataDir)
	ctx.ShutdownDendrite()

	// open existing index
	_, ctx = mustOpenIndex(t, dataDir)
	ctx.ShutdownDendrite()
}

func TestIndex(t *testing.T) {
	fts, ctx := mustOpenIndex(t, "")
	defer ctx.ShutdownDendrite()

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
	fts, ctx := mustOpenIndex(t, "")
	defer ctx.ShutdownDendrite()
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
		name           string
		args           args
		wantCount      int
		wantErr        bool
		wantHighlights []string
	}{
		{
			name:           "Can search for many results in one room",
			wantCount:      16,
			wantHighlights: []string{"lorem"},
			args: args{
				term:      "lorem",
				roomIndex: []int{0},
				limit:     20,
			},
		},
		{
			name:           "Can search for one result in one room",
			wantCount:      1,
			wantHighlights: []string{"lorem"},
			args: args{
				term:      "lorem",
				roomIndex: []int{16},
				limit:     20,
			},
		},
		{
			name:           "Can search for many results in multiple rooms",
			wantCount:      17,
			wantHighlights: []string{"lorem"},
			args: args{
				term:      "lorem",
				roomIndex: []int{0, 16},
				limit:     20,
			},
		},
		{
			name:           "Can search for many results in all rooms, reversed",
			wantCount:      30,
			wantHighlights: []string{"lorem"},
			args: args{
				term:             "lorem",
				limit:            30,
				orderByStreamPos: true,
			},
		},
		{
			name:           "Can search for specific search room name",
			wantCount:      1,
			wantHighlights: []string{"testing"},
			args: args{
				term:      "testing",
				roomIndex: []int{},
				limit:     20,
				keys:      []string{"content.name"},
			},
		},
		{
			name:           "Can search for specific search room topic",
			wantCount:      1,
			wantHighlights: []string{"fulltext"},
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
			f, ctx := mustOpenIndex(t, "")
			defer ctx.ShutdownDendrite()
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

			highlights := f.GetHighlights(got)
			if !reflect.DeepEqual(highlights, tt.wantHighlights) {
				t.Errorf("Search() got highligts = %v, want %v", highlights, tt.wantHighlights)
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
