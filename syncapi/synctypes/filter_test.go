package synctypes

import (
	"encoding/json"
	"reflect"
	"testing"
)

func Test_Filter(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  RoomEventFilter
	}{
		{
			name:  "empty types filter",
			input: []byte(`{ "types": [] }`),
			want: RoomEventFilter{
				Limit:                   0,
				NotSenders:              nil,
				NotTypes:                nil,
				Senders:                 nil,
				Types:                   &[]string{},
				LazyLoadMembers:         false,
				IncludeRedundantMembers: false,
				NotRooms:                nil,
				Rooms:                   nil,
				ContainsURL:             nil,
			},
		},
		{
			name:  "absent types filter",
			input: []byte(`{}`),
			want: RoomEventFilter{
				Limit:                   0,
				NotSenders:              nil,
				NotTypes:                nil,
				Senders:                 nil,
				Types:                   nil,
				LazyLoadMembers:         false,
				IncludeRedundantMembers: false,
				NotRooms:                nil,
				Rooms:                   nil,
				ContainsURL:             nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f RoomEventFilter
			if err := json.Unmarshal(tt.input, &f); err != nil {
				t.Fatalf("unable to parse filter: %v", err)
			}
			if !reflect.DeepEqual(f, tt.want) {
				t.Fatalf("Expected %+v\ngot %+v", tt.want, f)
			}
		})
	}

}
