package routing

import (
	"reflect"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
)

func pubRoom(name string) gomatrixserverlib.PublicRoom {
	return gomatrixserverlib.PublicRoom{
		Name: name,
	}
}

func TestSliceInto(t *testing.T) {
	slice := []gomatrixserverlib.PublicRoom{
		pubRoom("a"), pubRoom("b"), pubRoom("c"), pubRoom("d"), pubRoom("e"), pubRoom("f"), pubRoom("g"),
	}
	limit := int64(3)
	testCases := []struct {
		since      int64
		wantPrev   int
		wantNext   int
		wantSubset []gomatrixserverlib.PublicRoom
	}{
		{
			since:      0,
			wantPrev:   -1,
			wantNext:   3,
			wantSubset: slice[0:3],
		},
		{
			since:      3,
			wantPrev:   0,
			wantNext:   6,
			wantSubset: slice[3:6],
		},
		{
			since:      6,
			wantPrev:   3,
			wantNext:   -1,
			wantSubset: slice[6:7],
		},
	}
	for _, tc := range testCases {
		subset, prev, next := sliceInto(slice, tc.since, limit)
		if !reflect.DeepEqual(subset, tc.wantSubset) {
			t.Errorf("returned subset is wrong, got %v want %v", subset, tc.wantSubset)
		}
		if prev != tc.wantPrev {
			t.Errorf("returned prev is wrong, got %d want %d", prev, tc.wantPrev)
		}
		if next != tc.wantNext {
			t.Errorf("returned next is wrong, got %d want %d", next, tc.wantNext)
		}
	}
}
