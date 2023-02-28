package shared

import (
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
)

func Test_isStatefilterEmpty(t *testing.T) {
	filterSet := []string{"a"}
	boolValue := false

	tests := []struct {
		name   string
		filter *gomatrixserverlib.StateFilter
		want   bool
	}{
		{
			name:   "nil filter is empty",
			filter: nil,
			want:   true,
		},
		{
			name:   "Empty filter is empty",
			filter: &gomatrixserverlib.StateFilter{},
			want:   true,
		},
		{
			name: "NotTypes is set",
			filter: &gomatrixserverlib.StateFilter{
				NotTypes: &filterSet,
			},
		},
		{
			name: "Types is set",
			filter: &gomatrixserverlib.StateFilter{
				Types: &filterSet,
			},
		},
		{
			name: "Senders is set",
			filter: &gomatrixserverlib.StateFilter{
				Senders: &filterSet,
			},
		},
		{
			name: "NotSenders is set",
			filter: &gomatrixserverlib.StateFilter{
				NotSenders: &filterSet,
			},
		},
		{
			name: "NotRooms is set",
			filter: &gomatrixserverlib.StateFilter{
				NotRooms: &filterSet,
			},
		},
		{
			name: "ContainsURL is set",
			filter: &gomatrixserverlib.StateFilter{
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
