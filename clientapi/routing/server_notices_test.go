package routing

import (
	"testing"
)

func Test_sendServerNoticeRequest_validate(t *testing.T) {
	type fields struct {
		UserID  string `json:"user_id,omitempty"`
		Content struct {
			MsgType string `json:"msgtype,omitempty"`
			Body    string `json:"body,omitempty"`
		} `json:"content,omitempty"`
		Type     string `json:"type,omitempty"`
		StateKey string `json:"state_key,omitempty"`
	}

	content := struct {
		MsgType string `json:"msgtype,omitempty"`
		Body    string `json:"body,omitempty"`
	}{
		MsgType: "m.text",
		Body:    "Hello world!",
	}

	tests := []struct {
		name   string
		fields fields
		wantOk bool
	}{
		{
			name:   "empty request",
			fields: fields{},
		},
		{
			name: "msgtype empty",
			fields: fields{
				UserID: "@alice:localhost",
				Content: struct {
					MsgType string `json:"msgtype,omitempty"`
					Body    string `json:"body,omitempty"`
				}{
					Body: "Hello world!",
				},
			},
		},
		{
			name: "msg body empty",
			fields: fields{
				UserID: "@alice:localhost",
			},
		},
		{
			name: "statekey empty",
			fields: fields{
				UserID:  "@alice:localhost",
				Content: content,
			},
			wantOk: true,
		},
		{
			name: "type empty",
			fields: fields{
				UserID:  "@alice:localhost",
				Content: content,
			},
			wantOk: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := sendServerNoticeRequest{
				UserID:   tt.fields.UserID,
				Content:  tt.fields.Content,
				Type:     tt.fields.Type,
				StateKey: tt.fields.StateKey,
			}
			if gotOk := r.valid(); gotOk != tt.wantOk {
				t.Errorf("valid() = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}
