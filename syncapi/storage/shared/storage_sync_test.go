package shared

import (
	"testing"

	"github.com/element-hq/dendrite/syncapi/synctypes"
)

func Test_isStatefilterEmpty(t *testing.T) {
	filterSet := []string{"a"}
	boolValue := false

	tests := []struct {
		name   string
		filter *synctypes.StateFilter
		want   bool
	}{
		{
			name:   "nil filter is empty",
			filter: nil,
			want:   true,
		},
		{
			name:   "Empty filter is empty",
			filter: &synctypes.StateFilter{},
			want:   true,
		},
		{
			name: "NotTypes is set",
			filter: &synctypes.StateFilter{
				NotTypes: &filterSet,
			},
		},
		{
			name: "Types is set",
			filter: &synctypes.StateFilter{
				Types: &filterSet,
			},
		},
		{
			name: "Senders is set",
			filter: &synctypes.StateFilter{
				Senders: &filterSet,
			},
		},
		{
			name: "NotSenders is set",
			filter: &synctypes.StateFilter{
				NotSenders: &filterSet,
			},
		},
		{
			name: "NotRooms is set",
			filter: &synctypes.StateFilter{
				NotRooms: &filterSet,
			},
		},
		{
			name: "ContainsURL is set",
			filter: &synctypes.StateFilter{
				ContainsURL: &boolValue,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStatefilterEmpty(tt.filter); got != tt.want {
				t.Errorf("isStatefilterEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
