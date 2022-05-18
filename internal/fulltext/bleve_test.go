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
	"sort"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/fulltext"
	"github.com/matrix-org/util"
)

func TestSearch(t *testing.T) {
	// create new index
	dataDir := t.TempDir()
	fts, err := fulltext.New(dataDir)
	if err != nil {
		t.Fatal("failed to open fulltext index:", err)
	}
	if err = fts.Close(); err != nil {
		t.Fatal("unable to close fulltext index", err)
	}

	// open existing index
	fts, err = fulltext.New(dataDir)
	if err != nil {
		t.Fatal("failed to open fulltext index:", err)
	}
	defer fts.Close()
	if fts == nil {
		t.Fatal("fts is nil")
	}

	// add some data
	roomID := util.RandomString(8)
	e := fulltext.IndexElement{
		EventID: util.RandomString(16),
		RoomID:  roomID,
		Content: "lorem ipsum",
		Time:    time.Now(),
	}

	if err = fts.Index(e); err != nil {
		t.Fatal("failed to index element", err)
	}

	eventID := util.RandomString(16)
	e = fulltext.IndexElement{
		EventID: eventID,
		RoomID:  roomID,
		Content: "lorem ipsum",
		Time:    time.Now(),
	}

	if err = fts.Index(e); err != nil {
		t.Fatal("failed to index element", err)
	}

	// search data
	res, err := fts.Search("lorem", nil, 10, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("expected %d results, got %d", 2, res.Total)
	}

	// remove element
	if err = fts.Delete(eventID); err != nil {
		t.Fatal(err)
	}

	// create some more random data
	var batchItems []fulltext.IndexElement

	wantRoomID := util.RandomString(8)
	for i := 0; i < 30; i++ {
		eventID = util.RandomString(16)
		e = fulltext.IndexElement{
			EventID: eventID,
			RoomID:  wantRoomID,
			Content: "lorem ipsum",
			Time:    time.Now(),
		}
		batchItems = append(batchItems, e)
	}

	// Index the data
	if err = fts.BatchIndex(batchItems); err != nil {
		t.Fatal("failed to batch index")
	}

	// search for lorem, but only in a given room
	searchRooms := []string{roomID}
	res, err = fts.Search("lorem", searchRooms, 10, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("expected %d results, got %d", 1, res.Total)
	}

	// can get sorted results
	res, err = fts.Search("lorem", []string{wantRoomID}, 10, 0, true)
	if err != nil {
		t.Fatal(err)
	}

	if res.Hits[0].ID != eventID {
		t.Fatalf("expected %s to be first, got %s", eventID, res.Hits[0].ID)
	}

	sort.Sort(fulltext.IndexElements(batchItems))
	if eventID != batchItems[0].EventID {
		t.Fatalf("expected %s to be first, got %s", eventID, batchItems[0].EventID)
	}

	// test back pagination

}
