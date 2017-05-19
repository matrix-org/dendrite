package gomatrixserverlib

import (
	"encoding/json"
	"testing"
)

func TestRespSendJoinMarshalJSON(t *testing.T) {
	inputData := `{"pdus":[],"auth_chain":[]}`
	var input RespState
	if err := json.Unmarshal([]byte(inputData), &input); err != nil {
		t.Fatal(err)
	}

	gotBytes, err := json.Marshal(RespSendJoin(input))
	if err != nil {
		t.Fatal(err)
	}

	want := `[200,{"state":[],"auth_chain":[]}]`
	got := string(gotBytes)

	if want != got {
		t.Errorf("json.Marshal(RespSendJoin(%q)): wanted %q, got %q", inputData, want, got)
	}
}

func TestRespSendJoinUnmarshalJSON(t *testing.T) {
	inputData := `[200,{"state":[],"auth_chain":[]}]`
	var input RespSendJoin
	if err := json.Unmarshal([]byte(inputData), &input); err != nil {
		t.Fatal(err)
	}

	gotBytes, err := json.Marshal(RespState(input))
	if err != nil {
		t.Fatal(err)
	}

	want := `{"pdus":[],"auth_chain":[]}`
	got := string(gotBytes)

	if want != got {
		t.Errorf("json.Marshal(RespSendJoin(%q)): wanted %q, got %q", inputData, want, got)
	}

}
