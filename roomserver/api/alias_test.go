package api

import "testing"

func TestAliasEvent_Valid(t *testing.T) {
	type fields struct {
		Alias      string
		AltAliases []string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "empty alias",
			fields: fields{
				Alias: "",
			},
			want: true,
		},
		{
			name: "empty alias, invalid alt aliases",
			fields: fields{
				Alias:      "",
				AltAliases: []string{"%not:valid.local"},
			},
		},
		{
			name: "valid alias, invalid alt aliases",
			fields: fields{
				Alias:      "#valid:test.local",
				AltAliases: []string{"%not:valid.local"},
			},
		},
		{
			name: "empty alias, invalid alt aliases",
			fields: fields{
				Alias:      "",
				AltAliases: []string{"%not:valid.local"},
			},
		},
		{
			name: "invalid alias",
			fields: fields{
				Alias:      "%not:valid.local",
				AltAliases: []string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := AliasEvent{
				Alias:      tt.fields.Alias,
				AltAliases: tt.fields.AltAliases,
			}
			if got := a.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}
