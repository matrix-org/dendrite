package types

import "testing"

func TestPresenceFromString(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantStatus Presence
		wantOk     bool
	}{
		{
			name:       "presence unavailable",
			input:      "unavailable",
			wantStatus: PresenceUnavailable,
			wantOk:     true,
		},
		{
			name:       "presence online",
			input:      "OnLINE",
			wantStatus: PresenceOnline,
			wantOk:     true,
		},
		{
			name:       "unknown presence",
			input:      "unknown",
			wantStatus: 0,
			wantOk:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := PresenceFromString(tt.input)
			if got != tt.wantStatus {
				t.Errorf("PresenceFromString() got = %v, want %v", got, tt.wantStatus)
			}
			if got1 != tt.wantOk {
				t.Errorf("PresenceFromString() got1 = %v, want %v", got1, tt.wantOk)
			}
		})
	}
}
