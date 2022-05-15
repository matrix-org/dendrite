package fulltext

import (
	"testing"

	"github.com/matrix-org/util"
)

func TestSearch(t *testing.T) {
	// create new index
	dataDir := t.TempDir()
	fts, err := New(dataDir)
	if err != nil {
		t.Fatal("failed to open fulltext index:", err)
	}
	if err = fts.Close(); err != nil {
		t.Fatal("unable to close fulltext index", err)
	}

	// open existing index
	fts, err = New(dataDir)
	if err != nil {
		t.Fatal("failed to open fulltext index:", err)
	}
	defer fts.Close()
	if fts == nil {
		t.Fatal("fts is nil")
	}

	// add some data
	e := IndexElement{
		EventID: util.RandomString(16),
		Type:    "m.room.message",
		RoomID:  util.RandomString(8),
		Content: "lorem ipsum",
	}

	if err = fts.Index(e); err != nil {
		t.Fatal("failed to index element", err)
	}

	eventID := util.RandomString(16)
	e = IndexElement{
		EventID: eventID,
		Type:    "m.room.message",
		RoomID:  util.RandomString(8),
		Content: "lorem ipsum",
	}

	if err = fts.Index(e); err != nil {
		t.Fatal("failed to index element", err)
	}

	// search data
	res, err := fts.Search("lorem")
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

	res, err = fts.Search("lorem")
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("expected %d results, got %d", 2, res.Total)
	}
}
